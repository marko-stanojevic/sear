package handlers

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/marko-stanojevic/kompakt/internal/common"
)

// uiTokenAudience is the JWT audience claim used exclusively for UI session tokens.
// It prevents a UI token from being accepted as an agent token.
const uiTokenAudience = "kompakt-ui"

// ── Agent token helpers ───────────────────────────────────────────────────────

// issueAgentToken generates a cryptographically random opaque token for the
// given agent, stores a SHA-256 hash of it in the database, and returns the
// raw token. The raw token is never stored and is only returned once.
func (e *Handler) issueAgentToken(agentID string) (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generating token: %w", err)
	}
	raw := "kpkt_" + hex.EncodeToString(b)
	expiry := time.Duration(e.TokenExpiryHours) * time.Hour
	if expiry == 0 {
		expiry = 720 * time.Hour // 30 days default
	}
	exp := time.Now().Add(expiry)
	tok := &common.AgentToken{
		ID:        common.NewID(),
		AgentID:   agentID,
		TokenHash: sha256hex(raw),
		CreatedAt: time.Now(),
		ExpiresAt: &exp,
	}
	if err := e.Store.CreateAgentToken(tok); err != nil {
		return "", fmt.Errorf("storing token: %w", err)
	}
	return raw, nil
}

// sha256hex returns the SHA-256 hex digest of s.
func sha256hex(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}

// rawTokenFromRequest extracts the raw token string from the Authorization
// header ("Bearer <token>") or the "token" query parameter.
func rawTokenFromRequest(r *http.Request) string {
	if auth := r.Header.Get("Authorization"); strings.HasPrefix(auth, "Bearer ") {
		return strings.TrimPrefix(auth, "Bearer ")
	}
	return r.URL.Query().Get("token")
}

// agentIDFromToken looks up the hashed token in the database and returns the
// associated agent ID, or an error if the token is missing, unknown, revoked,
// or expired.
func (e *Handler) agentIDFromToken(r *http.Request) (string, error) {
	raw := rawTokenFromRequest(r)
	if raw == "" {
		return "", fmt.Errorf("authentication token is missing or invalid")
	}
	tok, err := e.Store.GetAgentTokenByHash(sha256hex(raw))
	if err != nil {
		return "", fmt.Errorf("invalid token")
	}
	if tok.RevokedAt != nil {
		return "", fmt.Errorf("token has been revoked")
	}
	if tok.ExpiresAt != nil && time.Now().After(*tok.ExpiresAt) {
		return "", fmt.Errorf("token has expired")
	}
	return tok.AgentID, nil
}

// ── Middleware ────────────────────────────────────────────────────────────────

// RequireAgentAuth validates the agent token and sets X-Agent-ID for
// downstream handlers. Also refreshes the agent's last-seen timestamp.
func (e *Handler) RequireAgentAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		agentID, err := e.agentIDFromToken(r)
		if err != nil {
			writeError(w, http.StatusUnauthorized, err.Error())
			return
		}
		if a, ok := e.Store.GetAgent(agentID); ok {
			a.LastActivityAt = time.Now()
			if a.Status == common.AgentStatusOffline {
				a.Status = common.AgentStatusConnected
			}
			_ = e.Store.SaveAgent(a)
		}
		r.Header.Set("X-Agent-ID", agentID)
		next.ServeHTTP(w, r)
	})
}

// ── UI JWT helpers ────────────────────────────────────────────────────────────

// issueUIToken signs a short-lived (8 h) JWT for a root UI session using the
// dedicated UI secret, keeping it independent from agent tokens.
func (e *Handler) issueUIToken() (string, error) {
	claims := jwt.RegisteredClaims{
		Subject:   "root",
		Audience:  jwt.ClaimStrings{uiTokenAudience},
		IssuedAt:  jwt.NewNumericDate(time.Now()),
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(8 * time.Hour)),
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return tok.SignedString(e.UserJWTSecret)
}

// isRootRequest returns true when the request carries valid root credentials —
// either HTTP Basic auth or a UI Bearer JWT issued by HandleUILogin.
func (e *Handler) isRootRequest(r *http.Request) bool {
	if auth := r.Header.Get("Authorization"); strings.HasPrefix(auth, "Bearer ") {
		raw := strings.TrimPrefix(auth, "Bearer ")
		tok, err := jwt.ParseWithClaims(raw, &jwt.RegisteredClaims{},
			func(_ *jwt.Token) (any, error) { return e.UserJWTSecret, nil },
			jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}),
			jwt.WithAudience(uiTokenAudience),
		)
		if err == nil {
			if claims, ok := tok.Claims.(*jwt.RegisteredClaims); ok && tok.Valid && claims.Subject == "root" {
				return true
			}
		}
	}
	user, pass, ok := r.BasicAuth()
	return ok &&
		subtle.ConstantTimeCompare([]byte(user), []byte("root")) == 1 &&
		subtle.ConstantTimeCompare([]byte(pass), []byte(e.RootPassword)) == 1
}

// RequireRootAuth enforces authentication for root/admin endpoints.
// It accepts either:
//   - HTTP Basic auth (username "root" + configured root password), or
//   - Bearer JWT issued by HandleUILogin.
func (e *Handler) RequireRootAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if auth := r.Header.Get("Authorization"); strings.HasPrefix(auth, "Bearer ") {
			raw := strings.TrimPrefix(auth, "Bearer ")
			tok, err := jwt.ParseWithClaims(raw, &jwt.RegisteredClaims{},
				func(_ *jwt.Token) (any, error) { return e.UserJWTSecret, nil },
				jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}),
				jwt.WithAudience(uiTokenAudience),
			)
			if err == nil {
				if claims, ok := tok.Claims.(*jwt.RegisteredClaims); ok && tok.Valid && claims.Subject == "root" {
					next.ServeHTTP(w, r)
					return
				}
			}
		}
		user, pass, ok := r.BasicAuth()
		if !ok ||
			subtle.ConstantTimeCompare([]byte(user), []byte("root")) != 1 ||
			subtle.ConstantTimeCompare([]byte(pass), []byte(e.RootPassword)) != 1 {
			// Only suppress WWW-Authenticate when the client already presented a
			// Bearer token (e.g. an expired UI JWT). In that case the browser
			// would pop a native Basic auth dialog before the JS handler can
			// show the login modal. For requests with no auth or with failed
			// Basic auth we still advertise the scheme.
			if !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ") {
				w.Header().Set("WWW-Authenticate", `Basic realm="kompakt-root"`)
			}
			writeError(w, http.StatusUnauthorized, "invalid root credentials")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// HandleUILogin processes POST /api/v1/ui/login.
// Validates the root password and returns a short-lived JWT so the browser
// never has to store the raw password in sessionStorage.
func (e *Handler) HandleUILogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var req struct {
		Password string `json:"password"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if subtle.ConstantTimeCompare([]byte(req.Password), []byte(e.RootPassword)) != 1 {
		writeError(w, http.StatusUnauthorized, "invalid password")
		return
	}
	token, err := e.issueUIToken()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to issue token")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"token": token})
}

// ── Registration ─────────────────────────────────────────────────────────────

// HandleAgentRegister processes POST /api/v1/register.
// Agents authenticate with a pre-shared registration secret.
// Re-registration of the same machine_id is idempotent — the existing agent
// record is reused, all previous tokens are revoked, and a fresh token is issued.
func (e *Handler) HandleAgentRegister(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var req common.RegistrationRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	if req.Hostname == "" {
		writeError(w, http.StatusBadRequest, "hostname is required")
		return
	}
	if !validPlatform(req.Platform) {
		writeError(w, http.StatusBadRequest, "platform must be one of linux, mac, or windows")
		return
	}
	if !e.validRegistrationSecret(req.RegistrationSecret) {
		writeError(w, http.StatusUnauthorized, "invalid registration secret")
		return
	}

	var agent *common.Agent
	machineID := strings.TrimSpace(req.Metadata["machine_id"])
	if machineID != "" {
		for _, a := range e.Store.ListAgents() {
			if strings.TrimSpace(a.Metadata["machine_id"]) == machineID {
				agent = a
				break
			}
		}
	}
	isReregistration := agent != nil
	if agent == nil {
		agent = &common.Agent{
			ID:           common.NewID(),
			RegisteredAt: time.Now(),
		}
	}
	agent.Hostname = req.Hostname
	agent.Platform = req.Platform
	agent.OS = strings.TrimSpace(req.Metadata["os"])
	if agent.OS == "" {
		agent.OS = strings.TrimSpace(req.Metadata["os_description"])
	}
	agent.Model = strings.TrimSpace(req.Model)
	if agent.Model == "" {
		agent.Model = strings.TrimSpace(req.Metadata["model"])
	}
	agent.Vendor = strings.TrimSpace(req.Vendor)
	if agent.Vendor == "" {
		agent.Vendor = strings.TrimSpace(req.Metadata["vendor"])
	}
	agent.IPAddress = requestIP(r)
	agent.Metadata = req.Metadata
	agent.Shells = req.Shells
	agent.Status = common.AgentStatusRegistered
	agent.LastActivityAt = time.Now()

	if err := e.Store.SaveAgent(agent); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to save agent")
		return
	}

	// On re-registration revoke all existing tokens so only the new one is valid.
	if isReregistration {
		_ = e.Store.RevokeAllAgentTokens(agent.ID)
	}

	token, err := e.issueAgentToken(agent.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to issue token")
		return
	}
	writeJSON(w, http.StatusOK, common.RegistrationResponse{
		AgentID: agent.ID,
		Token:   token,
	})
}

func (e *Handler) validRegistrationSecret(s string) bool {
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

func validPlatform(platform common.PlatformType) bool {
	switch platform {
	case common.PlatformLinux, common.PlatformMac, common.PlatformWindows:
		return true
	default:
		return false
	}
}

// requestIP extracts the best-effort client IP from forwarded headers or RemoteAddr.
func requestIP(r *http.Request) string {
	xff := strings.TrimSpace(r.Header.Get("X-Forwarded-For"))
	if xff != "" {
		parts := strings.Split(xff, ",")
		if len(parts) > 0 {
			ip := strings.TrimSpace(parts[0])
			if ip != "" {
				return ip
			}
		}
	}
	if xri := strings.TrimSpace(r.Header.Get("X-Real-IP")); xri != "" {
		return xri
	}
	host, _, err := net.SplitHostPort(strings.TrimSpace(r.RemoteAddr))
	if err == nil {
		return host
	}
	return strings.TrimSpace(r.RemoteAddr)
}

// ── Utility ───────────────────────────────────────────────────────────────────

// GenerateSecret produces a cryptographically random hex string.
func GenerateSecret(numBytes int) (string, error) {
	b := make([]byte, numBytes)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func decodeJSON(r *http.Request, v any) error {
	return json.NewDecoder(r.Body).Decode(v)
}
