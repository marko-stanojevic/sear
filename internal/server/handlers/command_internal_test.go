package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/marko-stanojevic/kompakt/internal/common"
	"github.com/marko-stanojevic/kompakt/internal/server/service"
	"github.com/marko-stanojevic/kompakt/internal/server/store"
)

// newCommandTestEnv creates a Handler with all fields needed for HandleCommand tests.
func newCommandTestEnv(t *testing.T) *Handler {
	t.Helper()
	st, err := store.New(t.TempDir(), "")
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	hub := NewHub()
	return &Handler{
		Store:            st,
		RootPassword:     "admin123",
		TokenExpiryHours: 24,
		ArtifactsDir:     t.TempDir(),
		ServerURL:        "http://localhost:8080",
		Hub:              hub,
		Service:          &service.Manager{Store: st, Hub: hub, ServerURL: "http://localhost:8080"},
		Commands:         NewCommandStore(),
		RegistrationSecrets: map[string]string{
			"prod": "reg-secret-1",
		},
	}
}

// seedAgent puts an agent directly into the store.
func seedAgent(t *testing.T, st interface {
	SaveAgent(*common.Agent) error
}, id, hostname string, shells []string) {
	t.Helper()
	a := &common.Agent{
		ID:       id,
		Hostname: hostname,
		Platform: common.PlatformLinux,
		Shells:   shells,
	}
	if err := st.SaveAgent(a); err != nil {
		t.Fatalf("seedAgent: %v", err)
	}
}

// addConnectedAgent puts a fake connection into the hub for agentID.
func addConnectedAgent(h *Hub, agentID string) {
	conn := makeTestConn(agentID, 64)
	h.mu.Lock()
	h.conns[agentID] = conn
	h.mu.Unlock()
}

func cmdRequest(t *testing.T, method, path string, body any) *http.Request {
	t.Helper()
	var buf *bytes.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		buf = bytes.NewReader(b)
	} else {
		buf = bytes.NewReader(nil)
	}
	req := httptest.NewRequest(method, path, buf)
	req.Header.Set("Content-Type", "application/json")
	return req
}

// ── POST /agents/{id}/command ─────────────────────────────────────────────────

func TestHandleCommand_POST_InvalidJSON(t *testing.T) {
	env := newCommandTestEnv(t)
	seedAgent(t, env.Store, "a1", "host-1", nil)

	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString("{invalid"))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	env.HandleCommand(rr, req, "a1", "")

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d; want 400", rr.Code)
	}
}

func TestHandleCommand_POST_EmptyCommand(t *testing.T) {
	env := newCommandTestEnv(t)
	seedAgent(t, env.Store, "a1", "host-1", nil)

	req := cmdRequest(t, http.MethodPost, "/", map[string]string{"command": "   "})
	rr := httptest.NewRecorder()
	env.HandleCommand(rr, req, "a1", "")

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d; want 400", rr.Code)
	}
}

func TestHandleCommand_POST_AgentNotFound(t *testing.T) {
	env := newCommandTestEnv(t)

	req := cmdRequest(t, http.MethodPost, "/", map[string]string{"command": "echo hi"})
	rr := httptest.NewRecorder()
	env.HandleCommand(rr, req, "nonexistent", "")

	if rr.Code != http.StatusNotFound {
		t.Errorf("status = %d; want 404", rr.Code)
	}
}

func TestHandleCommand_POST_ShellNotAvailable(t *testing.T) {
	env := newCommandTestEnv(t)
	seedAgent(t, env.Store, "a1", "host-1", []string{"bash", "sh"})

	req := cmdRequest(t, http.MethodPost, "/", map[string]any{
		"command": "echo hi",
		"shell":   "pwsh",
	})
	rr := httptest.NewRecorder()
	env.HandleCommand(rr, req, "a1", "")

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d; want 400", rr.Code)
	}
}

func TestHandleCommand_POST_AgentNotConnected(t *testing.T) {
	env := newCommandTestEnv(t)
	seedAgent(t, env.Store, "a1", "host-1", nil)

	req := cmdRequest(t, http.MethodPost, "/", map[string]string{"command": "echo hi"})
	rr := httptest.NewRecorder()
	env.HandleCommand(rr, req, "a1", "")

	if rr.Code != http.StatusConflict {
		t.Errorf("status = %d; want 409", rr.Code)
	}
}

func TestHandleCommand_POST_Success(t *testing.T) {
	env := newCommandTestEnv(t)
	seedAgent(t, env.Store, "a1", "host-1", nil)
	addConnectedAgent(env.Hub, "a1")

	req := cmdRequest(t, http.MethodPost, "/", map[string]string{"command": "echo hi"})
	rr := httptest.NewRecorder()
	env.HandleCommand(rr, req, "a1", "")

	if rr.Code != http.StatusAccepted {
		t.Errorf("status = %d; want 202 (body: %s)", rr.Code, rr.Body.String())
	}
	var resp map[string]string
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp["cmd_id"] == "" {
		t.Error("expected non-empty cmd_id in response")
	}
}

func TestHandleCommand_POST_AvailableShellAccepted(t *testing.T) {
	env := newCommandTestEnv(t)
	seedAgent(t, env.Store, "a1", "host-1", []string{"bash", "sh"})
	addConnectedAgent(env.Hub, "a1")

	req := cmdRequest(t, http.MethodPost, "/", map[string]any{
		"command": "echo hi",
		"shell":   "bash",
	})
	rr := httptest.NewRecorder()
	env.HandleCommand(rr, req, "a1", "")

	if rr.Code != http.StatusAccepted {
		t.Errorf("status = %d; want 202 (body: %s)", rr.Code, rr.Body.String())
	}
}

func TestHandleCommand_POST_NoShellRestriction_AnyShellAccepted(t *testing.T) {
	// When agent.Shells is empty, no shell validation is done.
	env := newCommandTestEnv(t)
	seedAgent(t, env.Store, "a1", "host-1", nil)
	addConnectedAgent(env.Hub, "a1")

	req := cmdRequest(t, http.MethodPost, "/", map[string]any{
		"command": "echo hi",
		"shell":   "pwsh",
	})
	rr := httptest.NewRecorder()
	env.HandleCommand(rr, req, "a1", "")

	if rr.Code != http.StatusAccepted {
		t.Errorf("status = %d; want 202", rr.Code)
	}
}

func TestHandleCommand_POST_WithCmdIDInPath_MethodNotAllowed(t *testing.T) {
	env := newCommandTestEnv(t)

	req := cmdRequest(t, http.MethodPost, "/", map[string]string{"command": "echo hi"})
	rr := httptest.NewRecorder()
	env.HandleCommand(rr, req, "a1", "/some-cmd-id")

	if rr.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d; want 405", rr.Code)
	}
}

// ── GET /agents/{id}/command/{cmd_id} ─────────────────────────────────────────

func TestHandleCommand_GET_MissingCmdID(t *testing.T) {
	env := newCommandTestEnv(t)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()
	env.HandleCommand(rr, req, "a1", "")

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d; want 400", rr.Code)
	}
}

func TestHandleCommand_GET_UnknownCmdID(t *testing.T) {
	env := newCommandTestEnv(t)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()
	env.HandleCommand(rr, req, "a1", "/nonexistent")

	if rr.Code != http.StatusNotFound {
		t.Errorf("status = %d; want 404", rr.Code)
	}
}

func TestHandleCommand_GET_WrongAgentID(t *testing.T) {
	env := newCommandTestEnv(t)
	env.Commands.Create("cmd-1", "agent-x", "echo hi")

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()
	// Session belongs to agent-x but we're requesting as agent-y.
	env.HandleCommand(rr, req, "agent-y", "/cmd-1")

	if rr.Code != http.StatusNotFound {
		t.Errorf("status = %d; want 404", rr.Code)
	}
}

func TestHandleCommand_GET_Success(t *testing.T) {
	env := newCommandTestEnv(t)
	cmdID := "cmd-abc"
	sess := env.Commands.Create(cmdID, "a1", "echo hi")
	_ = sess

	env.Commands.AppendOutput(cmdID, "hello")
	env.Commands.SetDone(cmdID, 0, "")

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()
	env.HandleCommand(rr, req, "a1", "/"+cmdID)

	if rr.Code != http.StatusOK {
		t.Errorf("status = %d; want 200", rr.Code)
	}

	var resp map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp["cmd_id"] != cmdID {
		t.Errorf("cmd_id = %v; want %s", resp["cmd_id"], cmdID)
	}
	if done, _ := resp["done"].(bool); !done {
		t.Error("expected done=true")
	}
}

// ── Unsupported methods ───────────────────────────────────────────────────────

func TestHandleCommand_UnsupportedMethod(t *testing.T) {
	env := newCommandTestEnv(t)

	req := httptest.NewRequest(http.MethodPatch, "/", nil)
	rr := httptest.NewRecorder()
	env.HandleCommand(rr, req, "a1", "")

	if rr.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d; want 405", rr.Code)
	}
}
