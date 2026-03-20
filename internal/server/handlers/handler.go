// Package handlers implements all HTTP and WebSocket handlers for the kompakt server.
package handlers

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/marko-stanojevic/kompakt/internal/common"
	"github.com/marko-stanojevic/kompakt/internal/server/ports"
	"github.com/marko-stanojevic/kompakt/internal/server/service"
)

// Handler bundles the dependencies shared by all handlers.
type Handler struct {
	Store               ports.Store
	UserJWTSecret       []byte // signs UI session tokens
	RootPassword        string
	TokenExpiryHours    int
	ArtifactsDir        string
	ServerURL           string
	RegistrationSecrets map[string]string // name→value from secrets.yml
	Hub                 *Hub
	Service             *service.Manager
	Commands            *CommandStore
}

// ── WebSocket Hub ─────────────────────────────────────────────────────────────

// Hub manages all active WebSocket connections.
type Hub struct {
	mu    sync.RWMutex
	conns map[string]*WSConn // agentID → connection
}

// WSConn wraps a single agent WebSocket connection with an outbound queue.
type WSConn struct {
	agentID string
	ws       *websocket.Conn
	send     chan []byte
	done     chan struct{}
}

// NewHub creates an empty Hub.
func NewHub() *Hub {
	return &Hub{conns: make(map[string]*WSConn)}
}

// register adds (or replaces) the connection for an agent.
func (h *Hub) register(conn *WSConn) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if old, ok := h.conns[conn.agentID]; ok {
		close(old.done)
		_ = old.ws.Close()
	}
	h.conns[conn.agentID] = conn
}

// unregister removes the connection for an agent.
func (h *Hub) unregister(agentID string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.conns, agentID)
}

// IsConnected reports whether an agent has an open WebSocket connection.
func (h *Hub) IsConnected(agentID string) bool {
	h.mu.RLock()
	defer h.mu.RUnlock()
	_, ok := h.conns[agentID]
	return ok
}

// Send queues a message for the named agent. Returns false if not connected.
func (h *Hub) Send(agentID string, msg common.WSMessage) bool {
	h.mu.RLock()
	conn, ok := h.conns[agentID]
	h.mu.RUnlock()
	if !ok {
		return false
	}
	data, err := json.Marshal(msg)
	if err != nil {
		slog.Error("failed to marshal websocket message", "agent_id", agentID, "msg_type", msg.Type, "err", err)
		return false
	}
	select {
	case conn.send <- data:
		return true
	default:
		return false
	}
}

// newWSConn creates a WSConn and starts its write pump goroutine.
func newWSConn(agentID string, ws *websocket.Conn) *WSConn {
	c := &WSConn{
		agentID: agentID,
		ws:       ws,
		send:     make(chan []byte, 64),
		done:     make(chan struct{}),
	}
	go c.writePump()
	return c
}

func (c *WSConn) writePump() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case data, ok := <-c.send:
			if err := c.ws.SetWriteDeadline(time.Now().Add(10 * time.Second)); err != nil {
				return
			}
			if !ok {
				if err := c.ws.WriteMessage(websocket.CloseMessage, []byte{}); err != nil {
					return
				}
				return
			}
			if err := c.ws.WriteMessage(websocket.TextMessage, data); err != nil {
				return
			}
		case <-ticker.C:
			if err := c.ws.SetWriteDeadline(time.Now().Add(10 * time.Second)); err != nil {
				return
			}
			if err := c.ws.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		case <-c.done:
			return
		}
	}
}

// ── Command store ─────────────────────────────────────────────────────────────

// CommandSession holds the in-memory state of one ad-hoc command execution.
type CommandSession struct {
	AgentID  string
	Command  string
	output   []string
	exitCode int
	errMsg   string
	done     bool
	mu       sync.RWMutex
}

// Snapshot returns a consistent copy of the session state.
func (s *CommandSession) Snapshot() (output []string, done bool, exitCode int, errMsg string) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]string, len(s.output))
	copy(out, s.output)
	return out, s.done, s.exitCode, s.errMsg
}

// CommandStore is an in-memory store for command sessions.
// Sessions are never evicted — they are small and short-lived in practice.
type CommandStore struct {
	mu   sync.RWMutex
	cmds map[string]*CommandSession
}

// NewCommandStore creates an empty CommandStore.
func NewCommandStore() *CommandStore {
	return &CommandStore{cmds: make(map[string]*CommandSession)}
}

// Create registers a new session and returns it.
func (s *CommandStore) Create(cmdID, agentID, command string) *CommandSession {
	sess := &CommandSession{AgentID: agentID, Command: command}
	s.mu.Lock()
	s.cmds[cmdID] = sess
	s.mu.Unlock()
	return sess
}

// Get retrieves a session by ID.
func (s *CommandStore) Get(cmdID string) (*CommandSession, bool) {
	s.mu.RLock()
	sess, ok := s.cmds[cmdID]
	s.mu.RUnlock()
	return sess, ok
}

// AppendOutput appends a line of output to the session.
func (s *CommandStore) AppendOutput(cmdID, line string) {
	s.mu.RLock()
	sess, ok := s.cmds[cmdID]
	s.mu.RUnlock()
	if !ok {
		return
	}
	sess.mu.Lock()
	sess.output = append(sess.output, line)
	sess.mu.Unlock()
}

// SetDone marks the session as completed.
func (s *CommandStore) SetDone(cmdID string, exitCode int, errMsg string) {
	s.mu.RLock()
	sess, ok := s.cmds[cmdID]
	s.mu.RUnlock()
	if !ok {
		return
	}
	sess.mu.Lock()
	sess.done = true
	sess.exitCode = exitCode
	sess.errMsg = errMsg
	sess.mu.Unlock()
}

// ── JSON / HTTP helpers ───────────────────────────────────────────────────────

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]string{"error": msg})
}
