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

// ---- /status ---------------------------------------------------------------

// StatusResponse is the payload returned by GET /status.
type StatusResponse struct {
	Clients     []*common.Client          `json:"clients"`
	Deployments []*common.DeploymentState `json:"deployments"`
}

// HandleStatus processes GET /status.
func (e *Env) HandleStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	writeJSON(w, http.StatusOK, StatusResponse{
		Clients:     e.Store.ListClients(),
		Deployments: e.Store.ListDeployments(),
	})
}

// ---- /admin/playbooks -------------------------------------------------------

// HandleAdminPlaybooks dispatches CRUD operations on playbooks.
//
//	GET    /admin/playbooks          – list all
//	POST   /admin/playbooks          – create
//	GET    /admin/playbooks/{id}     – get one
//	PUT    /admin/playbooks/{id}     – update
//	DELETE /admin/playbooks/{id}     – delete
func (e *Env) HandleAdminPlaybooks(w http.ResponseWriter, r *http.Request) {
	// Extract optional ID from path.
	id := strings.TrimPrefix(r.URL.Path, "/admin/playbooks")
	id = strings.TrimPrefix(id, "/")

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
		var rec store.PlaybookRecord
		if err := json.NewDecoder(r.Body).Decode(&rec); err != nil {
			writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
			return
		}
		if rec.Playbook == nil {
			writeError(w, http.StatusBadRequest, "playbook field is required")
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

// ---- /admin/clients --------------------------------------------------------

// HandleAdminClients dispatches CRUD operations on clients.
//
//	GET    /admin/clients          – list all
//	GET    /admin/clients/{id}     – get one
//	PUT    /admin/clients/{id}     – update (e.g. assign playbook)
//	DELETE /admin/clients/{id}     – delete
func (e *Env) HandleAdminClients(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/admin/clients")
	id = strings.TrimPrefix(id, "/")

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

// HandleAdminDeployments handles GET /admin/deployments and
// GET /admin/deployments/{id}/logs.
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
		logs := e.Store.GetLogsForDeployment(id)
		writeJSON(w, http.StatusOK, logs)
		return
	}
	dep, ok := e.Store.GetDeployment(id)
	if !ok {
		writeError(w, http.StatusNotFound, "deployment not found")
		return
	}
	writeJSON(w, http.StatusOK, dep)
}
