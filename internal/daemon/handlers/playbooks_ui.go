package handlers

import "net/http"

// HandlePlaybooksUI serves the playbooks management web page.
func (e *Handler) HandlePlaybooksUI(w http.ResponseWriter, r *http.Request) {
	renderUI(w, "playbooks.html")
}
