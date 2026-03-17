// Package server assembles and starts the kompakt HTTP server.
package server

import (
	"bufio"
	"log"
	"net"
	"net/http"
	"time"

	"github.com/marko-stanojevic/kompakt/internal/server/handlers"
)

// NewServer wires all HTTP routes and returns a ready-to-use http.Handler.
func NewServer(env *handlers.Handler) http.Handler {
	mux := http.NewServeMux()

	// ── Public (no auth) ─────────────────────────────────────────────────────
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	// ── Agent API ───────────────────────────────────────────────────────────
	// POST /api/v1/register  — no auth; validated by registration secret
	mux.Handle("/api/v1/register", http.HandlerFunc(env.HandleAgentRegister))

	// POST /api/v1/ui/login — no auth; validates root password, returns short-lived UI JWT
	mux.HandleFunc("/api/v1/ui/login", env.HandleUILogin)

	// GET  /api/v1/ws        — WebSocket; JWT via ?token= query param
	mux.Handle("/api/v1/ws", http.HandlerFunc(env.HandleAgentWS))

	// ── Root API (HTTP Basic auth) ───────────────────────────────────────────
	root := env.RequireRootAuth

	mux.Handle("/api/v1/status", root(http.HandlerFunc(env.HandleStatus)))

	// HTML UI pages are served without Basic auth; in-page JS handles auth for API calls.
	mux.Handle("/ui/assets/", http.HandlerFunc(handlers.ServeUIAsset))
	mux.Handle("/ui", http.HandlerFunc(env.HandleHomeUI))
	mux.Handle("/ui/", http.HandlerFunc(env.HandleHomeUI))
	mux.Handle("/ui/clients", http.HandlerFunc(env.HandleAgentsUI))
	mux.Handle("/ui/secrets", http.HandlerFunc(env.HandleSecretsUI))
	mux.Handle("/ui/playbooks", http.HandlerFunc(env.HandlePlaybooksUI))
	mux.Handle("/ui/deployments", http.HandlerFunc(env.HandleDeploymentsUI))
	mux.Handle("/ui/artifacts", http.HandlerFunc(env.HandleArtifactsUI))
	mux.Handle("/ui/artifacts/", http.HandlerFunc(env.HandleArtifactsUI))

	mux.Handle("/api/v1/playbooks", root(http.HandlerFunc(env.HandleRootPlaybooks)))
	mux.Handle("/api/v1/playbooks/", root(http.HandlerFunc(env.HandleRootPlaybooks)))

	mux.Handle("/api/v1/clients", root(http.HandlerFunc(env.HandleRootClients)))
	mux.Handle("/api/v1/clients/", root(http.HandlerFunc(env.HandleRootClients)))

	mux.Handle("/api/v1/deployments", root(http.HandlerFunc(env.HandleRootDeployments)))
	mux.Handle("/api/v1/deployments/", root(http.HandlerFunc(env.HandleRootDeployments)))

	// Artifacts are accessible by both agents (JWT) and root (Basic auth).
	// We now handle granular access (Public/Restricted) inside HandleArtifacts.
	mux.HandleFunc("/artifacts", env.HandleArtifacts)
	mux.HandleFunc("/artifacts/", env.HandleArtifacts)

	mux.Handle("/api/v1/secrets", root(http.HandlerFunc(env.HandleSecrets)))
	mux.Handle("/api/v1/secrets/", root(http.HandlerFunc(env.HandleSecrets)))

	return logging(mux)
}

// dualAuth accepts requests authenticated with either an agent JWT Bearer
// token or root HTTP Basic credentials.
func dualAuth(env *handlers.Handler, next http.Handler) http.Handler {
	clientMW := env.RequireAgentAuth(next)
	adminMW := env.RequireRootAuth(next)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, _, ok := r.BasicAuth(); ok {
			adminMW.ServeHTTP(w, r)
		} else {
			clientMW.ServeHTTP(w, r)
		}
	})
}

// logging logs every request with method, path, status, and duration.
func logging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		lrw := &loggingResponseWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(lrw, r)
		log.Printf("%s %s %d %s", r.Method, r.URL.Path, lrw.status, time.Since(start))
	})
}

type loggingResponseWriter struct {
	http.ResponseWriter
	status int
}

func (l *loggingResponseWriter) WriteHeader(code int) {
	l.status = code
	l.ResponseWriter.WriteHeader(code)
}

func (l *loggingResponseWriter) Flush() {
	if flusher, ok := l.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

func (l *loggingResponseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	hijacker, ok := l.ResponseWriter.(http.Hijacker)
	if !ok {
		return nil, nil, http.ErrNotSupported
	}
	return hijacker.Hijack()
}

func (l *loggingResponseWriter) Push(target string, opts *http.PushOptions) error {
	pusher, ok := l.ResponseWriter.(http.Pusher)
	if !ok {
		return http.ErrNotSupported
	}
	return pusher.Push(target, opts)
}
