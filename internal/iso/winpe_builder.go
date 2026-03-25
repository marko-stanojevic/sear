package iso

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const (
	winpeBaseTag = "kompakt-winpe-base:local"
)

// runWinPEBuild runs the Windows PE ISO build pipeline using Windows containers.
// Requires Docker Desktop to be in Windows Containers mode.
func runWinPEBuild(ctx context.Context, req BuildRequest, onLog func(string)) (string, error) {
	if runtime.GOOS != "windows" {
		return "", fmt.Errorf("WinPE builds are only supported on Windows hosts")
	}

	// Check Docker is in Windows Containers mode.
	mode, err := detectDockerMode()
	if err != nil {
		return "", fmt.Errorf("detecting Docker mode: %w", err)
	}
	if mode != "windows" {
		return "", fmt.Errorf("WinPE builds require Docker in Windows Containers mode (currently in %q mode); switch via the Docker Desktop tray icon", mode)
	}

	absOutputDir, err := filepath.Abs(req.OutputDir)
	if err != nil {
		return "", fmt.Errorf("resolving output dir: %w", err)
	}
	if err := os.MkdirAll(absOutputDir, 0o750); err != nil {
		return "", fmt.Errorf("creating output dir: %w", err)
	}

	// Step 1: Build base WinPE image (ADK installed; cached after first build).
	if err := ensureWinPEImage(ctx, onLog); err != nil {
		return "", err
	}

	// Step 2: Build transient image with agent binary + config layered on top.
	agentTag := "kompakt-winpe-agent:" + req.ID
	if err := buildWinPEAgentImage(ctx, agentTag, req, onLog); err != nil {
		return "", err
	}
	defer func() {
		if err := exec.Command("docker", "rmi", "-f", agentTag).Run(); err != nil {
			slog.Debug("iso: failed to remove transient WinPE agent image", "tag", agentTag, "error", err)
		}
	}()

	// Step 3: Use oscdimg inside the agent container to produce the ISO.
	isoName := "kompakt-agent-winpe"
	if n := sanitizeName(req.CustomName); n != "" {
		isoName = n
	}
	finalName := isoName + "-" + time.Now().UTC().Format("20060102-150405") + ".iso"
	finalPath := filepath.Join(absOutputDir, finalName)

	if err := buildWinPEISO(ctx, agentTag, absOutputDir, finalName, onLog); err != nil {
		return "", err
	}

	onLog("ISO ready: " + finalName)
	return finalPath, nil
}

// ensureWinPEImage builds the base WinPE image (kompakt-winpe-base:local) if it
// doesn't exist or the Dockerfile has changed.
func ensureWinPEImage(ctx context.Context, onLog func(string)) error {
	onLog("Checking WinPE base image…")

	ctxDir, err := os.MkdirTemp("", "kompakt-winpe-ctx-*")
	if err != nil {
		return fmt.Errorf("creating context dir: %w", err)
	}
	defer os.RemoveAll(ctxDir)

	if err := extractFS(dockerFS, "docker", ctxDir); err != nil {
		return fmt.Errorf("extracting embedded context: %w", err)
	}

	winpeDockerfile := filepath.Join(ctxDir, "winpe.Dockerfile")
	if _, err := os.Stat(winpeDockerfile); err != nil {
		return fmt.Errorf("winpe.Dockerfile not found in embedded context: %w", err)
	}

	cmd := exec.CommandContext(ctx, "docker", "build",
		"--isolation", "process",
		"-t", winpeBaseTag,
		"-f", winpeDockerfile,
		ctxDir,
	)
	return streamCommand(cmd, onLog)
}

// buildWinPEAgentImage creates a thin image on top of winpeBaseTag that layers
// the agent binary and a static runtime config.
func buildWinPEAgentImage(ctx context.Context, tag string, req BuildRequest, onLog func(string)) error {
	onLog("Layering agent binary into WinPE image…")

	buildDir, err := os.MkdirTemp("", "kompakt-winpe-agent-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(buildDir)

	// Copy the Windows agent binary.
	agentDst := filepath.Join(buildDir, "kompakt-agent.exe")
	if err := copyFile(req.AgentBinaryPath, agentDst, 0o755); err != nil {
		return fmt.Errorf("copying agent binary: %w", err)
	}

	// Write agent config.
	tlsStr := "false"
	if req.TLSSkipVerify {
		tlsStr = "true"
	}
	staticCfg := fmt.Sprintf(
		"server_url: %q\nregistration_secret: %q\ntls_skip_verify: %s\n"+
			"state_file: X:\\kompakt\\state.json\nwork_dir: X:\\kompakt\\work\n"+
			"reconnect_interval_seconds: 10\n",
		req.ServerURL, req.SecretValue, tlsStr,
	)
	if err := os.WriteFile(filepath.Join(buildDir, "agent.config.yml"), []byte(staticCfg), 0o644); err != nil {
		return err
	}

	// Write a Dockerfile that layers agent onto the base.
	dockerfile := fmt.Sprintf(
		"FROM %s\n"+
			"COPY kompakt-agent.exe C:\\\\Windows\\\\System32\\\\kompakt-agent.exe\n"+
			"COPY agent.config.yml C:\\\\kompakt\\\\agent.config.yml\n",
		winpeBaseTag,
	)
	if req.ExtraDockerfileInstructions != "" {
		dockerfile += req.ExtraDockerfileInstructions + "\n"
	}
	if err := os.WriteFile(filepath.Join(buildDir, "Dockerfile"), []byte(dockerfile), 0o644); err != nil {
		return err
	}

	cmd := exec.CommandContext(ctx, "docker", "build",
		"--isolation", "process",
		"-t", tag,
		buildDir,
	)
	return streamCommand(cmd, onLog)
}

// buildWinPEISO runs oscdimg inside the agent container to produce a bootable ISO.
func buildWinPEISO(ctx context.Context, agentTag, outputDir, isoName string, onLog func(string)) error {
	onLog("Building WinPE ISO…")

	// The winpe.Dockerfile sets the entrypoint to a build script that:
	// 1. Runs copype.cmd to create a WinPE working set.
	// 2. Injects the agent binary and config.
	// 3. Runs oscdimg to write the ISO to C:\output\<isoName>.
	// We mount absOutputDir to C:\output inside the container.
	cmd := exec.CommandContext(ctx, "docker", "run", "--rm",
		"--isolation", "process",
		"-v", outputDir+":C:\\output",
		"-e", "ISO_NAME="+isoName,
		agentTag,
		"cmd", "/C", strings.Join([]string{
			// Create WinPE working directory.
			`copype.cmd amd64 C:\winpe_amd64`,
			// Copy agent from System32 (already in the image layer) into the WinPE mount.
			`&&`, `mkdir C:\winpe_amd64\mount\kompakt`,
			`&&`, `copy C:\Windows\System32\kompakt-agent.exe C:\winpe_amd64\mount\Windows\System32\kompakt-agent.exe`,
			`&&`, `copy C:\kompakt\agent.config.yml C:\winpe_amd64\mount\kompakt\agent.config.yml`,
			// Write startnet.cmd to auto-start the agent after wpeinit.
			`&&`, `echo wpeinit > C:\winpe_amd64\mount\Windows\System32\startnet.cmd`,
			`&&`, `echo start /b kompakt-agent.exe -config C:\kompakt\agent.config.yml >> C:\winpe_amd64\mount\Windows\System32\startnet.cmd`,
			// Commit the WIM.
			`&&`, `Dism /Commit-Wim /WimFile:C:\winpe_amd64\media\sources\boot.wim`,
			// Build the ISO.
			`&&`, fmt.Sprintf(
				`oscdimg -n -m -bc:\winpe_amd64\fwfiles\etfsboot.com C:\winpe_amd64\media C:\output\%s`,
				isoName,
			),
		}, " "),
	)
	return streamCommand(cmd, onLog)
}

// detectDockerMode queries `docker info` to determine whether Docker is in
// "linux" or "windows" container mode. Returns "" on failure.
func detectDockerMode() (string, error) {
	out, err := exec.Command("docker", "info", "--format", "{{.OSType}}").Output()
	if err != nil {
		return "", fmt.Errorf("docker info: %w", err)
	}
	return strings.TrimSpace(strings.ToLower(string(out))), nil
}

// FindWindowsAgentBinary locates kompakt-agent.exe for WinPE builds.
func FindWindowsAgentBinary() (string, error) {
	var candidates []string
	if exe, err := os.Executable(); err == nil {
		candidates = append(candidates, filepath.Join(filepath.Dir(exe), "kompakt-agent.exe"))
	}
	candidates = append(candidates,
		`C:\Program Files\kompakt\kompakt-agent.exe`,
		`C:\ProgramData\kompakt\kompakt-agent.exe`,
	)
	for _, c := range candidates {
		if fi, err := os.Stat(c); err == nil && !fi.IsDir() {
			return c, nil
		}
	}
	return "", fmt.Errorf("kompakt-agent.exe not found; install it next to kompakt.exe")
}
