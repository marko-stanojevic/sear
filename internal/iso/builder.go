package iso

import (
	"bufio"
	"context"
	"embed"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

// dockerFS embeds the Docker build contexts and overlay files.
//
//go:embed docker
var dockerFS embed.FS

const (
	// baseTag is the pre-built live-system image (Debian + kernel + live-boot).
	// Built once; Docker layer cache makes subsequent starts instant.
	baseTag = "kompakt-livebase:local"

	// pkgTag is the pre-built packaging image (squashfs-tools + xorriso + grub).
	pkgTag = "kompakt-isopkg:local"

	// BuildTimeout is the maximum time a single ISO build may run.
	BuildTimeout = 60 * time.Minute

	// minDiskBytes is the minimum free space required before starting a build (~6 GB).
	minDiskBytes = 6 << 30
)

// BuildRequest holds all parameters needed for one ISO build.
type BuildRequest struct {
	ID                          string
	CustomName                  string // optional; used in the output filename
	ServerURL                   string
	SecretName                  string
	SecretValue                 string
	TLSSkipVerify               bool
	AgentBinaryPath             string
	OutputDir                   string
	ExtraDockerfileInstructions string // optional; appended to the agent layer Dockerfile
}

// RunBuild executes the Docker-based ISO build. Intended to be called in a goroutine.
// The caller is responsible for applying a timeout to ctx (e.g. BuildTimeout).
func RunBuild(ctx context.Context, store *BuildStore, build *Build, req BuildRequest) {
	build.setRunning(store)
	isoPath, err := runBuild(ctx, req, build.AppendLog)
	if err != nil {
		build.AppendLog("ERROR: " + err.Error())
		build.setFailed(err.Error(), store)
		return
	}
	build.setCompleted(isoPath, store)
}

func runBuild(ctx context.Context, req BuildRequest, onLog func(string)) (string, error) {
	absOutputDir, err := filepath.Abs(req.OutputDir)
	if err != nil {
		return "", fmt.Errorf("resolving output dir: %w", err)
	}
	if err := os.MkdirAll(absOutputDir, 0o750); err != nil {
		return "", fmt.Errorf("creating output dir: %w", err)
	}

	if avail, err := availableDiskBytes(absOutputDir); err == nil && avail > 0 && avail < minDiskBytes {
		return "", fmt.Errorf("insufficient disk space: %.1f GB available, need at least %.1f GB",
			float64(avail)/(1<<30), float64(minDiskBytes)/(1<<30))
	}

	tmpDir, err := os.MkdirTemp(absOutputDir, ".build-*")
	if err != nil {
		return "", fmt.Errorf("creating temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)
	// Docker Desktop's VM accesses host paths via virtiofs; 0700 directories
	// are not traversable by the VM process.
	_ = os.Chmod(tmpDir, 0o755)

	bootDir := filepath.Join(tmpDir, "boot")
	grubDir := filepath.Join(bootDir, "grub")
	liveDir := filepath.Join(tmpDir, "live")
	for _, d := range []string{grubDir, liveDir} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			return "", err
		}
	}

	// Ensure pre-built base and packaging images exist (Docker caches them).
	if err := ensureImage(ctx, baseTag, "base.Dockerfile", onLog); err != nil {
		return "", err
	}
	if err := ensureImage(ctx, pkgTag, "pkg.Dockerfile", onLog); err != nil {
		return "", err
	}

	// Build a transient image that layers this build's agent binary on the base.
	agentTag := "kompakt-agent-iso:" + req.ID
	if err := buildAgentImage(ctx, agentTag, req, onLog); err != nil {
		return "", err
	}
	defer func() {
		if err := exec.Command("docker", "rmi", "-f", agentTag).Run(); err != nil {
			slog.Debug("iso: failed to remove transient agent image", "tag", agentTag, "error", err)
		}
	}()

	// Extract kernel + initrd from the base image (same files for every build).
	if err := extractBootFiles(ctx, bootDir, onLog); err != nil {
		return "", err
	}

	// Export the agent image filesystem and compress it into a squashfs.
	squashfsPath := filepath.Join(liveDir, "filesystem.squashfs")
	if err := createSquashfs(ctx, agentTag, squashfsPath, onLog); err != nil {
		return "", err
	}

	// Write GRUB boot menu.
	if err := writeGrubConfig(grubDir); err != nil {
		return "", fmt.Errorf("writing grub.cfg: %w", err)
	}

	// Assemble the final hybrid BIOS+UEFI ISO.
	isoName := "kompakt-agent-live"
	if n := sanitizeName(req.CustomName); n != "" {
		isoName = n
	}
	finalPath := filepath.Join(absOutputDir, isoName+"-"+time.Now().UTC().Format("20060102-150405")+".iso")
	if err := buildISO(ctx, tmpDir, finalPath, onLog); err != nil {
		return "", err
	}

	onLog("ISO ready: " + filepath.Base(finalPath))
	return finalPath, nil
}

// ensureImage checks whether tag exists locally and, if not, builds it from
// dockerfileName (a path inside the embedded docker/ directory).
func ensureImage(ctx context.Context, tag, dockerfileName string, onLog func(string)) error {
	out, err := exec.CommandContext(ctx, "docker", "images", "-q", tag).Output()
	if err == nil && strings.TrimSpace(string(out)) != "" {
		return nil // already present
	}

	onLog("Building image " + tag + " (one-time, results are cached by Docker)…")
	ctxDir, err := os.MkdirTemp("", "kompakt-img-ctx-*")
	if err != nil {
		return fmt.Errorf("creating context dir: %w", err)
	}
	defer os.RemoveAll(ctxDir)

	if err := extractFS(dockerFS, "docker", ctxDir); err != nil {
		return fmt.Errorf("extracting embedded context: %w", err)
	}
	_ = os.Chmod(filepath.Join(ctxDir, "files", "kompakt-start"), 0o755)

	cmd := exec.CommandContext(ctx, "docker", "build",
		"--platform", "linux/amd64",
		"-t", tag,
		"-f", filepath.Join(ctxDir, dockerfileName),
		ctxDir,
	)
	return streamCommand(cmd, onLog)
}

// buildAgentImage creates a thin image on top of baseTag that adds the agent
// binary and writes the build's runtime config as the static fallback.
func buildAgentImage(ctx context.Context, tag string, req BuildRequest, onLog func(string)) error {
	onLog("Layering agent binary into image…")

	buildDir, err := os.MkdirTemp("", "kompakt-agent-build-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(buildDir)
	_ = os.Chmod(buildDir, 0o755)

	if err := copyFile(req.AgentBinaryPath, filepath.Join(buildDir, "kompakt-agent"), 0o755); err != nil {
		return fmt.Errorf("copying agent binary: %w", err)
	}

	tlsStr := "false"
	if req.TLSSkipVerify {
		tlsStr = "true"
	}
	// Embed the server URL and secret as a static config so the ISO works
	// out-of-the-box without needing kernel command-line parameters.
	staticCfg := fmt.Sprintf(
		"server_url: %q\nregistration_secret: %q\ndisable_tls_verification: %s\n"+
			"state_file: /var/lib/kompakt/state.json\nwork_dir: /var/lib/kompakt/work\n"+
			"reconnect_interval_seconds: 10\n",
		req.ServerURL, req.SecretValue, tlsStr,
	)
	if err := os.WriteFile(filepath.Join(buildDir, "agent.config.yml"), []byte(staticCfg), 0o644); err != nil {
		return err
	}

	dockerfile := fmt.Sprintf(
		"FROM %s\nCOPY kompakt-agent /usr/local/bin/kompakt-agent\nCOPY agent.config.yml /etc/kompakt/agent.config.yml\n",
		baseTag,
	)
	if req.ExtraDockerfileInstructions != "" {
		dockerfile += req.ExtraDockerfileInstructions + "\n"
	}
	if err := os.WriteFile(filepath.Join(buildDir, "Dockerfile"), []byte(dockerfile), 0o644); err != nil {
		return err
	}

	cmd := exec.CommandContext(ctx, "docker", "build", "--platform", "linux/amd64", "-t", tag, buildDir)
	return streamCommand(cmd, onLog)
}

// extractBootFiles copies vmlinuz and initrd.img out of the base image.
func extractBootFiles(ctx context.Context, bootDir string, onLog func(string)) error {
	onLog("Extracting kernel and initramfs…")

	out, err := exec.CommandContext(ctx, "docker", "create", "--platform", "linux/amd64", baseTag).Output()
	if err != nil {
		return fmt.Errorf("creating base container: %w", err)
	}
	containerID := strings.TrimSpace(string(out))
	defer func() {
		if err := exec.Command("docker", "rm", "-f", containerID).Run(); err != nil {
			slog.Debug("iso: failed to remove boot container", "container_id", containerID, "error", err)
		}
	}()

	tmpBoot, err := os.MkdirTemp("", "kompakt-boot-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmpBoot)

	if out, err := exec.CommandContext(ctx, "docker", "cp", containerID+":/boot", tmpBoot).CombinedOutput(); err != nil {
		return fmt.Errorf("docker cp /boot: %s: %w", strings.TrimSpace(string(out)), err)
	}

	srcBoot := filepath.Join(tmpBoot, "boot")
	entries, err := os.ReadDir(srcBoot)
	if err != nil {
		return fmt.Errorf("reading extracted boot dir: %w", err)
	}

	var vmlinuz, initrd string
	for _, e := range entries {
		name := e.Name()
		if strings.HasPrefix(name, "vmlinuz-") && !strings.HasSuffix(name, ".old") {
			vmlinuz = filepath.Join(srcBoot, name)
		}
		if strings.HasPrefix(name, "initrd.img-") && !strings.HasSuffix(name, ".old") {
			initrd = filepath.Join(srcBoot, name)
		}
	}
	if vmlinuz == "" || initrd == "" {
		return fmt.Errorf("vmlinuz or initrd.img not found in base image /boot/ (%d entries)", len(entries))
	}
	if err := copyFile(vmlinuz, filepath.Join(bootDir, "vmlinuz"), 0o644); err != nil {
		return err
	}
	return copyFile(initrd, filepath.Join(bootDir, "initrd.img"), 0o644)
}

// createSquashfs pipes docker export of agentTag through mksquashfs (running
// inside the pkg container) to produce a compressed squashfs at squashfsPath.
func createSquashfs(ctx context.Context, agentTag, squashfsPath string, onLog func(string)) error {
	onLog("Creating root filesystem squashfs (this may take several minutes)…")

	out, err := exec.CommandContext(ctx, "docker", "create", "--platform", "linux/amd64", agentTag).Output()
	if err != nil {
		return fmt.Errorf("creating agent container: %w", err)
	}
	containerID := strings.TrimSpace(string(out))
	defer func() {
		if err := exec.Command("docker", "rm", "-f", containerID).Run(); err != nil {
			slog.Debug("iso: failed to remove squashfs container", "container_id", containerID, "error", err)
		}
	}()

	absDir, err := filepath.Abs(filepath.Dir(squashfsPath))
	if err != nil {
		return err
	}
	outName := filepath.Base(squashfsPath)

	exportCmd := exec.CommandContext(ctx, "docker", "export", containerID)
	mksqCmd := exec.CommandContext(ctx, "docker", "run", "--rm", "-i",
		"--platform", "linux/amd64",
		"-v", absDir+":/out",
		pkgTag,
		"mksquashfs", "-", "/out/"+outName, "-tar", "-comp", "xz", "-processors", "2",
	)

	pr, pw := io.Pipe()
	exportCmd.Stdout = pw
	mksqCmd.Stdin = pr

	var mksqStderr strings.Builder
	mksqCmd.Stderr = &mksqStderr

	if err := exportCmd.Start(); err != nil {
		_ = pw.Close()
		_ = pr.Close()
		return fmt.Errorf("starting docker export: %w", err)
	}
	if err := mksqCmd.Start(); err != nil {
		_ = exportCmd.Process.Kill()
		_ = exportCmd.Wait()
		_ = pw.Close()
		_ = pr.Close()
		return fmt.Errorf("starting mksquashfs: %w", err)
	}

	exportErr := exportCmd.Wait()
	if exportErr != nil {
		pw.CloseWithError(exportErr)
	} else {
		pw.Close()
	}
	mksqErr := mksqCmd.Wait()
	pr.Close()

	if exportErr != nil {
		return fmt.Errorf("docker export: %w", exportErr)
	}
	if mksqErr != nil {
		return fmt.Errorf("mksquashfs: %s: %w", strings.TrimSpace(mksqStderr.String()), mksqErr)
	}
	return nil
}

// writeGrubConfig writes a minimal GRUB menu that boots the live system.
func writeGrubConfig(grubDir string) error {
	cfg := `set timeout=5
set default=0

menuentry "Kompakt Agent Live" {
    linux /boot/vmlinuz boot=live components quiet
    initrd /boot/initrd.img
}
`
	return os.WriteFile(filepath.Join(grubDir, "grub.cfg"), []byte(cfg), 0o644)
}

// buildISO runs grub-mkrescue inside the pkg container to produce a hybrid
// BIOS+UEFI bootable ISO from the staging directory.
func buildISO(ctx context.Context, stagingDir, finalPath string, onLog func(string)) error {
	onLog("Building bootable ISO…")

	absStaging, err := filepath.Abs(stagingDir)
	if err != nil {
		return err
	}
	absOutputDir, err := filepath.Abs(filepath.Dir(finalPath))
	if err != nil {
		return err
	}

	cmd := exec.CommandContext(ctx, "docker", "run", "--rm",
		"--platform", "linux/amd64",
		"-v", absStaging+":/staging:ro",
		"-v", absOutputDir+":/output",
		pkgTag,
		"grub-mkrescue", "-o", "/output/"+filepath.Base(finalPath), "/staging",
	)
	return streamCommand(cmd, onLog)
}

// streamCommand runs cmd, relaying stdout+stderr to onLog, and returns any error.
func streamCommand(cmd *exec.Cmd, onLog func(string)) error {
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("stdout pipe: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("stderr pipe: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	var wg sync.WaitGroup
	logCh := make(chan string, 256)
	for _, r := range []io.Reader{stdout, stderr} {
		wg.Add(1)
		go func(rd io.Reader) {
			defer wg.Done()
			sc := bufio.NewScanner(rd)
			for sc.Scan() {
				logCh <- sc.Text()
			}
		}(r)
	}
	go func() { wg.Wait(); close(logCh) }()
	for line := range logCh {
		onLog(line)
	}
	return cmd.Wait()
}

// extractFS extracts all files from fsys rooted at dir into dstDir on disk.
func extractFS(fsys embed.FS, dir, dstDir string) error {
	return fs.WalkDir(fsys, dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(dir, path)
		target := filepath.Join(dstDir, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		data, err := fsys.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, 0o644)
	})
}

// FindAgentBinary locates the kompakt-agent binary next to the running
// executable or in standard install paths.
func FindAgentBinary() (string, error) {
	var candidates []string
	if exe, err := os.Executable(); err == nil {
		candidates = append(candidates, filepath.Join(filepath.Dir(exe), "kompakt-agent"))
	}
	candidates = append(candidates, "/usr/local/bin/kompakt-agent", "/usr/bin/kompakt-agent")
	for _, c := range candidates {
		if fi, err := os.Stat(c); err == nil && !fi.IsDir() {
			return c, nil
		}
	}
	return "", fmt.Errorf(
		"kompakt-agent binary not found; install it next to the kompakt binary or at /usr/local/bin/kompakt-agent",
	)
}

// sanitizeName converts s into a safe filename component (alphanumeric + dash +
// underscore only). Returns "" if nothing valid remains after sanitization.
var sanitizeRe = regexp.MustCompile(`[^a-zA-Z0-9_-]+`)

func sanitizeName(s string) string {
	s = sanitizeRe.ReplaceAllString(strings.TrimSpace(s), "-")
	s = strings.Trim(s, "-")
	return s
}

func copyFile(src, dst string, mode os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}
