package agent

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/marko-stanojevic/kompakt/internal/common"
)

// ── Shared test helpers ───────────────────────────────────────────────────────

func mustNew(t *testing.T, cfg *common.AgentConfig) *Agent {
	t.Helper()
	c, err := New(cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return c
}

// newTestWriter builds a MessageWriter with no backing goroutine or connection.
// Send calls enqueue into outbox; callers drain directly via drainMessages.
func newTestWriter() *MessageWriter {
	return &MessageWriter{
		outbox: make(chan common.WSMessage, 128),
		stop:   make(chan struct{}),
		done:   make(chan struct{}),
	}
}

func drainMessages(w *MessageWriter) []common.WSMessage {
	msgs := make([]common.WSMessage, 0, len(w.outbox))
	for len(w.outbox) > 0 {
		msgs = append(msgs, <-w.outbox)
	}
	return msgs
}

func hasMessageType(msgs []common.WSMessage, typ common.WSMessageType) bool {
	for _, m := range msgs {
		if m.Type == typ {
			return true
		}
	}
	return false
}

// ── WebSocket URL construction ────────────────────────────────────────────────

func TestWSEndpoint_HTTP(t *testing.T) {
	got := wsEndpoint("http://example.com/base", "tok-123")
	if !strings.HasPrefix(got, "ws://example.com/base/api/v1/ws?") {
		t.Fatalf("ws endpoint prefix mismatch: %s", got)
	}
	if !strings.Contains(got, "token=tok-123") {
		t.Fatalf("token query missing: %s", got)
	}
}

func TestWSEndpoint_HTTPS_ProducesWSS(t *testing.T) {
	got := wsEndpoint("https://example.com", "t")
	if !strings.HasPrefix(got, "wss://example.com/api/v1/ws?") {
		t.Fatalf("wss endpoint prefix mismatch: %s", got)
	}
}

func TestWSEndpoint_TrailingSlashIsNormalized(t *testing.T) {
	got := wsEndpoint("http://example.com/", "tok")
	if !strings.HasPrefix(got, "ws://example.com/api/v1/ws?") {
		t.Fatalf("trailing slash not normalized: %s", got)
	}
}

func TestWSEndpoint_PathInServerURLIsPreserved(t *testing.T) {
	got := wsEndpoint("http://example.com/prefix", "tok")
	if !strings.Contains(got, "/prefix/api/v1/ws") {
		t.Fatalf("base path not preserved: %s", got)
	}
}

// ── State persistence ─────────────────────────────────────────────────────────

func TestSaveLoadState_RoundTrip(t *testing.T) {
	stateFile := filepath.Join(t.TempDir(), "state.json")
	c := mustNew(t, &common.AgentConfig{StateFile: stateFile})
	c.state = localState{AgentID: "c1", Token: "t1"}

	if err := c.saveState(); err != nil {
		t.Fatalf("saveState: %v", err)
	}

	c2 := mustNew(t, &common.AgentConfig{StateFile: stateFile})
	if err := c2.loadState(); err != nil {
		t.Fatalf("loadState: %v", err)
	}
	if c2.state.AgentID != "c1" || c2.state.Token != "t1" {
		t.Fatalf("loaded state mismatch: %+v", c2.state)
	}
}

func TestSaveState_CreatesParentDirectory(t *testing.T) {
	stateFile := filepath.Join(t.TempDir(), "nested", "deep", "state.json")
	c := mustNew(t, &common.AgentConfig{StateFile: stateFile})
	c.state = localState{AgentID: "a1", Token: "t1"}

	if err := c.saveState(); err != nil {
		t.Fatalf("saveState with nested path: %v", err)
	}
	if _, err := os.Stat(stateFile); err != nil {
		t.Fatalf("state file not found: %v", err)
	}
}

func TestLoadState_MissingFileIsNotAnError(t *testing.T) {
	c := mustNew(t, &common.AgentConfig{StateFile: filepath.Join(t.TempDir(), "nonexistent.json")})
	if err := c.loadState(); err != nil {
		t.Fatalf("loadState on missing file should be no-op: %v", err)
	}
}

// ── Registration ──────────────────────────────────────────────────────────────

func TestRegister_Success(t *testing.T) {
	stateFile := filepath.Join(t.TempDir(), "state.json")

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/register" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(common.RegistrationResponse{
			AgentID: "agent-reg-1",
			Token:   "token-reg-1",
		})
	}))
	defer ts.Close()

	c := mustNew(t, &common.AgentConfig{
		ServerURL:          ts.URL,
		RegistrationSecret: "secret",
		StateFile:          stateFile,
	})

	if err := c.register(context.Background()); err != nil {
		t.Fatalf("register: %v", err)
	}
	if c.state.AgentID != "agent-reg-1" || c.state.Token != "token-reg-1" {
		t.Fatalf("unexpected state after register: %+v", c.state)
	}
	if _, err := os.Stat(stateFile); err != nil {
		t.Fatalf("state file not written: %v", err)
	}
}

func TestRegister_ServerError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "internal error", http.StatusInternalServerError)
	}))
	defer ts.Close()

	c := mustNew(t, &common.AgentConfig{
		ServerURL: ts.URL,
		StateFile: filepath.Join(t.TempDir(), "state.json"),
	})
	if err := c.register(context.Background()); err == nil {
		t.Fatal("expected error for 500 response")
	}
}

func TestRegister_InvalidJSONResponse(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("not-json{{{"))
	}))
	defer ts.Close()

	c := mustNew(t, &common.AgentConfig{
		ServerURL: ts.URL,
		StateFile: filepath.Join(t.TempDir(), "state.json"),
	})
	if err := c.register(context.Background()); err == nil {
		t.Fatal("expected error for invalid JSON response")
	}
}

func TestRegister_ContextCancelled(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(5 * time.Second)
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel before the request is sent

	c := mustNew(t, &common.AgentConfig{
		ServerURL: ts.URL,
		StateFile: filepath.Join(t.TempDir(), "state.json"),
	})
	if err := c.register(ctx); err == nil {
		t.Fatal("expected error for cancelled context")
	}
}

// ── WebSocket connection ──────────────────────────────────────────────────────

func TestConnect_UnauthorizedClearsState(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/api/v1/ws") {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		http.NotFound(w, r)
	}))
	defer ts.Close()

	stateFile := filepath.Join(t.TempDir(), "state.json")
	c := mustNew(t, &common.AgentConfig{ServerURL: ts.URL, StateFile: stateFile})
	c.state.AgentID = "c1"
	c.state.Token = "t1"

	err := c.connect(context.Background())
	if !errors.Is(err, errTokenRejected) {
		t.Fatalf("expected errTokenRejected, got %v", err)
	}
	if c.state.AgentID != "" || c.state.Token != "" {
		t.Fatalf("state should be cleared after token rejection, got %+v", c.state)
	}
}

// ── Playbook execution ────────────────────────────────────────────────────────

func TestRunPlaybook_FatalFailureSendsDeployFailed(t *testing.T) {
	c := mustNew(t, &common.AgentConfig{ServerURL: "http://example.com"})
	w := newTestWriter()

	c.runPlaybook(context.Background(), w, &common.WSPlaybookData{
		DeploymentID: "dep-1",
		Playbook: &common.Playbook{
			Name: "pb",
			Jobs: []common.Job{{Name: "j1", Steps: []common.Step{
				{Name: "bad", Uses: "unknown-step"},
			}}},
		},
	})

	msgs := drainMessages(w)
	if !hasMessageType(msgs, common.WSMsgStepStart) {
		t.Error("expected step_start")
	}
	if !hasMessageType(msgs, common.WSMsgDeployFailed) {
		t.Error("expected deploy_failed")
	}
	if hasMessageType(msgs, common.WSMsgDeployDone) {
		t.Error("deploy_done must not be sent on fatal failure")
	}
}

func TestRunPlaybook_ContinueOnError_SendsDeployDone(t *testing.T) {
	c := mustNew(t, &common.AgentConfig{ServerURL: "http://example.com"})
	w := newTestWriter()

	c.runPlaybook(context.Background(), w, &common.WSPlaybookData{
		DeploymentID: "dep-2",
		Playbook: &common.Playbook{
			Name: "pb",
			Jobs: []common.Job{{Name: "j1", Steps: []common.Step{
				{Name: "bad", Uses: "unknown-step", ContinueOnError: true},
				{Name: "ok", Uses: "upload-logs"},
			}}},
		},
	})

	if !hasMessageType(drainMessages(w), common.WSMsgDeployDone) {
		t.Fatal("expected deploy_done when continue-on-error absorbs the failure")
	}
}

func TestRunPlaybook_SkipsStepsBeforeResumeIndex(t *testing.T) {
	c := mustNew(t, &common.AgentConfig{ServerURL: "http://example.com"})
	w := newTestWriter()

	// step 0 uses unknown-step which would cause deploy_failed if executed.
	// ResumeStepIndex=1 must skip it and run step 1 successfully.
	c.runPlaybook(context.Background(), w, &common.WSPlaybookData{
		DeploymentID:    "dep-resume",
		ResumeStepIndex: 1,
		Playbook: &common.Playbook{
			Name: "pb",
			Jobs: []common.Job{{Name: "j1", Steps: []common.Step{
				{Name: "should-be-skipped", Uses: "unknown-step"},
				{Name: "should-run", Uses: "upload-logs"},
			}}},
		},
	})

	msgs := drainMessages(w)
	for _, m := range msgs {
		if m.Type == common.WSMsgStepStart {
			if d, ok := m.Data.(common.WSStepData); ok {
				if d.StepName == "should-be-skipped" {
					t.Error("skipped step must not produce step_start")
				}
			}
		}
	}
	if !hasMessageType(msgs, common.WSMsgDeployDone) {
		t.Error("expected deploy_done for the resumed playbook")
	}
}

// ── Command execution ─────────────────────────────────────────────────────────

func TestRunCommand_Success(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses sh")
	}
	c := mustNew(t, &common.AgentConfig{ServerURL: "http://example.com"})
	w := newTestWriter()

	c.runCommand(context.Background(), w, &common.WSCommandData{
		CmdID:   "cmd-1",
		Command: "echo hello-kompakt",
		Shell:   "sh",
	})

	msgs := drainMessages(w)
	var sawOutput, sawCompleted bool
	for _, m := range msgs {
		switch m.Type {
		case common.WSMsgCommandStream:
			if d, ok := m.Data.(common.WSCommandChunk); ok && strings.Contains(d.Output, "hello-kompakt") {
				sawOutput = true
			}
		case common.WSMsgCommandCompleted:
			sawCompleted = true
			if d, ok := m.Data.(common.WSCommandStatus); ok && d.ExitCode != 0 {
				t.Errorf("exit code = %d; want 0", d.ExitCode)
			}
		}
	}
	if !sawOutput {
		t.Error("expected echo output in command_stream")
	}
	if !sawCompleted {
		t.Error("expected command_completed message")
	}
}

func TestRunCommand_NonZeroExitCode(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses sh")
	}
	c := mustNew(t, &common.AgentConfig{ServerURL: "http://example.com"})
	w := newTestWriter()

	c.runCommand(context.Background(), w, &common.WSCommandData{
		CmdID:   "cmd-fail",
		Command: "exit 42",
		Shell:   "sh",
	})

	for _, m := range drainMessages(w) {
		if m.Type == common.WSMsgCommandCompleted {
			if d, ok := m.Data.(common.WSCommandStatus); ok {
				if d.ExitCode != 42 {
					t.Errorf("exit code = %d; want 42", d.ExitCode)
				}
				return
			}
		}
	}
	t.Fatal("no command_completed message received")
}

func TestRunCommand_EmptyShellUsesDefault(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("platform-specific default shell")
	}
	c := mustNew(t, &common.AgentConfig{ServerURL: "http://example.com"})
	w := newTestWriter()

	c.runCommand(context.Background(), w, &common.WSCommandData{
		CmdID:   "cmd-default",
		Command: "echo default-ok",
		Shell:   "", // should fall back to defaultShell()
	})

	for _, m := range drainMessages(w) {
		if m.Type == common.WSMsgCommandCompleted {
			if d, ok := m.Data.(common.WSCommandStatus); ok && d.ExitCode != 0 {
				t.Errorf("default shell exited with code %d", d.ExitCode)
			}
			return
		}
	}
	t.Fatal("no command_completed message received")
}

// ── MessageWriter lifecycle ───────────────────────────────────────────────────

func TestMessageWriterLifecycle(t *testing.T) {
	received := make(chan common.WSMessage, 1)

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{InsecureSkipVerify: true})
		if err != nil {
			t.Errorf("accept: %v", err)
			return
		}
		defer conn.CloseNow()

		_, data, err := conn.Read(r.Context())
		if err != nil {
			t.Errorf("Read: %v", err)
			return
		}
		var msg common.WSMessage
		if err := json.Unmarshal(data, &msg); err != nil {
			t.Errorf("unmarshal message: %v", err)
			return
		}
		received <- msg
	}))
	defer ts.Close()

	wsURL := strings.Replace(ts.URL, "http://", "ws://", 1)
	conn, _, err := websocket.Dial(context.Background(), wsURL, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.CloseNow()

	w := newMessageWriter(conn)
	w.Send(common.WSMessage{Type: common.WSMsgPing, Timestamp: time.Now()})

	select {
	case got := <-received:
		if got.Type != common.WSMsgPing {
			t.Fatalf("message type = %q; want %q", got.Type, common.WSMsgPing)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for websocket message")
	}

	w.Stop()

	done := make(chan struct{})
	go func() {
		w.Send(common.WSMessage{Type: common.WSMsgPong, Timestamp: time.Now()})
		close(done)
	}()

	select {
	case <-done:
		// Send after Stop must return promptly without blocking.
	case <-time.After(2 * time.Second):
		t.Fatal("Send after Stop blocked unexpectedly")
	}
}
