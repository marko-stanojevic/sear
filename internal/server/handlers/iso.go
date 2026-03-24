package handlers

import (
	"encoding/json"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/marko-stanojevic/kompakt/internal/iso"
)

// HandleISOBuild dispatches ISO-build API requests.
//
//	POST   /api/v1/iso/build               — start a new build
//	GET    /api/v1/iso/build               — list all builds
//	GET    /api/v1/iso/build/{id}          — poll status + logs (supports ?offset=N)
//	GET    /api/v1/iso/build/{id}/download — download finished ISO
//	DELETE /api/v1/iso/build/{id}          — delete build and its ISO file
func (e *Handler) HandleISOBuild(w http.ResponseWriter, r *http.Request) {
	tail := strings.TrimPrefix(r.URL.Path, "/api/v1/iso/build")
	tail = strings.TrimPrefix(tail, "/")
	parts := strings.SplitN(tail, "/", 2)
	buildID := parts[0]
	sub := ""
	if len(parts) > 1 {
		sub = parts[1]
	}

	switch {
	case r.Method == http.MethodPost && buildID == "":
		e.startISOBuild(w, r)
	case r.Method == http.MethodGet && buildID == "":
		e.listISOBuilds(w, r)
	case r.Method == http.MethodGet && sub == "download":
		e.downloadISO(w, r, buildID)
	case r.Method == http.MethodGet && buildID != "":
		e.pollISOBuild(w, r, buildID)
	case r.Method == http.MethodDelete && buildID != "" && sub == "":
		e.deleteIsoBuild(w, r, buildID)
	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (e *Handler) startISOBuild(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ServerURL                   string `json:"server_url"`
		SecretName                  string `json:"secret_name"`
		TLSSkipVerify               bool   `json:"tls_skip_verify"`
		CustomName                  string `json:"custom_name"`
		ExtraDockerfileInstructions string `json:"extra_dockerfile_instructions"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	body.ServerURL = strings.TrimSpace(body.ServerURL)
	if body.ServerURL == "" {
		writeError(w, http.StatusBadRequest, "server_url is required")
		return
	}
	if body.SecretName == "" {
		body.SecretName = "default"
	}

	secretValue, ok := e.RegistrationSecrets[body.SecretName]
	if !ok {
		writeError(w, http.StatusBadRequest, "registration secret '"+body.SecretName+"' not found")
		return
	}

	build, err := e.Service.StartISOBuild(body.ServerURL, body.SecretName, secretValue, body.CustomName, body.ExtraDockerfileInstructions, body.TLSSkipVerify)
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, err.Error())
		return
	}

	writeJSON(w, http.StatusAccepted, map[string]string{"build_id": build.ID})
}

func (e *Handler) listISOBuilds(w http.ResponseWriter, r *http.Request) {
	builds := e.ISOBuilds.List()
	snapshots := make([]iso.BuildSnapshot, len(builds))
	for i, b := range builds {
		snapshots[i] = b.Snapshot()
	}
	writeJSON(w, http.StatusOK, snapshots)
}

func (e *Handler) pollISOBuild(w http.ResponseWriter, r *http.Request, buildID string) {
	build, ok := e.ISOBuilds.Get(buildID)
	if !ok {
		writeError(w, http.StatusNotFound, "build not found")
		return
	}
	snap := build.Snapshot()
	// offset allows the client to fetch only new log lines
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	if offset > 0 && offset < len(snap.Logs) {
		snap.Logs = snap.Logs[offset:]
	} else if offset >= len(snap.Logs) {
		snap.Logs = nil
	}
	writeJSON(w, http.StatusOK, snap)
}

func (e *Handler) downloadISO(w http.ResponseWriter, r *http.Request, buildID string) {
	build, ok := e.ISOBuilds.Get(buildID)
	if !ok {
		writeError(w, http.StatusNotFound, "build not found")
		return
	}
	snap := build.Snapshot()
	if snap.Status != iso.BuildStatusCompleted || !snap.HasISO {
		writeError(w, http.StatusConflict, "ISO is not ready yet")
		return
	}
	isoPath := build.GetISOPath()
	if isoPath == "" {
		writeError(w, http.StatusInternalServerError, "ISO path missing")
		return
	}
	w.Header().Set("Content-Disposition", `attachment; filename="`+filepath.Base(isoPath)+`"`)
	w.Header().Set("Content-Type", "application/octet-stream")
	http.ServeFile(w, r, isoPath)
}

func (e *Handler) deleteIsoBuild(w http.ResponseWriter, _ *http.Request, buildID string) {
	if !e.ISOBuilds.Delete(buildID) {
		writeError(w, http.StatusNotFound, "build not found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
