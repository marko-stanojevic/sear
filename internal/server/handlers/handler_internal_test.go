package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/marko-stanojevic/kompakt/internal/common"
)

// makeTestConn builds an AgentConn suitable for unit tests: the conn field is nil
// because Hub.Send and Hub.IsConnected never touch it.
func makeTestConn(agentID string, bufSize int) *AgentConn {
	return &AgentConn{
		agentID: agentID,
		outbox:  make(chan []byte, bufSize),
		stop:    make(chan struct{}),
	}
}

// ── Hub ───────────────────────────────────────────────────────────────────────

func TestHub_IsConnected_FalseBeforeRegistration(t *testing.T) {
	h := NewHub()
	if h.IsConnected("c1") {
		t.Fatal("IsConnected should return false before registration")
	}
}

func TestHub_Send_EnqueuesMessageForConnectedAgent(t *testing.T) {
	h := NewHub()
	conn := makeTestConn("c1", 64)
	h.mu.Lock()
	h.conns["c1"] = conn
	h.mu.Unlock()

	if !h.IsConnected("c1") {
		t.Fatal("IsConnected should return true after conn is added")
	}

	if !h.Send("c1", common.WSMessage{Type: common.WSMsgLog}) {
		t.Fatal("Send should return true for a connected agent")
	}

	select {
	case raw := <-conn.outbox:
		var msg common.WSMessage
		if err := json.Unmarshal(raw, &msg); err != nil {
			t.Fatalf("unmarshal sent message: %v", err)
		}
		if msg.Type != common.WSMsgLog {
			t.Errorf("msg.Type = %q; want %q", msg.Type, common.WSMsgLog)
		}
	default:
		t.Fatal("no message received in outbox channel")
	}
}

func TestHub_Send_ReturnsFalseForUnknownAgent(t *testing.T) {
	h := NewHub()
	if h.Send("ghost", common.WSMessage{Type: common.WSMsgLog}) {
		t.Error("Send should return false for an unconnected agent")
	}
}

func TestHub_Send_ReturnsFalseWhenOutboxFull(t *testing.T) {
	h := NewHub()
	conn := makeTestConn("c2", 1)
	h.mu.Lock()
	h.conns["c2"] = conn
	h.mu.Unlock()

	if !h.Send("c2", common.WSMessage{Type: common.WSMsgLog}) {
		t.Fatal("first Send should succeed when queue has space")
	}
	if h.Send("c2", common.WSMessage{Type: common.WSMsgLog}) {
		t.Error("Send should return false when outbox is full")
	}
}

func TestHub_Unregister_RemovesAgent(t *testing.T) {
	h := NewHub()
	conn := makeTestConn("c3", 8)
	h.mu.Lock()
	h.conns["c3"] = conn
	h.mu.Unlock()

	h.unregister("c3")
	if h.IsConnected("c3") {
		t.Fatal("IsConnected should return false after unregister")
	}
}

func TestHub_Unregister_UnknownAgentDoesNotPanic(t *testing.T) {
	h := NewHub()
	h.unregister("nobody") // must not panic
	if h.IsConnected("nobody") {
		t.Error("IsConnected should be false for an unknown agent")
	}
}

func TestHub_Register_NewAgentIsConnected(t *testing.T) {
	h := NewHub()
	conn := makeTestConn("c4", 8)
	h.register(conn)
	if !h.IsConnected("c4") {
		t.Error("IsConnected should be true after register")
	}
}

func TestHub_ConcurrentSend_NoDataRace(t *testing.T) {
	h := NewHub()
	conn := makeTestConn("concurrent", 1000)
	h.mu.Lock()
	h.conns["concurrent"] = conn
	h.mu.Unlock()

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			h.Send("concurrent", common.WSMessage{Type: common.WSMsgLog})
		}()
	}
	wg.Wait()
}

// ── CommandStore ──────────────────────────────────────────────────────────────

func TestCommandStore_CreateAndGet(t *testing.T) {
	cs := NewCommandStore()
	sess := cs.Create("cmd-1", "agent-1", "echo hi")
	if sess == nil {
		t.Fatal("Create should return a non-nil session")
	}
	got, ok := cs.Get("cmd-1")
	if !ok {
		t.Fatal("Get should find the created session")
	}
	if got != sess {
		t.Fatal("Get should return the same session pointer")
	}
}

func TestCommandStore_Get_UnknownID(t *testing.T) {
	cs := NewCommandStore()
	_, ok := cs.Get("nonexistent")
	if ok {
		t.Error("Get should return false for unknown command ID")
	}
}

func TestCommandStore_AppendOutput_AccumulatesLines(t *testing.T) {
	cs := NewCommandStore()
	cs.Create("cmd-2", "agent-1", "echo hi")

	cs.AppendOutput("cmd-2", "line1")
	cs.AppendOutput("cmd-2", "line2")
	cs.AppendOutput("cmd-2", "line3")

	sess, _ := cs.Get("cmd-2")
	out, done, _, _ := sess.Snapshot()
	if len(out) != 3 {
		t.Fatalf("expected 3 output lines, got %d: %v", len(out), out)
	}
	if done {
		t.Error("session should not be done yet")
	}
}

func TestCommandStore_AppendOutput_UnknownIDDoesNotPanic(t *testing.T) {
	cs := NewCommandStore()
	cs.AppendOutput("nonexistent", "line") // must not panic
}

func TestCommandStore_SetDone_MarksSessionComplete(t *testing.T) {
	cs := NewCommandStore()
	cs.Create("cmd-3", "agent-1", "ls")

	cs.AppendOutput("cmd-3", "file.txt")
	cs.SetDone("cmd-3", 42, "something failed")

	sess, _ := cs.Get("cmd-3")
	out, done, exitCode, errMsg := sess.Snapshot()
	if !done {
		t.Error("session should be done")
	}
	if exitCode != 42 {
		t.Errorf("exit code = %d; want 42", exitCode)
	}
	if errMsg != "something failed" {
		t.Errorf("errMsg = %q; want %q", errMsg, "something failed")
	}
	if len(out) != 1 || out[0] != "file.txt" {
		t.Errorf("output = %v; want [file.txt]", out)
	}
}

func TestCommandStore_SetDone_UnknownIDDoesNotPanic(t *testing.T) {
	cs := NewCommandStore()
	cs.SetDone("nonexistent", 0, "") // must not panic
}

func TestCommandSession_Snapshot_ReturnsCopy(t *testing.T) {
	cs := NewCommandStore()
	cs.Create("cmd-4", "agent-1", "cmd")
	cs.AppendOutput("cmd-4", "original")

	sess, _ := cs.Get("cmd-4")
	out1, _, _, _ := sess.Snapshot()
	out1[0] = "mutated"

	out2, _, _, _ := sess.Snapshot()
	if out2[0] == "mutated" {
		t.Error("Snapshot should return an independent copy, not a reference")
	}
}

// ── JSON / HTTP helpers ───────────────────────────────────────────────────────

func TestTmplFuncs_PolicyClass(t *testing.T) {
	fn := htmlTemplateFunctions["policyClass"].(func(string) string)
	tests := []struct{ in, want string }{
		{"authenticated", "badge-success"},
		{"public", "badge-warning"},
		{"restricted", "badge-success"},
		{"", "badge-success"},
	}
	for _, tt := range tests {
		if got := fn(tt.in); got != tt.want {
			t.Errorf("policyClass(%q) = %q; want %q", tt.in, got, tt.want)
		}
	}
}

func TestTmplFuncs_PolicyLabel(t *testing.T) {
	fn := htmlTemplateFunctions["policyLabel"].(func(string) string)
	tests := []struct{ in, want string }{
		{"authenticated", "Authenticated"},
		{"public", "Public"},
		{"restricted", "Restricted"},
		{"", "Authenticated"},
	}
	for _, tt := range tests {
		if got := fn(tt.in); got != tt.want {
			t.Errorf("policyLabel(%q) = %q; want %q", tt.in, got, tt.want)
		}
	}
}

func TestWriteJSON_SetsHeaderAndBody(t *testing.T) {
	rr := httptest.NewRecorder()
	writeJSON(rr, http.StatusCreated, map[string]string{"hello": "world"})

	if rr.Code != http.StatusCreated {
		t.Errorf("status = %d; want 201", rr.Code)
	}
	if ct := rr.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q; want application/json", ct)
	}
	var got map[string]string
	if err := json.NewDecoder(rr.Body).Decode(&got); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if got["hello"] != "world" {
		t.Errorf("body[hello] = %q; want world", got["hello"])
	}
}

func TestWriteError_WrapsMessageInErrorField(t *testing.T) {
	rr := httptest.NewRecorder()
	writeError(rr, http.StatusBadRequest, "something went wrong")

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d; want 400", rr.Code)
	}
	var got map[string]string
	if err := json.NewDecoder(rr.Body).Decode(&got); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if got["error"] != "something went wrong" {
		t.Errorf("error field = %q; want 'something went wrong'", got["error"])
	}
}
