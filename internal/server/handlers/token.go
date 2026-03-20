package handlers

import "net/http"

// HandleTokenRefresh handles POST /api/v1/token/refresh.
// The agent presents its current token; a new token is issued and the old one
// is revoked atomically. This allows zero-downtime key rotation.
func (e *Handler) HandleTokenRefresh(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	agentID := r.Header.Get("X-Agent-ID")
	oldRaw := rawTokenFromRequest(r)

	newRaw, err := e.issueAgentToken(agentID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to issue token")
		return
	}

	// Revoke the old token after the new one is safely stored.
	if oldRaw != "" {
		if tok, err := e.Store.GetAgentTokenByHash(sha256hex(oldRaw)); err == nil {
			_ = e.Store.RevokeAgentToken(tok.ID)
		}
	}

	writeJSON(w, http.StatusOK, map[string]string{"token": newRaw})
}
