// Package daemon assembles the HTTP server for the sear daemon.
package daemon

import (
	"net/http"

	"github.com/marko-stanojevic/sear/internal/daemon/handlers"
)

// NewServer wires all HTTP routes and returns a ready-to-use http.Handler.
func NewServer(env *handlers.Env) http.Handler {
	mux := http.NewServeMux()

	// ---- Public (no auth) --------------------------------------------------
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	// ---- Client API (JWT auth) ---------------------------------------------
	clientMW := env.RequireClientAuth

	mux.Handle("/api/v1/register", http.HandlerFunc(env.HandleRegister))
	mux.Handle("/api/v1/connect", clientMW(http.HandlerFunc(env.HandleConnect)))
	mux.Handle("/api/v1/state", clientMW(http.HandlerFunc(env.HandleStateUpdate)))
	mux.Handle("/api/v1/logs", clientMW(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			env.HandleLogUpload(w, r)
		} else {
			env.HandleGetLogs(w, r)
		}
	})))

	// ---- Admin API (basic auth) --------------------------------------------
	adminMW := env.RequireAdminAuth

	mux.Handle("/status", adminMW(http.HandlerFunc(env.HandleStatus)))
	mux.Handle("/admin/playbooks", adminMW(http.HandlerFunc(env.HandleAdminPlaybooks)))
	mux.Handle("/admin/playbooks/", adminMW(http.HandlerFunc(env.HandleAdminPlaybooks)))
	mux.Handle("/admin/clients", adminMW(http.HandlerFunc(env.HandleAdminClients)))
	mux.Handle("/admin/clients/", adminMW(http.HandlerFunc(env.HandleAdminClients)))
	mux.Handle("/admin/deployments", adminMW(http.HandlerFunc(env.HandleAdminDeployments)))
	mux.Handle("/admin/deployments/", adminMW(http.HandlerFunc(env.HandleAdminDeployments)))
	mux.Handle("/artifacts", adminMW(http.HandlerFunc(env.HandleArtifacts)))
	mux.Handle("/artifacts/", adminMW(http.HandlerFunc(env.HandleArtifacts)))
	mux.Handle("/secrets", adminMW(http.HandlerFunc(env.HandleSecrets)))
	mux.Handle("/secrets/", adminMW(http.HandlerFunc(env.HandleSecrets)))

	return mux
}
