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
	"github.com/google/uuid"
	"github.com/marko-stanojevic/sear/internal/common"
)

// ── JWT helpers ───────────────────────────────────────────────────────────────

// issueToken signs a JWT for the given client ID.
func (e *Env) issueToken(clientID string) (string, error) {
	expiry := time.Duration(e.TokenExpiryHours) * time.Hour
	if expiry == 0 {
		expiry = 720 * time.Hour // 30 days default
	}
	claims := jwt.RegisteredClaims{
		Subject:   clientID,
		IssuedAt:  jwt.NewNumericDate(time.Now()),
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(expiry)),
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return tok.SignedString(e.JWTSecret)
}

// clientIDFromToken validates the Bearer token in the request and returns
// the embedded client ID.
func (e *Env) clientIDFromToken(r *http.Request) (string, error) {
	auth := r.Header.Get("Authorization")
	if !strings.HasPrefix(auth, "Bearer ") {
		// Also accept token as query parameter (for WebSocket clients that
		// cannot set headers during the handshake).
		token := r.URL.Query().Get("token")
		if token == "" {
			return "", fmt.Errorf("missing bearer token")
		}
		return e.parseToken(token)
	}
	return e.parseToken(strings.TrimPrefix(auth, "Bearer "))
}

func (e *Env) parseToken(raw string) (string, error) {
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

// ── Middleware ────────────────────────────────────────────────────────────────

// RequireClientAuth validates the client JWT and sets X-Client-ID for
// downstream handlers. Also refreshes the client's last-seen timestamp.
func (e *Env) RequireClientAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		clientID, err := e.clientIDFromToken(r)
		if err != nil {
			writeError(w, http.StatusUnauthorized, err.Error())
			return
		}
		if c, ok := e.Store.GetClient(clientID); ok {
			c.LastActivityAt = time.Now()
			if c.Status == common.ClientStatusOffline {
				c.Status = common.ClientStatusConnected
			}
			_ = e.Store.SaveClient(c)
		}
		r.Header.Set("X-Client-ID", clientID)
		next.ServeHTTP(w, r)
	})
}

// RequireRootAuth enforces HTTP Basic auth with username "root" and the
// configured root password.
func (e *Env) RequireRootAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, pass, ok := r.BasicAuth()
		if !ok ||
			subtle.ConstantTimeCompare([]byte(user), []byte("root")) != 1 ||
			subtle.ConstantTimeCompare([]byte(pass), []byte(e.RootPassword)) != 1 {
			w.Header().Set("WWW-Authenticate", `Basic realm="sear-root"`)
			writeError(w, http.StatusUnauthorized, "invalid root credentials")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// ── Registration ─────────────────────────────────────────────────────────────

// HandleRegister processes POST /api/v1/register.
// Clients authenticate with a pre-shared registration secret.
// Re-registration of the same machine_id is idempotent — the existing client
// record is reused and a fresh JWT is issued.
func (e *Env) HandleRegister(w http.ResponseWriter, r *http.Request) {
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

	// Reuse existing client record when the same machine_id re-registers
	// (e.g., after an OS re-image that cleared the client state file).
	var client *common.Client
	machineID := strings.TrimSpace(req.Metadata["machine_id"])
	if machineID != "" {
		for _, c := range e.Store.ListClients() {
			if strings.TrimSpace(c.Metadata["machine_id"]) == machineID {
				client = c
				break
			}
		}
	}
	if client == nil {
		clientID := preferredClientID(machineID)
		if existing, ok := e.Store.GetClient(clientID); ok {
			existingMachineID := ""
			if existing.Metadata != nil {
				existingMachineID = strings.TrimSpace(existing.Metadata["machine_id"])
			}
			if machineID == "" || existingMachineID == "" || existingMachineID != machineID {
				clientID = uuid.New().String()
			}
		}
		client = &common.Client{
			ID:           clientID,
			RegisteredAt: time.Now(),
		}
	}
	client.Hostname = req.Hostname
	client.Platform = req.Platform
	client.OS = strings.TrimSpace(req.Metadata["os"])
	if client.OS == "" {
		client.OS = strings.TrimSpace(req.Metadata["os_description"])
	}
	client.Model = strings.TrimSpace(req.Model)
	if client.Model == "" {
		client.Model = strings.TrimSpace(req.Metadata["model"])
	}
	client.Vendor = strings.TrimSpace(req.Vendor)
	if client.Vendor == "" {
		client.Vendor = strings.TrimSpace(req.Metadata["vendor"])
	}
	client.IPAddress = requestIP(r)
	client.Metadata = req.Metadata
	client.Status = common.ClientStatusRegistered
	client.LastActivityAt = time.Now()

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

func preferredClientID(machineID string) string {
	v := strings.TrimSpace(machineID)
	if v == "" {
		return uuid.New().String()
	}

	var b strings.Builder
	b.Grow(len(v))
	for _, r := range v {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
		case r >= 'A' && r <= 'Z':
			b.WriteRune(r)
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '-', r == '_', r == '.', r == ':':
			b.WriteRune(r)
		case r == ' ', r == '/', r == '\\':
			b.WriteRune('-')
		}
	}
	out := strings.Trim(b.String(), "-._:")
	if out == "" {
		return uuid.New().String()
	}
	if len(out) > 128 {
		out = out[:128]
	}
	return out
}
