package handlers

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/marko-stanojevic/sear/internal/common"
)

// HandleArtifacts dispatches artifact CRUD + download/upload.
//
//	GET    /artifacts              – list artifacts
//	GET    /artifacts/{id}         – download artifact file
//	GET    /artifacts/{id}/meta    – get artifact metadata
//	POST   /artifacts              – upload artifact (multipart or raw body)
//	DELETE /artifacts/{id}         – delete artifact
func (e *Env) HandleArtifacts(w http.ResponseWriter, r *http.Request) {
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
		// Download file.
		a, ok := e.Store.GetArtifact(id)
		if !ok {
			// Try lookup by name.
			a, ok = e.Store.GetArtifactByName(id)
			if !ok {
				writeError(w, http.StatusNotFound, "artifact not found")
				return
			}
		}
		filePath := filepath.Join(e.ArtifactsDir, a.ID, a.Filename)
		f, err := os.Open(filePath)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "artifact file missing")
			return
		}
		defer f.Close()
		ct := a.ContentType
		if ct == "" {
			ct = "application/octet-stream"
		}
		w.Header().Set("Content-Type", ct)
		w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, a.Filename))
		http.ServeContent(w, r, a.Filename, time.Now(), f)

	case http.MethodPost:
		name := r.URL.Query().Get("name")
		if name == "" {
			writeError(w, http.StatusBadRequest, "name query parameter required")
			return
		}
		contentType := r.Header.Get("Content-Type")
		if contentType == "" {
			contentType = "application/octet-stream"
		}
		artID := uuid.New().String()
		artDir := filepath.Join(e.ArtifactsDir, artID)
		if err := os.MkdirAll(artDir, 0o700); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to create artifact dir")
			return
		}
		filename := name
		destPath := filepath.Join(artDir, filename)
		f, err := os.Create(destPath)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to create artifact file")
			return
		}
		size, copyErr := io.Copy(f, r.Body)
		f.Close()
		if copyErr != nil {
			writeError(w, http.StatusInternalServerError, "failed to write artifact")
			return
		}
		art := &common.Artifact{
			ID:          artID,
			Name:        name,
			Filename:    filename,
			Size:        size,
			ContentType: contentType,
			UploadedAt:  time.Now(),
		}
		if err := e.Store.SaveArtifact(art); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to save artifact metadata")
			return
		}
		writeJSON(w, http.StatusCreated, art)

	case http.MethodDelete:
		if id == "" {
			writeError(w, http.StatusBadRequest, "artifact ID required")
			return
		}
		a, ok := e.Store.GetArtifact(id)
		if !ok {
			writeError(w, http.StatusNotFound, "artifact not found")
			return
		}
		artDir := filepath.Join(e.ArtifactsDir, a.ID)
		_ = os.RemoveAll(artDir)
		if err := e.Store.DeleteArtifact(id); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to delete artifact")
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})

	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}
