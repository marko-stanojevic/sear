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
	AgentJWTSecret      []byte // signs agent tokens
	UserJWTSecret       []byte // signs UI session tokens (separate to allow independent rotation)
	RootPassword        string
	TokenExpiryHours    int
	ArtifactsDir        string
	ServerURL           string
	RegistrationSecrets map[string]string // name→value from secrets.yml
	Hub                 *Hub
	Service             *service.Manager
}

// ── WebSocket Hub ─────────────────────────────────────────────────────────────

// Hub manages all active WebSocket connections.
type Hub struct {
	mu    sync.RWMutex
	conns map[string]*WSConn // clientID → connection
}

// WSConn wraps a single client WebSocket connection with an outbound queue.
type WSConn struct {
	clientID string
	ws       *websocket.Conn
	send     chan []byte
	done     chan struct{}
}

// NewHub creates an empty Hub.
func NewHub() *Hub {
	return &Hub{conns: make(map[string]*WSConn)}
}

// register adds (or replaces) the connection for a client.
func (h *Hub) register(conn *WSConn) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if old, ok := h.conns[conn.clientID]; ok {
		close(old.done)
		_ = old.ws.Close()
	}
	h.conns[conn.clientID] = conn
}

// unregister removes the connection for a client.
func (h *Hub) unregister(clientID string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.conns, clientID)
}

// IsConnected reports whether a client has an open WebSocket connection.
func (h *Hub) IsConnected(clientID string) bool {
	h.mu.RLock()
	defer h.mu.RUnlock()
	_, ok := h.conns[clientID]
	return ok
}

// Send queues a message for the named client. Returns false if not connected.
func (h *Hub) Send(clientID string, msg common.WSMessage) bool {
	h.mu.RLock()
	conn, ok := h.conns[clientID]
	h.mu.RUnlock()
	if !ok {
		return false
	}
	data, err := json.Marshal(msg)
	if err != nil {
		slog.Error("failed to marshal websocket message", "client_id", clientID, "msg_type", msg.Type, "err", err)
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
func newWSConn(clientID string, ws *websocket.Conn) *WSConn {
	c := &WSConn{
		clientID: clientID,
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

// ── JSON / HTTP helpers ───────────────────────────────────────────────────────

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]string{"error": msg})
}
