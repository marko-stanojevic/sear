// Package handlers implements the HTTP request handlers for the sear daemon.
package handlers

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"

	"github.com/marko-stanojevic/sear/internal/common"
	"github.com/marko-stanojevic/sear/internal/daemon/store"
)

// Env bundles the dependencies shared by all handlers.
type Env struct {
	Store            *store.Store
	JWTSecret        []byte
	RootPassword     string
	TokenExpiryHours int
	ArtifactsDir     string
	ServerURL        string
	// RegistrationSecrets maps secret-name → secret-value used during
	// client registration.
	RegistrationSecrets map[string]string
}

// ---- helpers ---------------------------------------------------------------

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]string{"error": msg})
}

// issueToken signs a JWT for the given client ID.
func (e *Env) issueToken(clientID string) (string, error) {
	expiry := time.Duration(e.TokenExpiryHours) * time.Hour
	if expiry == 0 {
		expiry = 720 * time.Hour
	}
	claims := jwt.RegisteredClaims{
		Subject:   clientID,
		IssuedAt:  jwt.NewNumericDate(time.Now()),
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(expiry)),
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return tok.SignedString(e.JWTSecret)
}

// clientIDFromToken validates the Bearer token and returns the client ID.
func (e *Env) clientIDFromToken(r *http.Request) (string, error) {
	auth := r.Header.Get("Authorization")
	if !strings.HasPrefix(auth, "Bearer ") {
		return "", fmt.Errorf("missing bearer token")
	}
	raw := strings.TrimPrefix(auth, "Bearer ")
	tok, err := jwt.ParseWithClaims(raw, &jwt.RegisteredClaims{},
		func(_ *jwt.Token) (any, error) { return e.JWTSecret, nil },
		jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}),
	)
	if err != nil {
		return "", fmt.Errorf("invalid token: %w", err)
	}
	claims, ok := tok.Claims.(*jwt.RegisteredClaims)
	if !ok || !tok.Valid {
		return "", fmt.Errorf("invalid token claims")
	}
	return claims.Subject, nil
}

// requireClientAuth is middleware that validates the client JWT and sets the
// "X-Client-ID" header for downstream handlers.
func (e *Env) RequireClientAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		clientID, err := e.clientIDFromToken(r)
		if err != nil {
			writeError(w, http.StatusUnauthorized, err.Error())
			return
		}
		// Refresh last-seen timestamp.
		if c, ok := e.Store.GetClient(clientID); ok {
			c.LastSeenAt = time.Now()
			if c.Status == common.ClientStatusOffline {
				c.Status = common.ClientStatusConnected
			}
			_ = e.Store.SaveClient(c)
		}
		r.Header.Set("X-Client-ID", clientID)
		next.ServeHTTP(w, r)
	})
}

// RequireAdminAuth is middleware that validates the root password via
// Basic Auth (user "admin", password = root password).
func (e *Env) RequireAdminAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, pass, ok := r.BasicAuth()
		if !ok || subtle.ConstantTimeCompare([]byte(pass), []byte(e.RootPassword)) != 1 {
			w.Header().Set("WWW-Authenticate", `Basic realm="sear-admin"`)
			writeError(w, http.StatusUnauthorized, "invalid admin credentials")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// GenerateSecret produces a cryptographically random hex string.
func GenerateSecret(bytes int) (string, error) {
	b := make([]byte, bytes)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// ---- Registration ----------------------------------------------------------

// HandleRegister processes POST /api/v1/register.
func (e *Env) HandleRegister(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var req common.RegistrationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	if req.PlatformID == "" || req.Hostname == "" {
		writeError(w, http.StatusBadRequest, "platform_id and hostname are required")
		return
	}
	// Validate registration secret.
	if !e.validRegistrationSecret(req.RegistrationSecret) {
		writeError(w, http.StatusUnauthorized, "invalid registration secret")
		return
	}
	// Re-use existing client record if the same platform_id re-registers.
	var client *common.Client
	for _, c := range e.Store.ListClients() {
		if c.PlatformID == req.PlatformID {
			client = c
			break
		}
	}
	if client == nil {
		client = &common.Client{
			ID:           uuid.New().String(),
			RegisteredAt: time.Now(),
		}
	}
	client.Hostname = req.Hostname
	client.Platform = req.Platform
	client.PlatformID = req.PlatformID
	client.Metadata = req.Metadata
	client.Status = common.ClientStatusRegistered
	client.LastSeenAt = time.Now()

	if err := e.Store.SaveClient(client); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to save client")
		return
	}
	token, err := e.issueToken(client.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to issue token")
		return
	}
	writeJSON(w, http.StatusOK, common.RegistrationResponse{
		ClientID: client.ID,
		Token:    token,
	})
}

func (e *Env) validRegistrationSecret(s string) bool {
	if s == "" {
		return false
	}
	for _, v := range e.RegistrationSecrets {
		if subtle.ConstantTimeCompare([]byte(s), []byte(v)) == 1 {
			return true
		}
	}
	return false
}

// ---- Connect ---------------------------------------------------------------

// HandleConnect processes GET /api/v1/connect.
// The client polls this endpoint; the server returns the next action.
func (e *Env) HandleConnect(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	clientID := r.Header.Get("X-Client-ID")
	client, ok := e.Store.GetClient(clientID)
	if !ok {
		writeError(w, http.StatusNotFound, "client not found")
		return
	}
	client.Status = common.ClientStatusConnected
	client.LastSeenAt = time.Now()
	_ = e.Store.SaveClient(client)

	dep, hasDep := e.Store.GetDeploymentForClient(clientID)
	if !hasDep || dep.Status == common.DeploymentStatusDone || dep.Status == common.DeploymentStatusFailed {
		// Check whether a playbook is assigned to this client.
		if client.PlaybookID == "" {
			writeJSON(w, http.StatusOK, common.ConnectResponse{Action: "wait"})
			return
		}
		// Start a new deployment.
		pb, ok := e.Store.GetPlaybook(client.PlaybookID)
		if !ok {
			writeJSON(w, http.StatusOK, common.ConnectResponse{Action: "wait"})
			return
		}
		dep = &common.DeploymentState{
			ID:               uuid.New().String(),
			ClientID:         clientID,
			PlaybookID:       client.PlaybookID,
			Status:           common.DeploymentStatusPending,
			CurrentJobName:   "",
			CurrentStepIndex: 0,
			StartedAt:        time.Now(),
			UpdatedAt:        time.Now(),
		}
		if err := e.Store.SaveDeployment(dep); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to create deployment")
			return
		}
		client.Status = common.ClientStatusDeploying
		_ = e.Store.SaveClient(client)
		writeJSON(w, http.StatusOK, e.buildDeployResponse(dep, pb))
		return
	}

	// Active or rebooting deployment: resume.
	if dep.Status == common.DeploymentStatusRebooting || dep.Status == common.DeploymentStatusRunning || dep.Status == common.DeploymentStatusPending {
		pb, ok := e.Store.GetPlaybook(dep.PlaybookID)
		if !ok {
			writeJSON(w, http.StatusOK, common.ConnectResponse{Action: "wait"})
			return
		}
		client.Status = common.ClientStatusDeploying
		_ = e.Store.SaveClient(client)
		writeJSON(w, http.StatusOK, e.buildDeployResponse(dep, pb))
		return
	}

	writeJSON(w, http.StatusOK, common.ConnectResponse{Action: "wait"})
}

func (e *Env) buildDeployResponse(dep *common.DeploymentState, pb *store.PlaybookRecord) common.ConnectResponse {
	return common.ConnectResponse{
		Action:           "deploy",
		DeploymentID:     dep.ID,
		PlaybookID:       dep.PlaybookID,
		Playbook:         pb.Playbook,
		ResumeJobName:    dep.CurrentJobName,
		ResumeStepIndex:  dep.CurrentStepIndex,
		Secrets:          e.Store.AllSecrets(),
		ArtifactsBaseURL: e.ServerURL + "/artifacts",
	}
}

// ---- State -----------------------------------------------------------------

// HandleStateUpdate processes POST /api/v1/state.
func (e *Env) HandleStateUpdate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var req common.StateUpdateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	dep, ok := e.Store.GetDeployment(req.DeploymentID)
	if !ok {
		writeError(w, http.StatusNotFound, "deployment not found")
		return
	}
	dep.Status = req.Status
	dep.CurrentJobName = req.CurrentJobName
	dep.CurrentStepIndex = req.CurrentStepIndex
	dep.ErrorDetail = req.ErrorDetail
	dep.UpdatedAt = time.Now()
	if req.Status == common.DeploymentStatusDone || req.Status == common.DeploymentStatusFailed {
		now := time.Now()
		dep.FinishedAt = &now
		// Update client status.
		if c, ok := e.Store.GetClient(dep.ClientID); ok {
			if req.Status == common.DeploymentStatusDone {
				c.Status = common.ClientStatusDone
			} else {
				c.Status = common.ClientStatusFailed
			}
			_ = e.Store.SaveClient(c)
		}
	}
	if err := e.Store.SaveDeployment(dep); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to save deployment")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// ---- Logs ------------------------------------------------------------------

// HandleLogUpload processes POST /api/v1/logs.
func (e *Env) HandleLogUpload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var batch common.LogBatch
	if err := json.NewDecoder(r.Body).Decode(&batch); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	ptrs := make([]*common.LogEntry, len(batch.Entries))
	for i := range batch.Entries {
		e := batch.Entries[i]
		ptrs[i] = &e
	}
	if err := e.Store.AppendLogs(ptrs); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to store logs")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// HandleGetLogs processes GET /api/v1/logs?deployment_id=...
func (e *Env) HandleGetLogs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	depID := r.URL.Query().Get("deployment_id")
	if depID == "" {
		writeError(w, http.StatusBadRequest, "deployment_id is required")
		return
	}
	logs := e.Store.GetLogsForDeployment(depID)
	writeJSON(w, http.StatusOK, logs)
}
