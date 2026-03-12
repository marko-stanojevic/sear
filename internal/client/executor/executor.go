// Package executor runs individual playbook steps on the local machine.
package executor

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/marko-stanojevic/sear/internal/common"
)

// Logger is a function the executor calls to emit log lines.
type Logger func(level common.LogLevel, msg string)

// Result is returned after running a step.
type Result struct {
	Err         error
	NeedsReboot bool
	RebootReason string
}

// RunStep executes a single playbook step.
//   - secrets contains the server-resolved secrets map (key→value).
//   - artifactsBaseURL is the daemon's /artifacts endpoint.
//   - token is the client JWT for authenticated artifact requests.
func RunStep(
	ctx context.Context,
	step common.Step,
	secrets map[string]string,
	artifactsBaseURL, token string,
	log Logger,
) Result {
	// Apply step-level timeout if configured.
	if step.TimeoutMinutes > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, time.Duration(step.TimeoutMinutes)*time.Minute)
		defer cancel()
	}

	switch {
	case step.Uses == "reboot":
		reason := step.With["reason"]
		if reason == "" {
			reason = "playbook reboot step"
		}
		return Result{NeedsReboot: true, RebootReason: reason}

	case step.Uses == "download-artifact":
		return runDownloadArtifact(ctx, step, artifactsBaseURL, token, log)

	case step.Uses == "upload-artifact":
		return runUploadArtifact(ctx, step, artifactsBaseURL, token, log)

	case step.Uses == "upload-logs":
		// Logs are streamed continuously over WebSocket; this is a no-op.
		log(common.LogLevelInfo, "upload-logs: logs are already streamed in real time")
		return Result{}

	case step.Run != "":
		// Resolve ${{ secrets.NAME }} references in the step's env block.
		resolvedEnv := common.ResolveEnvSecrets(step.Env, secrets)
		return runShell(ctx, step, secrets, resolvedEnv, log)

	default:
		return Result{Err: fmt.Errorf("unknown step: name=%q uses=%q", step.Name, step.Uses)}
	}
}

// ── Shell execution ───────────────────────────────────────────────────────────

func runShell(ctx context.Context, step common.Step, secrets, stepEnv map[string]string, log Logger) Result {
	shell := strings.ToLower(step.Shell)
	if shell == "" {
		shell = "bash"
	}

	var cmd *exec.Cmd
	switch shell {
	case "bash":
		cmd = exec.CommandContext(ctx, "bash", "-euo", "pipefail", "-c", step.Run)
	case "sh":
		cmd = exec.CommandContext(ctx, "sh", "-e", "-c", step.Run)
	case "pwsh", "powershell":
		cmd = exec.CommandContext(ctx, "pwsh", "-NonInteractive", "-Command", step.Run)
	case "cmd":
		cmd = exec.CommandContext(ctx, "cmd.exe", "/C", step.Run)
	case "python", "python3":
		cmd = exec.CommandContext(ctx, "python3", "-c", step.Run)
	default:
		cmd = exec.CommandContext(ctx, "bash", "-euo", "pipefail", "-c", step.Run)
	}

	// Build environment: inherit the current process env, overlay secrets
	// (so they're available as $VAR_NAME), then overlay step-specific env.
	env := os.Environ()
	for k, v := range secrets {
		env = append(env, k+"="+v)
	}
	for k, v := range stepEnv {
		env = append(env, k+"="+v)
	}
	cmd.Env = env

	// Pipe stdout and stderr together and stream line-by-line to the logger.
	pr, pw, err := os.Pipe()
	if err != nil {
		return Result{Err: fmt.Errorf("creating output pipe: %w", err)}
	}
	cmd.Stdout = pw
	cmd.Stderr = pw

	if err := cmd.Start(); err != nil {
		pw.Close()
		pr.Close()
		return Result{Err: fmt.Errorf("starting shell: %w", err)}
	}
	pw.Close()

	// Stream output to the logger.
	go func() {
		buf := make([]byte, 4096)
		var leftover string
		for {
			n, err := pr.Read(buf)
			if n > 0 {
				chunk := leftover + string(buf[:n])
				lines := strings.Split(chunk, "\n")
				for _, line := range lines[:len(lines)-1] {
					log(common.LogLevelInfo, line)
				}
				leftover = lines[len(lines)-1]
			}
			if err != nil {
				if leftover != "" {
					log(common.LogLevelInfo, leftover)
				}
				break
			}
		}
	}()

	if err := cmd.Wait(); err != nil {
		return Result{Err: fmt.Errorf("shell exited with error: %w", err)}
	}
	return Result{}
}

// ── Artifact actions ──────────────────────────────────────────────────────────

func runDownloadArtifact(ctx context.Context, step common.Step, artifactsBaseURL, token string, log Logger) Result {
	name := step.With["artifact"]
	if name == "" {
		name = step.With["name"] // allow either key
	}
	dest := step.With["path"]
	if name == "" {
		return Result{Err: fmt.Errorf("download-artifact: 'artifact' is required")}
	}
	if dest == "" {
		dest = "."
	}
	url := strings.TrimRight(artifactsBaseURL, "/") + "/" + name
	log(common.LogLevelInfo, fmt.Sprintf("downloading artifact %q → %s", name, dest))

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return Result{Err: fmt.Errorf("download-artifact: %w", err)}
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	client := &http.Client{Timeout: 10 * time.Minute}
	resp, err := client.Do(req)
	if err != nil {
		return Result{Err: fmt.Errorf("download-artifact: %w", err)}
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return Result{Err: fmt.Errorf("download-artifact: server returned %d", resp.StatusCode)}
	}

	if err := os.MkdirAll(dest, 0o750); err != nil {
		return Result{Err: fmt.Errorf("download-artifact: mkdir %s: %w", dest, err)}
	}
	outPath := filepath.Join(dest, name)
	f, err := os.Create(outPath)
	if err != nil {
		return Result{Err: fmt.Errorf("download-artifact: create %s: %w", outPath, err)}
	}
	defer f.Close()
	written, err := io.Copy(f, resp.Body)
	if err != nil {
		return Result{Err: fmt.Errorf("download-artifact: write: %w", err)}
	}
	log(common.LogLevelInfo, fmt.Sprintf("artifact saved to %s (%d bytes)", outPath, written))
	return Result{}
}

func runUploadArtifact(ctx context.Context, step common.Step, artifactsBaseURL, token string, log Logger) Result {
	name := step.With["artifact"]
	if name == "" {
		name = step.With["name"]
	}
	src := step.With["path"]
	if name == "" || src == "" {
		return Result{Err: fmt.Errorf("upload-artifact: 'artifact' and 'path' are required")}
	}
	log(common.LogLevelInfo, fmt.Sprintf("uploading artifact %q from %s", name, src))

	f, err := os.Open(src)
	if err != nil {
		return Result{Err: fmt.Errorf("upload-artifact: open %s: %w", src, err)}
	}
	defer f.Close()

	url := strings.TrimRight(artifactsBaseURL, "/") + "?name=" + name
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, f)
	if err != nil {
		return Result{Err: fmt.Errorf("upload-artifact: build request: %w", err)}
	}
	req.Header.Set("Content-Type", "application/octet-stream")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	httpClient := &http.Client{Timeout: 10 * time.Minute}
	resp, err := httpClient.Do(req)
	if err != nil {
		return Result{Err: fmt.Errorf("upload-artifact: %w", err)}
	}
	defer resp.Body.Close()
	var body bytes.Buffer
	_, _ = io.Copy(&body, resp.Body)
	if resp.StatusCode != http.StatusCreated {
		return Result{Err: fmt.Errorf("upload-artifact: server returned %d: %s", resp.StatusCode, body.String())}
	}
	log(common.LogLevelInfo, "artifact uploaded successfully")
	return Result{}
}
