package handlers

import (
	"encoding/json"
	"net/http"
	"strings"
)

// HandleSecrets manages the server-side secrets store.
//
//	GET    /api/v1/secrets          – list secret names (values are never exposed in list)
//	GET    /api/v1/secrets/{name}   – get a specific secret value
//	PUT    /api/v1/secrets/{name}   – set or update a secret value
//	DELETE /api/v1/secrets/{name}   – delete a secret
func (e *Env) HandleSecrets(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimPrefix(r.URL.Path, "/api/v1/secrets")
	name = strings.TrimPrefix(name, "/")

	switch r.Method {
	case http.MethodGet:
		if name == "" {
			// List names only — never expose values via the list endpoint.
			writeJSON(w, http.StatusOK, e.Store.ListSecretNames())
			return
		}
		val, ok := e.Store.GetSecret(name)
		if !ok {
			writeError(w, http.StatusNotFound, "secret not found")
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"name": name, "value": val})

	case http.MethodPut:
		if name == "" {
			writeError(w, http.StatusBadRequest, "secret name required in path")
			return
		}
		var body struct {
			Value string `json:"value"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
			return
		}
		if err := e.Store.SetSecret(name, body.Value); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to store secret")
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})

	case http.MethodDelete:
		if name == "" {
			writeError(w, http.StatusBadRequest, "secret name required in path")
			return
		}
		if err := e.Store.DeleteSecret(name); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to delete secret")
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})

	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

// HandleSecretsUI serves the secrets management web page.
func (e *Env) HandleSecretsUI(w http.ResponseWriter, r *http.Request) {
	renderUI(w, "secrets.html")
}
