package handlers

import (
	"crypto/rand"
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
// It prevents a UI token from being accepted by the agent client-auth middleware.
const uiTokenAudience = "kompakt-ui"

// ── JWT helpers ───────────────────────────────────────────────────────────────

// issueToken signs a JWT for the given agent ID.
func (e *Handler) issueToken(agentID string) (string, error) {
	expiry := time.Duration(e.TokenExpiryHours) * time.Hour
	if expiry == 0 {
		expiry = 720 * time.Hour // 30 days default
	}
	claims := jwt.RegisteredClaims{
		Subject:   agentID,
		IssuedAt:  jwt.NewNumericDate(time.Now()),
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(expiry)),
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return tok.SignedString(e.AgentJWTSecret)
}

// agentIDFromToken validates the Bearer token in the request and returns
// the embedded agent ID.
func (e *Handler) agentIDFromToken(r *http.Request) (string, error) {
	auth := r.Header.Get("Authorization")
	if !strings.HasPrefix(auth, "Bearer ") {
		// Also accept token as query parameter (for WebSocket agents that
		// cannot set headers during the handshake).
		token := r.URL.Query().Get("token")
		if token == "" {
			return "", fmt.Errorf("authentication token is missing or invalid")
		}
		return e.parseToken(token)
	}
	return e.parseToken(strings.TrimPrefix(auth, "Bearer "))
}

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

func (e *Handler) parseToken(raw string) (string, error) {
	tok, err := jwt.ParseWithClaims(raw, &jwt.RegisteredClaims{},
		func(_ *jwt.Token) (any, error) { return e.AgentJWTSecret, nil },
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

// ── Middleware ────────────────────────────────────────────────────────────────

// RequireAgentAuth validates the agent JWT and sets X-Agent-ID for
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

// isRootRequest returns true when the request carries valid root credentials —
// either HTTP Basic auth or a UI Bearer JWT issued by HandleUILogin.
// Use this for inline auth checks inside handlers that serve mixed audiences.
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
//   - HTTP Basic auth (username "root" + configured root password), for scripts/tools, or
//   - Bearer JWT issued by HandleUILogin, for the web UI (never stores the raw password).
func (e *Handler) RequireRootAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Accept UI Bearer JWT
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
		// Fall back to HTTP Basic auth
		user, pass, ok := r.BasicAuth()
		if !ok ||
			subtle.ConstantTimeCompare([]byte(user), []byte("root")) != 1 ||
			subtle.ConstantTimeCompare([]byte(pass), []byte(e.RootPassword)) != 1 {
			w.Header().Set("WWW-Authenticate", `Basic realm="kompakt-root"`)
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
// record is reused and a fresh JWT is issued.
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

	// Validate registration secret using constant-time comparison to prevent
	// timing-based enumeration of valid secrets.
	if !e.validRegistrationSecret(req.RegistrationSecret) {
		writeError(w, http.StatusUnauthorized, "invalid registration secret")
		return
	}

	// Reuse existing agent record when the same machine_id re-registers
	// (e.g., after an OS re-image that cleared the agent state file).
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
	token, err := e.issueToken(agent.ID)
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

// requestIP extracts the best-effort client IP from forwarded headers or
// RemoteAddr.
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

