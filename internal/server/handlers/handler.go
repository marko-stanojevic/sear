// Package handlers implements all HTTP and WebSocket handlers for the kompakt server.
package handlers

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/coder/websocket"
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
	conns map[string]*AgentConn // agentID → connection
}

// AgentConn wraps a single agent WebSocket connection with an outbound queue.
type AgentConn struct {
	agentID string
	conn    *websocket.Conn
	outbox  chan []byte
	stop    chan struct{} // closed to stop the write pump
}

// NewHub creates an empty Hub.
func NewHub() *Hub {
	return &Hub{conns: make(map[string]*AgentConn)}
}

// register adds (or replaces) the connection for an agent.
func (h *Hub) register(conn *AgentConn) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if old, ok := h.conns[conn.agentID]; ok {
		close(old.stop)
		_ = old.conn.Close(websocket.StatusGoingAway, "replaced by new connection")
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
	case conn.outbox <- data:
		return true
	default:
		return false
	}
}

// newAgentConn creates an AgentConn and starts its write pump goroutine.
func newAgentConn(agentID string, ws *websocket.Conn) *AgentConn {
	c := &AgentConn{
		agentID: agentID,
		conn:    ws,
		outbox:  make(chan []byte, 64),
		stop:    make(chan struct{}),
	}
	go c.writePump()
	return c
}

func (c *AgentConn) writePump() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case data, ok := <-c.outbox:
			if !ok {
				_ = c.conn.Close(websocket.StatusNormalClosure, "")
				return
			}
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			err := c.conn.Write(ctx, websocket.MessageText, data)
			cancel()
			if err != nil {
				return
			}
		case <-ticker.C:
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			err := c.conn.Ping(ctx)
			cancel()
			if err != nil {
				return
			}
		case <-c.stop:
			_ = c.conn.Close(websocket.StatusGoingAway, "shutting down")
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
