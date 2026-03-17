package handlers

import "net/http"

// HandleHomeUI returns the main dashboard entry point.
func (e *Handler) HandleHomeUI(w http.ResponseWriter, r *http.Request) {
	renderUI(w, "index.html")
}
