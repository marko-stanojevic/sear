package handlers

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/marko-stanojevic/kompakt/internal/common"
)

// HandleArtifacts dispatches artifact CRUD and download/upload.
//
//	GET    /artifacts              – list artifacts
//	GET    /artifacts/{id}         – download file (or look up by name if not UUID)
//	GET    /artifacts/{id}/meta    – get metadata only
//	POST   /artifacts?name=foo     – upload (raw request body)
//	DELETE /artifacts/{id}         – delete artifact
//
// Uploading uses a raw request body rather than multipart to keep CLI usage
// simple:  curl -T myapp http://daemon/artifacts?name=myapp
func (e *Handler) HandleArtifacts(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/artifacts")
	path = strings.TrimPrefix(path, "/")
	parts := strings.SplitN(path, "/", 2)
	id := parts[0]
	sub := ""
	if len(parts) == 2 {
		sub = parts[1]
	}

	switch r.Method {
	case http.MethodGet:
		if id == "" {
			// Listing requires authentication (root or any client)
			if !e.isRootRequest(r) {
				if _, err := e.agentIDFromToken(r); err != nil {
					writeError(w, http.StatusUnauthorized, "authentication required to list artifacts")
					return
				}
			}
			writeJSON(w, http.StatusOK, e.Store.ListArtifacts())
			return
		}
		if sub == "meta" {
			a, ok := e.Store.GetArtifact(id)
			if !ok {
				writeError(w, http.StatusNotFound, "artifact not found")
				return
			}
			writeJSON(w, http.StatusOK, a)
			return
		}
		// Download: try by ID first, then fall back to name lookup.
		a, ok := e.Store.GetArtifact(id)
		if !ok {
			a, ok = e.Store.GetArtifactByName(id)
			if !ok {
				writeError(w, http.StatusNotFound, "artifact not found")
				return
			}
		}

		// Enforce Access Policy
		if a.AccessPolicy != common.AccessPublic {
			// Check for root auth first
			if !e.isRootRequest(r) {
				// Check for agent auth
				agentID, err := e.agentIDFromToken(r)
				if err != nil {
					writeError(w, http.StatusUnauthorized, "authenticated access required")
					return
				}
				if a.AccessPolicy == common.AccessRestricted {
					allowed := false
					for _, aid := range a.AllowedAgents {
						if aid == agentID {
							allowed = true
							break
						}
					}
					if !allowed {
						writeError(w, http.StatusForbidden, "access to this artifact is restricted")
						return
					}
				}
			}
		}

		filePath := filepath.Join(e.ArtifactsDir, a.ID, a.Filename)
		f, err := os.Open(filePath)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "artifact file missing on server")
			return
		}
		defer func() { _ = f.Close() }()
		ct := a.ContentType
		if ct == "" {
			ct = "application/octet-stream"
		}
		w.Header().Set("Content-Type", ct)
		w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, a.Filename))
		http.ServeContent(w, r, a.Filename, time.Now(), f)

	case http.MethodPost:
		// Upload requires authentication (root or any client)
		if !e.isRootRequest(r) {
			if _, err := e.agentIDFromToken(r); err != nil {
				writeError(w, http.StatusUnauthorized, "authentication required to upload artifacts")
				return
			}
		}
		name := r.URL.Query().Get("name")
		filename := r.URL.Query().Get("filename")
		if filename == "" {
			filename = name
		}
		if name == "" {
			name = filename // fallback: if no user-supplied name, use filename
		}
		if filename == "" {
			writeError(w, http.StatusBadRequest, "'filename' query parameter is required")
			return
		}
		ct := r.Header.Get("Content-Type")
		if ct == "" {
			ct = "application/octet-stream"
		}
		artID := common.NewID()
		artDir := filepath.Join(e.ArtifactsDir, artID)
		if err := os.MkdirAll(artDir, 0o700); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to create artifact directory")
			return
		}
		destPath := filepath.Join(artDir, filename)
		f, err := os.Create(destPath)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to create artifact file")
			return
		}
		size, copyErr := io.Copy(f, r.Body)
		closeErr := f.Close()
		if copyErr != nil {
			writeError(w, http.StatusInternalServerError, "failed to write artifact")
			return
		}
		if closeErr != nil {
			writeError(w, http.StatusInternalServerError, "failed to write artifact")
			return
		}
		art := &common.Artifact{
			ID:             artID,
			Name:           name,
			Filename:       filename,
			Size:           size,
			ContentType:    ct,
			AccessPolicy:   common.AccessPolicy(r.URL.Query().Get("access_policy")),
			AllowedAgents: strings.Split(r.URL.Query().Get("allowed_agents"), ","),
			UploadedAt:     time.Now(),
		}
		if art.AccessPolicy == "" {
			art.AccessPolicy = common.AccessAuthenticated // default
		}
		if len(art.AllowedAgents) == 1 && art.AllowedAgents[0] == "" {
			art.AllowedAgents = nil
		}
		if err := e.Store.SaveArtifact(art); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to save artifact metadata")
			return
		}
		writeJSON(w, http.StatusCreated, art)

	case http.MethodPatch:
		// Update metadata requires authentication (root or any client)
		if !e.isRootRequest(r) {
			if _, err := e.agentIDFromToken(r); err != nil {
				writeError(w, http.StatusUnauthorized, "authentication required to modify artifacts")
				return
			}
		}
		if id == "" {
			writeError(w, http.StatusBadRequest, "artifact ID required in path")
			return
		}
		a, ok := e.Store.GetArtifact(id)
		if !ok {
			writeError(w, http.StatusNotFound, "artifact not found")
			return
		}

		policy := r.URL.Query().Get("access_policy")
		allowedClients := r.URL.Query().Get("allowed_agents")

		if policy != "" {
			a.AccessPolicy = common.AccessPolicy(policy)
		}
		if allowedClients != "" {
			clients := strings.Split(allowedClients, ",")
			var filtered []string
			for _, c := range clients {
				c = strings.TrimSpace(c)
				if c != "" {
					filtered = append(filtered, c)
				}
			}
			a.AllowedAgents = filtered
		} else if r.URL.Query().Has("allowed_agents") {
			a.AllowedAgents = nil
		}

		if err := e.Store.SaveArtifact(a); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to update artifact: "+err.Error())
			return
		}
		writeJSON(w, http.StatusOK, a)

	case http.MethodDelete:
		// Delete requires authentication (root or any client)
		if !e.isRootRequest(r) {
			if _, err := e.agentIDFromToken(r); err != nil {
				writeError(w, http.StatusUnauthorized, "authentication required to delete artifacts")
				return
			}
		}
		if id == "" {
			writeError(w, http.StatusBadRequest, "artifact ID required in path")
			return
		}
		a, ok := e.Store.GetArtifact(id)
		if !ok {
			writeError(w, http.StatusNotFound, "artifact not found")
			return
		}
		_ = os.RemoveAll(filepath.Join(e.ArtifactsDir, a.ID))
		if err := e.Store.DeleteArtifact(id); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to delete artifact")
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})

	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

