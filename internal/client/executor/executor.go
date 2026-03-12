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

// Logger is a function that the executor calls to emit log lines.
type Logger func(level common.LogLevel, msg string)

// Result is returned after running a step.
type Result struct {
	Err       error
	NeedsReboot bool
}

// RunStep executes a single playbook step.
//
//   - env contains all variables (secrets + step.Env) available as
//     environment variables.
//   - artifactsBaseURL is the server's /artifacts endpoint.
func RunStep(ctx context.Context, step common.Step, env map[string]string, artifactsBaseURL, token string, log Logger) Result {
	switch {
	case step.Uses == "reboot":
		return Result{NeedsReboot: true}

	case step.Uses == "download-artifact":
		return runDownloadArtifact(ctx, step, artifactsBaseURL, token, log)

	case step.Uses == "upload-artifact":
		return runUploadArtifact(ctx, step, artifactsBaseURL, token, log)

	case step.Uses == "upload-logs":
		// upload-logs is handled by the client itself; nothing extra here.
		log(common.LogLevelInfo, "upload-logs: logs are streamed continuously")
		return Result{}

	case step.Run != "":
		return runShell(ctx, step, env, log)

	default:
		return Result{Err: fmt.Errorf("unknown step configuration: name=%q uses=%q", step.Name, step.Uses)}
	}
}

// runShell executes a shell script step.
func runShell(ctx context.Context, step common.Step, env map[string]string, log Logger) Result {
	shell := step.Shell
	if shell == "" {
		shell = "bash"
	}

	var cmd *exec.Cmd
	switch strings.ToLower(shell) {
	case "bash", "sh":
		cmd = exec.CommandContext(ctx, "bash", "-c", step.Run)
	case "pwsh", "powershell":
		cmd = exec.CommandContext(ctx, "pwsh", "-Command", step.Run)
	default:
		cmd = exec.CommandContext(ctx, "bash", "-c", step.Run)
	}

	// Build environment: inherit current env, then overlay secrets and step env.
	cmdEnv := os.Environ()
	for k, v := range env {
		cmdEnv = append(cmdEnv, k+"="+v)
	}
	for k, v := range step.Env {
		cmdEnv = append(cmdEnv, k+"="+v)
	}
	cmd.Env = cmdEnv

	// Capture stdout/stderr and stream to logger.
	pr, pw, _ := os.Pipe()
	cmd.Stdout = pw
	cmd.Stderr = pw

	if err := cmd.Start(); err != nil {
		return Result{Err: fmt.Errorf("starting shell: %w", err)}
	}
	pw.Close()

	go func() {
		buf := make([]byte, 4096)
		for {
			n, err := pr.Read(buf)
			if n > 0 {
				log(common.LogLevelInfo, string(buf[:n]))
			}
			if err != nil {
				break
			}
		}
	}()

	if err := cmd.Wait(); err != nil {
		return Result{Err: fmt.Errorf("shell exited: %w", err)}
	}
	return Result{}
}

// runDownloadArtifact downloads an artifact from the server.
func runDownloadArtifact(ctx context.Context, step common.Step, artifactsBaseURL, token string, log Logger) Result {
	name := step.With["name"]
	dest := step.With["path"]
	if name == "" {
		return Result{Err: fmt.Errorf("download-artifact: 'name' is required")}
	}
	if dest == "" {
		dest = "."
	}
	url := strings.TrimRight(artifactsBaseURL, "/") + "/" + name
	log(common.LogLevelInfo, fmt.Sprintf("downloading artifact %q → %s", name, dest))

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return Result{Err: fmt.Errorf("download-artifact: build request: %w", err)}
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	client := &http.Client{Timeout: 5 * time.Minute}
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
	if _, err := io.Copy(f, resp.Body); err != nil {
		return Result{Err: fmt.Errorf("download-artifact: write: %w", err)}
	}
	log(common.LogLevelInfo, fmt.Sprintf("artifact saved to %s", outPath))
	return Result{}
}

// runUploadArtifact uploads a file to the server's /artifacts endpoint.
func runUploadArtifact(ctx context.Context, step common.Step, artifactsBaseURL, token string, log Logger) Result {
	name := step.With["name"]
	src := step.With["path"]
	if name == "" || src == "" {
		return Result{Err: fmt.Errorf("upload-artifact: 'name' and 'path' are required")}
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
	httpClient := &http.Client{Timeout: 5 * time.Minute}
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
