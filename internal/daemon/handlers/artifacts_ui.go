package handlers

import "net/http"

// HandleArtifactsUI serves the artifacts page.
func (e *Env) HandleArtifactsUI(w http.ResponseWriter, r *http.Request) {
	renderUI(w, "artifacts.html")
}
