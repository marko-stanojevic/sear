package handlers

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/marko-stanojevic/sear/internal/common"
	"github.com/marko-stanojevic/sear/internal/daemon/store"
)

// ── GET /status ───────────────────────────────────────────────────────────────

// StatusResponse is returned by GET /status (JSON) and GET /status/ui (rendered).
type StatusResponse struct {
	Clients     []*common.Client          `json:"clients"`
	Deployments []*common.DeploymentState `json:"deployments"`
}

// ── /admin/playbooks ──────────────────────────────────────────────────────────

// HandleAdminPlaybooks dispatches CRUD on playbooks.
//
//	GET    /admin/playbooks              – list all
//	POST   /admin/playbooks              – create
//	GET    /admin/playbooks/{id}         – get one
//	PUT    /admin/playbooks/{id}         – update
//	DELETE /admin/playbooks/{id}         – delete
//	POST   /admin/playbooks/{id}/assign  – assign to a client (pushes immediately
//	                                       if client is connected via WebSocket)
func (e *Env) HandleAdminPlaybooks(w http.ResponseWriter, r *http.Request) {
	// Strip prefix to isolate the path tail.
	tail := strings.TrimPrefix(r.URL.Path, "/admin/playbooks")
	tail = strings.TrimPrefix(tail, "/")
	parts := strings.SplitN(tail, "/", 2)
	id := parts[0]
	sub := ""
	if len(parts) == 2 {
		sub = parts[1]
	}

	switch r.Method {
	case http.MethodGet:
		if id == "" {
			writeJSON(w, http.StatusOK, e.Store.ListPlaybooks())
			return
		}
		pb, ok := e.Store.GetPlaybook(id)
		if !ok {
			writeError(w, http.StatusNotFound, "playbook not found")
			return
		}
		writeJSON(w, http.StatusOK, pb)

	case http.MethodPost:
		if id != "" && sub == "assign" {
			e.assignPlaybook(w, r, id)
			return
		}
		var rec store.PlaybookRecord
		if err := json.NewDecoder(r.Body).Decode(&rec); err != nil {
			writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
			return
		}
		if rec.Playbook == nil {
			writeError(w, http.StatusBadRequest, "playbook field is required")
			return
		}
		if len(rec.Playbook.Jobs) == 0 {
			writeError(w, http.StatusBadRequest, "playbook must contain at least one job")
			return
		}
		rec.ID = uuid.New().String()
		rec.CreatedAt = time.Now()
		rec.UpdatedAt = time.Now()
		if err := e.Store.SavePlaybook(&rec); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to save playbook")
			return
		}
		writeJSON(w, http.StatusCreated, rec)

	case http.MethodPut:
		if id == "" {
			writeError(w, http.StatusBadRequest, "playbook ID required in path")
			return
		}
		existing, ok := e.Store.GetPlaybook(id)
		if !ok {
			writeError(w, http.StatusNotFound, "playbook not found")
			return
		}
		var updated store.PlaybookRecord
		if err := json.NewDecoder(r.Body).Decode(&updated); err != nil {
			writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
			return
		}
		updated.ID = existing.ID
		updated.CreatedAt = existing.CreatedAt
		updated.UpdatedAt = time.Now()
		if err := e.Store.SavePlaybook(&updated); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to update playbook")
			return
		}
		writeJSON(w, http.StatusOK, updated)

	case http.MethodDelete:
		if id == "" {
			writeError(w, http.StatusBadRequest, "playbook ID required in path")
			return
		}
		if err := e.Store.DeletePlaybook(id); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to delete playbook")
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})

	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

// assignPlaybook assigns a playbook to a client and immediately pushes it
// if the client is connected via WebSocket.
func (e *Env) assignPlaybook(w http.ResponseWriter, r *http.Request, playbookID string) {
	var body struct {
		ClientID string `json:"client_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	client, ok := e.Store.GetClient(body.ClientID)
	if !ok {
		writeError(w, http.StatusNotFound, "client not found")
		return
	}
	if _, ok := e.Store.GetPlaybook(playbookID); !ok {
		writeError(w, http.StatusNotFound, "playbook not found")
		return
	}
	client.PlaybookID = playbookID
	if err := e.Store.SaveClient(client); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to save client")
		return
	}
	// If the client is connected via WebSocket, push the playbook now.
	if e.Hub.IsConnected(body.ClientID) {
		e.pushPlaybookIfAssigned(body.ClientID)
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "assigned"})
}

// ── /admin/clients ────────────────────────────────────────────────────────────

// HandleAdminClients dispatches CRUD on clients.
//
//	GET    /admin/clients          – list all
//	GET    /admin/clients/{id}     – get one
//	PUT    /admin/clients/{id}     – update (e.g. assign playbook, set status)
//	DELETE /admin/clients/{id}     – delete
func (e *Env) HandleAdminClients(w http.ResponseWriter, r *http.Request) {
	tail := strings.TrimPrefix(r.URL.Path, "/admin/clients")
	tail = strings.TrimPrefix(tail, "/")
	id := tail

	switch r.Method {
	case http.MethodGet:
		if id == "" {
			writeJSON(w, http.StatusOK, e.Store.ListClients())
			return
		}
		c, ok := e.Store.GetClient(id)
		if !ok {
			writeError(w, http.StatusNotFound, "client not found")
			return
		}
		writeJSON(w, http.StatusOK, c)

	case http.MethodPut:
		if id == "" {
			writeError(w, http.StatusBadRequest, "client ID required in path")
			return
		}
		existing, ok := e.Store.GetClient(id)
		if !ok {
			writeError(w, http.StatusNotFound, "client not found")
			return
		}
		var patch struct {
			PlaybookID string              `json:"playbook_id"`
			Status     common.ClientStatus `json:"status"`
		}
		if err := json.NewDecoder(r.Body).Decode(&patch); err != nil {
			writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
			return
		}
		if patch.PlaybookID != "" {
			existing.PlaybookID = patch.PlaybookID
		}
		if patch.Status != "" {
			existing.Status = patch.Status
		}
		if err := e.Store.SaveClient(existing); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to update client")
			return
		}
		writeJSON(w, http.StatusOK, existing)

	case http.MethodDelete:
		if id == "" {
			writeError(w, http.StatusBadRequest, "client ID required in path")
			return
		}
		if err := e.Store.DeleteClient(id); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to delete client")
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})

	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

// ── /admin/deployments ────────────────────────────────────────────────────────

// HandleAdminDeployments lists deployments and exposes per-deployment logs.
//
//	GET /admin/deployments              – list all
//	GET /admin/deployments/{id}         – get one
//	GET /admin/deployments/{id}/logs    – get logs for deployment
func (e *Env) HandleAdminDeployments(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	path := strings.TrimPrefix(r.URL.Path, "/admin/deployments")
	path = strings.TrimPrefix(path, "/")
	parts := strings.SplitN(path, "/", 2)
	id := parts[0]

	if id == "" {
		writeJSON(w, http.StatusOK, e.Store.ListDeployments())
		return
	}
	if len(parts) == 2 && parts[1] == "logs" {
		writeJSON(w, http.StatusOK, e.Store.GetLogsForDeployment(id))
		return
	}
	dep, ok := e.Store.GetDeployment(id)
	if !ok {
		writeError(w, http.StatusNotFound, "deployment not found")
		return
	}
	writeJSON(w, http.StatusOK, dep)
}
