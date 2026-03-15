package handlers

import (
	"embed"
	"net/http"
)

//go:embed ui/*.html
var uiFS embed.FS

func renderUI(w http.ResponseWriter, name string) {
	data, err := uiFS.ReadFile("ui/" + name)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load UI asset")
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("Expires", "0")
	_, _ = w.Write(data)
}
