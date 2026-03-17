package handlers

import "net/http"

// HandleDeploymentsUI serves the deployments page.
func (e *Handler) HandleDeploymentsUI(w http.ResponseWriter, r *http.Request) {
	renderUI(w, "deployments.html")
}
