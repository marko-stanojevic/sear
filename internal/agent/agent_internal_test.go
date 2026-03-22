package agent

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/marko-stanojevic/kompakt/internal/common"
)

func mustNew(t *testing.T, cfg *common.AgentConfig) *Agent {
	t.Helper()
	c, err := New(cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return c
}

func TestWSEndpoint(t *testing.T) {
	got := wsEndpoint("http://example.com/base", "tok-123")
	if !strings.HasPrefix(got, "ws://example.com/base/api/v1/ws?") {
		t.Fatalf("ws endpoint prefix mismatch: %s", got)
	}
	if !strings.Contains(got, "token=tok-123") {
		t.Fatalf("token query missing: %s", got)
	}

	gotTLS := wsEndpoint("https://example.com", "t")
	if !strings.HasPrefix(gotTLS, "wss://example.com/api/v1/ws?") {
		t.Fatalf("wss endpoint prefix mismatch: %s", gotTLS)
	}
}

func TestSaveLoadStateRoundTrip(t *testing.T) {
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

func TestRegisterSuccess(t *testing.T) {
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
		t.Fatalf("expected state file to be written: %v", err)
	}
}

func TestConnectUnauthorizedClearsState(t *testing.T) {
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

func TestRunPlaybookMessageFlows(t *testing.T) {
	newWriter := func() *MessageWriter {
		return &MessageWriter{
			outbox: make(chan common.WSMessage, 64),
			stop: make(chan struct{}),
			done: make(chan struct{}),
		}
	}
	drain := func(w *MessageWriter) []common.WSMessage {
		msgs := make([]common.WSMessage, 0, len(w.outbox))
		for len(w.outbox) > 0 {
			msgs = append(msgs, <-w.outbox)
		}
		return msgs
	}
	hasType := func(msgs []common.WSMessage, typ common.WSMessageType) bool {
		for _, m := range msgs {
			if m.Type == typ {
				return true
			}
		}
		return false
	}

	c := mustNew(t, &common.AgentConfig{ServerURL: "http://example.com"})

	t.Run("fatal failure sends deploy_failed", func(t *testing.T) {
		w := newWriter()
		pd := &common.WSPlaybookData{
			DeploymentID: "dep-1",
			Playbook: &common.Playbook{
				Name: "pb",
				Jobs: []common.Job{{Name: "j1", Steps: []common.Step{{Name: "bad", Uses: "unknown-step"}}}},
			},
		}
		c.runPlaybook(context.Background(), w, pd)
		msgs := drain(w)
		if !hasType(msgs, common.WSMsgStepStart) || !hasType(msgs, common.WSMsgDeployFailed) {
			t.Fatalf("expected step_start and deploy_failed, got %#v", msgs)
		}
		if hasType(msgs, common.WSMsgDeployDone) {
			t.Fatalf("did not expect deploy_done on fatal failure")
		}
	})

	t.Run("continue on error eventually sends deploy_done", func(t *testing.T) {
		w := newWriter()
		pd := &common.WSPlaybookData{
			DeploymentID: "dep-2",
			Playbook: &common.Playbook{
				Name: "pb",
				Jobs: []common.Job{{Name: "j1", Steps: []common.Step{
					{Name: "bad", Uses: "unknown-step", ContinueOnError: true},
					{Name: "ok", Uses: "upload-logs"},
				}}},
			},
		}
		c.runPlaybook(context.Background(), w, pd)
		msgs := drain(w)
		if !hasType(msgs, common.WSMsgDeployDone) {
			t.Fatalf("expected deploy_done, got %#v", msgs)
		}
	})
}

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
		// send after stop should return promptly and not block forever
	case <-time.After(2 * time.Second):
		t.Fatal("Send after Stop blocked unexpectedly")
	}
}
