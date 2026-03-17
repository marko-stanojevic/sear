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

	"github.com/gorilla/websocket"
	"github.com/marko-stanojevic/kompakt/internal/common"
)

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
	cfg := &common.AgentConfig{StateFile: stateFile}
	c := New(cfg)
	c.state = localState{ClientID: "c1", Token: "t1"}

	if err := c.saveState(); err != nil {
		t.Fatalf("saveState: %v", err)
	}

	c2 := New(&common.AgentConfig{StateFile: stateFile})
	if err := c2.loadState(); err != nil {
		t.Fatalf("loadState: %v", err)
	}
	if c2.state.ClientID != "c1" || c2.state.Token != "t1" {
		t.Fatalf("loaded state mismatch: %+v", c2.state)
	}
}

func TestPost(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/ok":
			if got := r.Header.Get("Authorization"); got != "Bearer tok" {
				t.Fatalf("Authorization = %q; want Bearer tok", got)
			}
			_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
		case "/err":
			http.Error(w, "nope", http.StatusUnauthorized)
		default:
			http.NotFound(w, r)
		}
	}))
	defer ts.Close()

	cfg := &common.AgentConfig{ServerURL: ts.URL}
	c := New(cfg)
	c.httpClient.Timeout = 5 * time.Second

	ctx := context.Background()
	var out map[string]string
	if err := c.post(ctx, "/ok", map[string]string{"x": "y"}, &out, "tok"); err != nil {
		t.Fatalf("post /ok: %v", err)
	}
	if out["status"] != "ok" {
		t.Fatalf("response decode failed: %+v", out)
	}

	if err := c.post(ctx, "/err", map[string]string{}, nil, ""); err == nil {
		t.Fatal("expected error for /err, got nil")
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
			ClientID: "client-reg-1",
			Token:    "token-reg-1",
		})
	}))
	defer ts.Close()

	c := New(&common.AgentConfig{
		ServerURL:          ts.URL,
		RegistrationSecret: "secret",
		Platform:           "auto",
		StateFile:          stateFile,
	})

	if err := c.register(context.Background()); err != nil {
		t.Fatalf("register: %v", err)
	}
	if c.state.ClientID != "client-reg-1" || c.state.Token != "token-reg-1" {
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
	c := New(&common.AgentConfig{ServerURL: ts.URL, StateFile: stateFile})
	c.state.ClientID = "c1"
	c.state.Token = "t1"

	err := c.connect(context.Background())
	if !errors.Is(err, errTokenRejected) {
		t.Fatalf("expected errTokenRejected, got %v", err)
	}
	if c.state.ClientID != "" || c.state.Token != "" {
		t.Fatalf("state should be cleared after token rejection, got %+v", c.state)
	}
}

func TestRunPlaybookMessageFlows(t *testing.T) {
	newWriter := func() *WSOutboundWriter {
		return &WSOutboundWriter{
			ch:   make(chan common.WSMessage, 64),
			stop: make(chan struct{}),
			done: make(chan struct{}),
		}
	}
	drain := func(w *WSOutboundWriter) []common.WSMessage {
		msgs := make([]common.WSMessage, 0, len(w.ch))
		for len(w.ch) > 0 {
			msgs = append(msgs, <-w.ch)
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

	c := New(&common.AgentConfig{ServerURL: "http://example.com"})

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

func TestWSOutboundWriterLifecycle(t *testing.T) {
	received := make(chan common.WSMessage, 1)

	upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("upgrade: %v", err)
			return
		}
		defer func() { _ = conn.Close() }()

		_, data, err := conn.ReadMessage()
		if err != nil {
			t.Errorf("ReadMessage: %v", err)
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
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer func() { _ = conn.Close() }()

	w := newWSOutboundWriter(conn)
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
