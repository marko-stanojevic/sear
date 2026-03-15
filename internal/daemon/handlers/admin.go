package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/marko-stanojevic/sear/internal/common"
	"github.com/marko-stanojevic/sear/internal/daemon/service"
	"github.com/marko-stanojevic/sear/internal/daemon/store"
	"gopkg.in/yaml.v3"
)

// ── Status endpoint and UI ─────────────────────────────────────────────────────

// StatusResponse is returned by GET /api/v1/status (JSON) and used by the /ui status dashboard.
type StatusResponse struct {
	Clients     []*common.Client          `json:"clients"`
	Deployments []*common.DeploymentState `json:"deployments"`
}

// ── /api/v1/playbooks ─────────────────────────────────────────────────────────────

// HandleRootPlaybooks dispatches CRUD on playbooks.
//
//	GET    /api/v1/playbooks              – list all
//	POST   /api/v1/playbooks              – create
//	GET    /api/v1/playbooks/{id}         – get one
//	PUT    /api/v1/playbooks/{id}         – update
//	DELETE /api/v1/playbooks/{id}         – delete
//	POST   /api/v1/playbooks/{id}/assign  – assign to a client (pushes immediately
//	                                         if client is connected via WebSocket)
func (e *Env) HandleRootPlaybooks(w http.ResponseWriter, r *http.Request) {
	// Strip prefix to isolate the path tail.
	tail := strings.TrimPrefix(r.URL.Path, "/api/v1/playbooks")
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
		playbookYAML, err := yaml.Marshal(pb.Playbook)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to render playbook YAML")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"id":            pb.ID,
			"name":          pb.Name,
			"description":   pb.Description,
			"playbook":      pb.Playbook,
			"playbook_yaml": string(playbookYAML),
			"created_at":    pb.CreatedAt,
			"updated_at":    pb.UpdatedAt,
		})

	case http.MethodPost:
		if id != "" && sub == "assign" {
			e.assignPlaybook(w, r, id)
			return
		}
		rec, err := decodePlaybookWritePayload(r)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		if rec.Playbook == nil {
			writeError(w, http.StatusBadRequest, "playbook or playbook_yaml field is required")
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
		updated, err := decodePlaybookWritePayload(r)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		if updated.Playbook == nil {
			writeError(w, http.StatusBadRequest, "playbook or playbook_yaml field is required")
			return
		}
		if len(updated.Playbook.Jobs) == 0 {
			writeError(w, http.StatusBadRequest, "playbook must contain at least one job")
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

func decodePlaybookWritePayload(r *http.Request) (store.PlaybookRecord, error) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return store.PlaybookRecord{}, fmt.Errorf("failed to read request body: %w", err)
	}

	var in struct {
		Name         string           `json:"name"`
		Description  string           `json:"description"`
		Playbook     *common.Playbook `json:"playbook"`
		PlaybookYAML string           `json:"playbook_yaml"`
	}
	if err := json.Unmarshal(body, &in); err != nil {
		return store.PlaybookRecord{}, fmt.Errorf("invalid JSON: %w", err)
	}

	out := store.PlaybookRecord{
		Name:        in.Name,
		Description: in.Description,
		Playbook:    in.Playbook,
	}

	if strings.TrimSpace(in.PlaybookYAML) != "" {
		var playbook common.Playbook
		if err := yaml.Unmarshal([]byte(in.PlaybookYAML), &playbook); err != nil {
			return store.PlaybookRecord{}, fmt.Errorf("invalid YAML in playbook_yaml: %w", err)
		}
		out.Playbook = &playbook
	}

	if out.Name == "" && out.Playbook != nil {
		out.Name = out.Playbook.Name
	}

	return out, nil
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
	if e.Service == nil {
		writeError(w, http.StatusInternalServerError, "service not configured")
		return
	}
	if err := e.Service.AssignPlaybookToClient(playbookID, body.ClientID); err != nil {
		if errors.Is(err, service.ErrClientNotFound) || errors.Is(err, service.ErrPlaybookNotFound) {
		switch {
		case errors.Is(err, service.ErrClientNotFound), errors.Is(err, service.ErrPlaybookNotFound):
			writeError(w, http.StatusNotFound, err.Error())
		} else {
			writeError(w, http.StatusInternalServerError, err.Error())
		}
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "assigned"})
}

// ── /api/v1/clients ───────────────────────────────────────────────────────────────

// HandleRootClients dispatches CRUD on clients.
//
//	GET    /api/v1/clients          – list all
//	GET    /api/v1/clients/{id}     – get one
//	PUT    /api/v1/clients/{id}     – update (e.g. assign playbook, set status)
//	DELETE /api/v1/clients/{id}     – delete
func (e *Env) HandleRootClients(w http.ResponseWriter, r *http.Request) {
	tail := strings.TrimPrefix(r.URL.Path, "/api/v1/clients")
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

// ── /api/v1/deployments ───────────────────────────────────────────────────────────

// HandleRootDeployments lists deployments and exposes per-deployment logs.
//
//	GET /api/v1/deployments              – list all
//	GET /api/v1/deployments/{id}         – get one
//	GET /api/v1/deployments/{id}/logs    – get logs for deployment
func (e *Env) HandleRootDeployments(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/deployments")
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
