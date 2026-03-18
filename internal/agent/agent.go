// Package agent implements the kompakt deployment agent.
package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gorilla/websocket"
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
	cfg        *common.AgentConfig
	state      localState
	httpClient *http.Client
}

// New creates a new Agent with sensible defaults applied.
func New(cfg *common.AgentConfig) *Agent {
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
	return &Agent{
		cfg:        cfg,
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
}

// Run starts the agent: registers if needed, then connects and reconnects
// via WebSocket until ctx is cancelled.
func (c *Agent) Run(ctx context.Context) error {
	if err := c.loadState(); err != nil {
		log.Printf("warn: could not load local state: %v", err)
	}

	interval := time.Duration(c.cfg.ReconnectIntervalSeconds) * time.Second
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := c.register(ctx); err != nil {
			return fmt.Errorf("registration: %w", err)
		}
		log.Printf("connecting to %s", c.cfg.ServerURL)
		if err := c.connect(ctx); err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			if errors.Is(err, errTokenRejected) {
				// Token was cleared; re-register immediately without sleeping.
				continue
			}
			log.Printf("connection lost: %v — retrying in %s", err, interval)
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
	pf := identity.Collect(c.cfg.Platform)
	req := common.RegistrationRequest{
		Platform:           common.PlatformType(pf.Platform),
		Hostname:           pf.Hostname,
		Model:              pf.Model,
		Vendor:             pf.Vendor,
		RegistrationSecret: c.cfg.RegistrationSecret,
		Metadata:           pf.Metadata,
	}
	var resp common.RegistrationResponse
	if err := c.post(ctx, "/api/v1/register", req, &resp, ""); err != nil {
		return err
	}
	c.state.AgentID = resp.AgentID
	c.state.Token = resp.Token
	log.Printf("registered as agent %s", c.state.AgentID)
	return c.saveState()
}

// ── WebSocket connection ──────────────────────────────────────────────────────

func (c *Agent) connect(ctx context.Context) error {
	wsURL := wsEndpoint(c.cfg.ServerURL, c.state.Token)
	dialer := websocket.Dialer{HandshakeTimeout: 10 * time.Second}
	ws, resp, err := dialer.DialContext(ctx, wsURL, nil)
	if err != nil {
		if resp != nil && resp.StatusCode == http.StatusUnauthorized {
			log.Printf("token rejected by server, will re-register")
			c.state.Token = ""
			c.state.AgentID = ""
			_ = c.saveState()
			return errTokenRejected
		}
		return fmt.Errorf("dial: %w", err)
	}
	defer func() { _ = ws.Close() }()
	writer := newWSOutboundWriter(ws)
	defer writer.Stop()

	log.Printf("WebSocket connected")

	ws.SetPingHandler(func(appData string) error {
		// Reset the read deadline so the server's periodic pings keep the
		// connection alive on our side too.
		_ = ws.SetReadDeadline(time.Now().Add(90 * time.Second))
		return ws.WriteControl(websocket.PongMessage, nil, time.Now().Add(5*time.Second))
	})

	// Unblock ReadMessage when the context is cancelled.
	go func() {
		<-ctx.Done()
		// Force any blocking read/write operations to return immediately.
		_ = ws.SetReadDeadline(time.Now())
		_ = ws.SetWriteDeadline(time.Now())
		_ = ws.Close()
	}()

	for {
		if err := ws.SetReadDeadline(time.Now().Add(90 * time.Second)); err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			return fmt.Errorf("set read deadline: %w", err)
		}
		_, data, err := ws.ReadMessage()
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			return fmt.Errorf("read: %w", err)
		}

		var msg common.WSMessage
		if err := json.Unmarshal(data, &msg); err != nil {
			log.Printf("warn: invalid WS message: %v", err)
			continue
		}

		if msg.Type == common.WSMsgPlaybook {
			raw, _ := json.Marshal(msg.Data)
			var pd common.WSPlaybookData
			if err := json.Unmarshal(raw, &pd); err != nil {
				log.Printf("warn: invalid playbook message: %v", err)
				continue
			}
			c.runPlaybook(ctx, writer, &pd)
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

func (c *Agent) runPlaybook(ctx context.Context, writer *WSOutboundWriter, pd *common.WSPlaybookData) {
	pb := pd.Playbook
	deploymentID := pd.DeploymentID
	log.Printf("starting playbook %q (deployment %s, resume step %d)", pb.Name, deploymentID, pd.ResumeStepIndex)

	flat := common.FlattenPlaybook(pb)

	for _, fs := range flat {
		if fs.GlobalIndex < pd.ResumeStepIndex {
			continue // already completed before last reboot
		}

		stepName := fs.Name
		if stepName == "" {
			stepName = fmt.Sprintf("step-%d", fs.GlobalIndex)
		}
		log.Printf("[%s / %s] starting", fs.JobName, stepName)

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
			log.Printf("[%s] %s", level, msg)
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
				log.Printf("error: reboot failed: %v", err)
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

		log.Printf("[%s / %s] completed", fs.JobName, stepName)
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

	log.Printf("playbook %q completed successfully", pb.Name)
	writer.Send(common.WSMessage{
		Type:      common.WSMsgDeployDone,
		Timestamp: time.Now(),
		Data:      common.WSStepData{DeploymentID: deploymentID},
	})
}

type WSOutboundWriter struct {
	ch   chan common.WSMessage
	stop chan struct{}
	done chan struct{}
}

func newWSOutboundWriter(ws *websocket.Conn) *WSOutboundWriter {
	w := &WSOutboundWriter{
		ch:   make(chan common.WSMessage, 256),
		stop: make(chan struct{}),
		done: make(chan struct{}),
	}

	go func() {
		defer close(w.done)
		for {
			select {
			case <-w.stop:
				return
			case msg := <-w.ch:
				data, err := json.Marshal(msg)
				if err != nil {
					log.Printf("warn: marshal WS message: %v", err)
					continue
				}
				if err := ws.SetWriteDeadline(time.Now().Add(10 * time.Second)); err != nil {
					log.Printf("warn: WS set write deadline: %v", err)
					return
				}
				if err := ws.WriteMessage(websocket.TextMessage, data); err != nil {
					log.Printf("warn: WS write: %v", err)
					return
				}
			}
		}
	}()

	return w
}

func (w *WSOutboundWriter) Send(msg common.WSMessage) {
	// Fast-path check: if the writer is already closed, don't block.
	select {
	case <-w.done:
		log.Printf("warn: dropping WS message %q: writer closed", msg.Type)
		return
	default:
	}

	// Apply backpressure: block until the message is enqueued or the writer closes.
	select {
	case <-w.done:
		log.Printf("warn: dropping WS message %q: writer closed", msg.Type)
	case w.ch <- msg:
	}
}

func (w *WSOutboundWriter) Stop() {
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

// ── HTTP helpers (registration only) ─────────────────────────────────────────

func (c *Agent) post(ctx context.Context, path string, body, out any, token string) error {
	b, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.cfg.ServerURL+path, bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode >= 400 {
		return fmt.Errorf("HTTP %d from %s", resp.StatusCode, path)
	}
	if out != nil {
		return json.NewDecoder(resp.Body).Decode(out)
	}
	return nil
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
