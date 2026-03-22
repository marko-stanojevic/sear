// Package agent implements the kompakt deployment agent.
package agent

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/coder/websocket"
	"github.com/marko-stanojevic/kompakt/internal/agent/executor"
	"github.com/marko-stanojevic/kompakt/internal/agent/identity"
	"github.com/marko-stanojevic/kompakt/internal/common"
)

// errTokenRejected is returned by connect when the server responds with 401,
// signalling that the caller should re-register immediately without sleeping.
var errTokenRejected = errors.New("token rejected by server")

// localState is persisted to disk so the agent can resume after a reboot.
type localState struct {
	AgentID string `json:"agent_id"`
	Token   string `json:"token"`
}

// Agent is the kompakt deployment agent.
type Agent struct {
	platformInfo *identity.PlatformInfo
	cfg          *common.AgentConfig
	state        localState
	tlsCfg       *tls.Config
	httpClient   *http.Client
}

// New creates a new Agent with sensible defaults applied.
// Returns an error if the TLS configuration is invalid (e.g. CA file not found).
func New(cfg *common.AgentConfig) (*Agent, error) {
	if cfg.ReconnectIntervalSeconds == 0 {
		cfg.ReconnectIntervalSeconds = 10
	}
	if cfg.LogBatchSize == 0 {
		cfg.LogBatchSize = 100
	}
	if cfg.StateFile == "" {
		cfg.StateFile = defaultStateFile()
	}
	if cfg.WorkDir == "" {
		cfg.WorkDir = defaultWorkDir()
	}
	// Set up HTTP client with optional TLS verification skip
	var httpClient *http.Client
	if cfg.DisableTLSVerification {
		// #nosec G402 -- This is only enabled for trusted/dev environments
		httpClient = &http.Client{
			Timeout: 30 * time.Second,
			Transport: &http.Transport{
				//nolint:gosec // This is only enabled for trusted/dev environments
				TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
			},
		}
	} else {
		httpClient = &http.Client{Timeout: 30 * time.Second}
	}
	return &Agent{
		cfg:        cfg,
		httpClient: httpClient,
	}

	pf := identity.Collect()
	return &Agent{
		cfg:          cfg,
		platformInfo: &pf,
		tlsCfg:       tlsCfg,
		httpClient:   httpClient,
	}, nil
}

// Run starts the agent: registers if needed, then connects and reconnects
// via WebSocket until ctx is cancelled.
func (c *Agent) Run(ctx context.Context) error {

	if err := c.loadState(); err != nil {
		slog.Warn("could not load local state", "error", err)
	}

	// Print platform info on startup (split for readability)
	slog.Info("Agent", "hostname", c.platformInfo.Hostname, "model", c.platformInfo.Model, "vendor", c.platformInfo.Vendor)
	slog.Info("Agent (metadata)", "metadata", c.platformInfo.Metadata)

	interval := time.Duration(c.cfg.ReconnectIntervalSeconds) * time.Second
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := c.register(ctx); err != nil {
			return fmt.Errorf("registration: %w", err)
		}
		slog.Info("connecting to server", "url", c.cfg.ServerURL)
		if err := c.connect(ctx); err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			if errors.Is(err, errTokenRejected) {
				// Token was cleared; re-register immediately without sleeping.
				continue
			}
			slog.Warn("connection lost, retrying", "error", err, "retry_in", interval)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(interval):
		}
	}
}

// ── Registration ─────────────────────────────────────────────────────────────

func (c *Agent) register(ctx context.Context) error {
	pf := *c.platformInfo
	req := common.RegistrationRequest{
		Platform:           common.PlatformType(pf.Platform),
		Hostname:           pf.Hostname,
		Model:              pf.Model,
		Vendor:             pf.Vendor,
		RegistrationSecret: c.cfg.RegistrationSecret,
		Metadata:           pf.Metadata,
		Shells:             identity.DetectShells(),
	}

	// Send registration request to server with context
	regURL := strings.TrimRight(c.cfg.ServerURL, "/") + "/api/v1/register"
	body, err := json.Marshal(req)
	if err != nil {
		slog.Error("failed to marshal registration request", "error", err)
		return err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, regURL, bytes.NewReader(body))
	if err != nil {
		slog.Error("failed to create registration request", "error", err)
		return err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		slog.Error("registration request failed", "error", err)
		return err
	}
	defer func() {
		_ = resp.Body.Close()
	}()
	if resp.StatusCode != http.StatusOK {
		slog.Error("registration failed", "status", resp.Status)
		return fmt.Errorf("registration failed: %s", resp.Status)
	}
	var regResp common.RegistrationResponse
	if err := json.NewDecoder(resp.Body).Decode(&regResp); err != nil {
		slog.Error("failed to decode registration response", "error", err)
		return err
	}
	c.state.AgentID = regResp.AgentID
	c.state.Token = regResp.Token
	slog.Info("registered", "agent_id", c.state.AgentID)
	return c.saveState()
}

// ── WebSocket connection ──────────────────────────────────────────────────────

func (c *Agent) connect(ctx context.Context) error {
	wsURL := wsEndpoint(c.cfg.ServerURL, c.state.Token)
	dialer := websocket.Dialer{HandshakeTimeout: 10 * time.Second}
	// If TLS verification is disabled, set InsecureSkipVerify for the WebSocket dialer
	if c.cfg.DisableTLSVerification {
		dialer.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
	}
	ws, resp, err := dialer.DialContext(ctx, wsURL, nil)
	if err != nil {
		if resp != nil && resp.StatusCode == http.StatusUnauthorized {
			slog.Warn("token rejected by server, will re-register")
			c.state.Token = ""
			c.state.AgentID = ""
			_ = c.saveState()
			return errTokenRejected
		}
		return fmt.Errorf("dial: %w", err)
	}
	defer ws.CloseNow()
	writer := newMessageWriter(ws)
	defer writer.Stop()

	slog.Info("WebSocket connected")

	// coder/websocket automatically responds to pings with pongs.
	// The read loop unblocks naturally when ctx is cancelled.
	for {
		_, data, err := ws.Read(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			return fmt.Errorf("read: %w", err)
		}

		var msg common.WSMessage
		if err := json.Unmarshal(data, &msg); err != nil {
			slog.Warn("invalid WS message", "error", err)
			continue
		}

		switch msg.Type {
		case common.WSMsgPlaybook:
			raw, _ := json.Marshal(msg.Data)
			var pd common.WSPlaybookData
			if err := json.Unmarshal(raw, &pd); err != nil {
				slog.Warn("invalid playbook message", "error", err)
				continue
			}
			go c.runPlaybook(ctx, writer, &pd)

		case common.WSMsgCommand:
			raw, _ := json.Marshal(msg.Data)
			var cd common.WSCommandData
			if err := json.Unmarshal(raw, &cd); err != nil {
				slog.Warn("invalid command message", "error", err)
				continue
			}
			go c.runCommand(ctx, writer, &cd)
		}
	}
}

// wsEndpoint builds the WebSocket URL, converting http(s) → ws(s) and
// appending the JWT as a query parameter.
func wsEndpoint(serverURL, token string) string {
	u, err := url.Parse(serverURL)
	if err != nil {
		return serverURL
	}
	switch u.Scheme {
	case "https":
		u.Scheme = "wss"
	default:
		u.Scheme = "ws"
	}
	u.Path = strings.TrimRight(u.Path, "/") + "/api/v1/ws"
	q := u.Query()
	q.Set("token", token)
	u.RawQuery = q.Encode()
	return u.String()
}

// ── Playbook execution ────────────────────────────────────────────────────────

func (c *Agent) runPlaybook(ctx context.Context, writer *MessageWriter, pd *common.WSPlaybookData) {
	pb := pd.Playbook
	deploymentID := pd.DeploymentID
	slog.Info("starting playbook", "name", pb.Name, "deployment_id", deploymentID, "resume_step", pd.ResumeStepIndex)

	flat := common.FlattenPlaybook(pb)

	for _, fs := range flat {
		if fs.GlobalIndex < pd.ResumeStepIndex {
			continue // already completed before last reboot
		}

		stepName := fs.Name
		if stepName == "" {
			stepName = fmt.Sprintf("step-%d", fs.GlobalIndex)
		}
		slog.Info("step starting", "job", fs.JobName, "step", stepName)

		writer.Send(common.WSMessage{
			Type:      common.WSMsgStepStart,
			Timestamp: time.Now(),
			Data: common.WSStepData{
				DeploymentID: deploymentID,
				JobName:      fs.JobName,
				StepName:     stepName,
				StepIndex:    fs.GlobalIndex,
			},
		})

		logger := func(level common.LogLevel, msg string) {
			switch level {
			case common.LogLevelError:
				slog.Error(msg)
			case common.LogLevelWarn:
				slog.Warn(msg)
			default:
				slog.Info(msg)
			}
			writer.Send(common.WSMessage{
				Type:      common.WSMsgLog,
				Timestamp: time.Now(),
				Data: common.WSLogData{
					DeploymentID: deploymentID,
					JobName:      fs.JobName,
					StepIndex:    fs.GlobalIndex,
					Level:        level,
					Message:      msg,
				},
			})
		}

		result := executor.RunStep(ctx, fs.Step, pd.Secrets, pd.ArtifactsBaseURL, c.state.Token, logger)

		if result.NeedsReboot {
			resumeAt := fs.GlobalIndex + 1
			logger(common.LogLevelInfo, fmt.Sprintf("step %q requested reboot (reason: %s); will resume at step %d",
				stepName, result.RebootReason, resumeAt))
			writer.Send(common.WSMessage{
				Type:      common.WSMsgReboot,
				Timestamp: time.Now(),
				Data: common.WSRebootData{
					DeploymentID:    deploymentID,
					ResumeStepIndex: resumeAt,
					Reason:          result.RebootReason,
				},
			})
			// Give the server a moment to persist the state before rebooting.
			time.Sleep(500 * time.Millisecond)
			if err := rebootOS(); err != nil {
				slog.Error("reboot failed", "error", err)
			}
			return
		}

		if result.Err != nil {
			if fs.ContinueOnError {
				logger(common.LogLevelWarn, fmt.Sprintf("step %q failed (continue-on-error): %v", stepName, result.Err))
				writer.Send(common.WSMessage{
					Type:      common.WSMsgStepComplete,
					Timestamp: time.Now(),
					Data: common.WSStepData{
						DeploymentID: deploymentID,
						JobName:      fs.JobName,
						StepName:     stepName,
						StepIndex:    fs.GlobalIndex,
					},
				})
				continue
			}
			errMsg := fmt.Sprintf("step %q failed: %v", stepName, result.Err)
			logger(common.LogLevelError, errMsg)
			writer.Send(common.WSMessage{
				Type:      common.WSMsgDeployFailed,
				Timestamp: time.Now(),
				Data: common.WSStepData{
					DeploymentID: deploymentID,
					JobName:      fs.JobName,
					StepName:     stepName,
					StepIndex:    fs.GlobalIndex,
					Error:        errMsg,
				},
			})
			return
		}

		slog.Info("step completed", "job", fs.JobName, "step", stepName)
		writer.Send(common.WSMessage{
			Type:      common.WSMsgStepComplete,
			Timestamp: time.Now(),
			Data: common.WSStepData{
				DeploymentID: deploymentID,
				JobName:      fs.JobName,
				StepName:     stepName,
				StepIndex:    fs.GlobalIndex,
			},
		})
	}

	slog.Info("playbook completed", "name", pb.Name, "deployment_id", deploymentID)
	writer.Send(common.WSMessage{
		Type:      common.WSMsgDeployDone,
		Timestamp: time.Now(),
		Data:      common.WSStepData{DeploymentID: deploymentID},
	})
}

// ── Command execution ─────────────────────────────────────────────────────────

func (c *Agent) runCommand(ctx context.Context, writer *MessageWriter, cd *common.WSCommandData) {
	shell := strings.ToLower(cd.Shell)
	if shell == "" {
		shell = defaultShell()
	}

	if c.cfg.WorkDir != "" {
		if err := os.MkdirAll(c.cfg.WorkDir, 0o750); err != nil {
			slog.Warn("could not create work dir for remote command", "path", c.cfg.WorkDir, "error", err)
		}
	}

	// Build the command. For cmd.exe, multi-line scripts require a temp batch
	// file because cmd.exe /C only executes the first line of a multi-line string.
	var cmd *exec.Cmd
	var tmpFile string // path of temp file to clean up after execution
	switch shell {
	case "bash":
		cmd = exec.CommandContext(ctx, "bash", "-c", cd.Command)
	case "sh":
		cmd = exec.CommandContext(ctx, "sh", "-c", cd.Command)
	case "pwsh":
		cmd = exec.CommandContext(ctx, "pwsh", "-NonInteractive", "-Command", cd.Command)
	case "powershell":
		cmd = exec.CommandContext(ctx, "powershell.exe", "-NonInteractive", "-Command", cd.Command)
	case "cmd":
		if strings.Contains(cd.Command, "\n") {
			f, err := os.CreateTemp("", "kompakt-*.bat")
			if err != nil {
				writer.Send(common.WSMessage{
					Type:      common.WSMsgCommandCompleted,
					Timestamp: time.Now(),
					Data:      common.WSCommandStatus{CmdID: cd.CmdID, ExitCode: 1, Error: "temp file: " + err.Error()},
				})
				return
			}
			tmpFile = f.Name()
			_, _ = f.WriteString("@echo off\r\n" + strings.ReplaceAll(cd.Command, "\n", "\r\n"))
			_ = f.Close()
			cmd = exec.CommandContext(ctx, "cmd.exe", "/C", tmpFile)
		} else {
			cmd = exec.CommandContext(ctx, "cmd.exe", "/C", cd.Command)
		}
	default:
		cmd = exec.CommandContext(ctx, "bash", "-c", cd.Command)
	}
	if tmpFile != "" {
		defer func() { _ = os.Remove(tmpFile) }()
	}

	if c.cfg.WorkDir != "" {
		cmd.Dir = c.cfg.WorkDir
	}

	sendDone := func(exitCode int, errMsg string) {
		writer.Send(common.WSMessage{
			Type:      common.WSMsgCommandCompleted,
			Timestamp: time.Now(),
			Data:      common.WSCommandStatus{CmdID: cd.CmdID, ExitCode: exitCode, Error: errMsg},
		})
	}

	pr, pw, err := os.Pipe()
	if err != nil {
		sendDone(1, "pipe: "+err.Error())
		return
	}
	cmd.Stdout = pw
	cmd.Stderr = pw

	if err := cmd.Start(); err != nil {
		_ = pw.Close()
		_ = pr.Close()
		sendDone(1, "start: "+err.Error())
		return
	}
	_ = pw.Close()

	executor.DrainPipeLines(pr, func(line string) {
		writer.Send(common.WSMessage{
			Type:      common.WSMsgCommandStream,
			Timestamp: time.Now(),
			Data:      common.WSCommandChunk{CmdID: cd.CmdID, Output: executor.SanitizeLine(line)},
		})
	})
	_ = pr.Close()

	exitCode := 0
	errMsg := ""
	if err := cmd.Wait(); err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			exitCode = exitErr.ExitCode()
		} else {
			exitCode = 1
			errMsg = err.Error()
		}
	}
	sendDone(exitCode, errMsg)
}

type MessageWriter struct {
	outbox chan common.WSMessage
	stop   chan struct{}
	done   chan struct{}
}

func newMessageWriter(ws *websocket.Conn) *MessageWriter {
	w := &MessageWriter{
		outbox: make(chan common.WSMessage, 256),
		stop: make(chan struct{}),
		done: make(chan struct{}),
	}

	go func() {
		defer close(w.done)
		for {
			select {
			case <-w.stop:
				return
			case msg := <-w.outbox:
				data, err := json.Marshal(msg)
				if err != nil {
					slog.Warn("failed to marshal WS message", "error", err)
					continue
				}
				writeCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				err = ws.Write(writeCtx, websocket.MessageText, data)
				cancel()
				if err != nil {
					slog.Warn("WS write failed", "error", err)
					return
				}
			}
		}
	}()

	return w
}

func (w *MessageWriter) Send(msg common.WSMessage) {
	// Fast-path check: if the writer is already closed, don't block.
	select {
	case <-w.done:
		slog.Warn("dropping WS message, writer closed", "type", msg.Type)
		return
	default:
	}

	// Apply backpressure: block until the message is enqueued or the writer closes.
	select {
	case <-w.done:
		slog.Warn("dropping WS message, writer closed", "type", msg.Type)
	case w.outbox <- msg:
	}
}

func (w *MessageWriter) Stop() {
	close(w.stop)
	<-w.done
}

// ── Local state ───────────────────────────────────────────────────────────────

func (c *Agent) loadState() error {
	data, err := os.ReadFile(c.cfg.StateFile)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	return json.Unmarshal(data, &c.state)
}

func (c *Agent) saveState() error {
	_ = os.MkdirAll(filepath.Dir(c.cfg.StateFile), 0o700)
	data, err := json.MarshalIndent(c.state, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(c.cfg.StateFile, data, 0o600)
}

// ── Portable default paths ────────────────────────────────────────────────────

// kompaktDir returns the .kompakt directory next to the running executable.
// Falls back to .kompakt in the current working directory if the executable
// path cannot be determined.
func kompaktDir() string {
	exe, err := os.Executable()
	if err != nil {
		return ".kompakt"
	}
	return filepath.Join(filepath.Dir(exe), ".kompakt")
}

func defaultStateFile() string {
	return filepath.Join(kompaktDir(), "state.json")
}

func defaultWorkDir() string {
	return filepath.Join(kompaktDir(), "work")
}
