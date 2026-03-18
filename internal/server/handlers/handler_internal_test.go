package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/marko-stanojevic/kompakt/internal/common"
)

// makeTestConn builds a WSConn suitable for unit tests: the ws field is nil
// because Hub.Send and Hub.IsConnected never touch it.
func makeTestConn(agentID string, bufSize int) *WSConn {
	return &WSConn{
		agentID: agentID,
		send:    make(chan []byte, bufSize),
		done:    make(chan struct{}),
	}
}

// ── Hub ───────────────────────────────────────────────────────────────────────

func TestHub_IsConnectedAndSend(t *testing.T) {
	h := NewHub()

	if h.IsConnected("c1") {
		t.Fatal("IsConnected should return false before registration")
	}

	conn := makeTestConn("c1", 64)
	h.mu.Lock()
	h.conns["c1"] = conn
	h.mu.Unlock()

	if !h.IsConnected("c1") {
		t.Fatal("IsConnected should return true after conn is added")
	}

	if !h.Send("c1", common.WSMessage{Type: common.WSMsgLog}) {
		t.Fatal("Send should return true for connected agent")
	}

	select {
	case raw := <-conn.send:
		var msg common.WSMessage
		if err := json.Unmarshal(raw, &msg); err != nil {
			t.Fatalf("unmarshal sent message: %v", err)
		}
		if msg.Type != common.WSMsgLog {
			t.Errorf("msg.Type = %q; want %q", msg.Type, common.WSMsgLog)
		}
	default:
		t.Fatal("no message received in send channel")
	}

	h.unregister("c1")
	if h.IsConnected("c1") {
		t.Fatal("IsConnected should return false after unregister")
	}
}

func TestHub_Send_NotConnected(t *testing.T) {
	h := NewHub()
	if h.Send("ghost", common.WSMessage{Type: common.WSMsgLog}) {
		t.Error("Send should return false for unconnected agent")
	}
}

func TestHub_Send_QueueFull(t *testing.T) {
	h := NewHub()
	conn := makeTestConn("c2", 1)
	h.mu.Lock()
	h.conns["c2"] = conn
	h.mu.Unlock()

	if !h.Send("c2", common.WSMessage{Type: common.WSMsgLog}) {
		t.Fatal("first Send should succeed when queue has space")
	}
	if h.Send("c2", common.WSMessage{Type: common.WSMsgLog}) {
		t.Error("Send should return false when channel is full")
	}
}

func TestHub_Register_NewAgent(t *testing.T) {
	h := NewHub()
	conn := makeTestConn("c3", 8)
	// register() with no prior entry for c3 must not close any ws.
	h.register(conn)
	if !h.IsConnected("c3") {
		t.Error("IsConnected should be true after register")
	}
}

func TestHub_Unregister_Unknown(t *testing.T) {
	// Unregistering an agent that was never registered should not panic.
	h := NewHub()
	h.unregister("nobody")
	if h.IsConnected("nobody") {
		t.Error("IsConnected should be false for unknown agent")
	}
}

// ── writeJSON / writeError ────────────────────────────────────────────────────

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

func TestWriteError_WrapsInErrorField(t *testing.T) {
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
