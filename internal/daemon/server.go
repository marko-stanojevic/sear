// Package daemon assembles and starts the sear HTTP server.
package daemon

import (
	"log"
	"net/http"
	"time"

	"github.com/sear-project/sear/internal/daemon/handlers"
)

// NewServer wires all HTTP routes and returns a ready-to-use http.Handler.
func NewServer(env *handlers.Env) http.Handler {
	mux := http.NewServeMux()

	// ── Public (no auth) ─────────────────────────────────────────────────────
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	// ── Client API ───────────────────────────────────────────────────────────
	// POST /api/v1/register  — no auth; validated by registration secret
	mux.Handle("/api/v1/register", http.HandlerFunc(env.HandleRegister))

	// GET  /api/v1/ws        — WebSocket; JWT via ?token= query param
	mux.Handle("/api/v1/ws", http.HandlerFunc(env.HandleWS))

	// ── Admin API (HTTP Basic auth) ───────────────────────────────────────────
	admin := env.RequireAdminAuth

	mux.Handle("/status", admin(http.HandlerFunc(env.HandleStatus)))
	mux.Handle("/status/ui", admin(http.HandlerFunc(env.HandleStatusUI)))

	mux.Handle("/admin/playbooks", admin(http.HandlerFunc(env.HandleAdminPlaybooks)))
	mux.Handle("/admin/playbooks/", admin(http.HandlerFunc(env.HandleAdminPlaybooks)))

	mux.Handle("/admin/clients", admin(http.HandlerFunc(env.HandleAdminClients)))
	mux.Handle("/admin/clients/", admin(http.HandlerFunc(env.HandleAdminClients)))

	mux.Handle("/admin/deployments", admin(http.HandlerFunc(env.HandleAdminDeployments)))
	mux.Handle("/admin/deployments/", admin(http.HandlerFunc(env.HandleAdminDeployments)))

	// Artifacts are accessible by both clients (JWT) and admins (Basic auth).
	// We use a dual-auth wrapper that accepts either credential type.
	mux.Handle("/artifacts", dualAuth(env, http.HandlerFunc(env.HandleArtifacts)))
	mux.Handle("/artifacts/", dualAuth(env, http.HandlerFunc(env.HandleArtifacts)))

	mux.Handle("/secrets", admin(http.HandlerFunc(env.HandleSecrets)))
	mux.Handle("/secrets/", admin(http.HandlerFunc(env.HandleSecrets)))

	return logging(cors(mux))
}

// dualAuth accepts requests authenticated with either a client JWT Bearer
// token or admin HTTP Basic credentials.
func dualAuth(env *handlers.Env, next http.Handler) http.Handler {
	clientMW := env.RequireClientAuth(next)
	adminMW := env.RequireAdminAuth(next)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, _, ok := r.BasicAuth(); ok {
			adminMW.ServeHTTP(w, r)
		} else {
			clientMW.ServeHTTP(w, r)
		}
	})
}

// cors adds permissive CORS headers (appropriate for private network use).
func cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
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
