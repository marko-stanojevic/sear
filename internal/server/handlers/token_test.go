package handlers_test

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/marko-stanojevic/kompakt/internal/common"
)

// sha256hex mirrors the private helper in auth.go for test use.
func sha256hex(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}

// ── HandleTokenRefresh ────────────────────────────────────────────────────────

func TestHandleTokenRefresh_NewTokenIssuedOldRevoked(t *testing.T) {
	env := newTestEnv(t)
	agentID, oldToken := registerAgent(t, env, "SN-REFRESH-01", "refresh-host")

	// RequireAgentAuth sets X-Agent-ID; simulate that here.
	req := httptest.NewRequest(http.MethodPost, "/api/v1/token/refresh", nil)
	req.Header.Set("Authorization", "Bearer "+oldToken)
	req.Header.Set("X-Agent-ID", agentID)
	rr := httptest.NewRecorder()
	env.HandleTokenRefresh(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d; want 200 (body: %s)", rr.Code, rr.Body.String())
	}

	resp := decode[map[string]string](t, rr)
	newToken := resp["token"]
	if newToken == "" {
		t.Fatal("expected non-empty token in response")
	}
	if !strings.HasPrefix(newToken, "kpkt_") {
		t.Errorf("new token = %q; want kpkt_ prefix", newToken)
	}
	if newToken == oldToken {
		t.Error("new token must differ from the old token")
	}
}

func TestHandleTokenRefresh_OldTokenRejectedAfterRotation(t *testing.T) {
	env := newTestEnv(t)
	agentID, oldToken := registerAgent(t, env, "SN-REFRESH-02", "refresh-host-2")

	req := httptest.NewRequest(http.MethodPost, "/api/v1/token/refresh", nil)
	req.Header.Set("Authorization", "Bearer "+oldToken)
	req.Header.Set("X-Agent-ID", agentID)
	rr := httptest.NewRecorder()
	env.HandleTokenRefresh(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("refresh status = %d; want 200", rr.Code)
	}
	newToken := decode[map[string]string](t, rr)["token"]

	okHandler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })

	// Old token must now be rejected.
	oldReq := httptest.NewRequest(http.MethodGet, "/api/v1/ws", nil)
	oldReq.Header.Set("Authorization", "Bearer "+oldToken)
	oldRR := httptest.NewRecorder()
	env.RequireAgentAuth(okHandler).ServeHTTP(oldRR, oldReq)
	if oldRR.Code != http.StatusUnauthorized {
		t.Errorf("old token: status = %d; want 401 after rotation", oldRR.Code)
	}

	// New token must be accepted.
	newReq := httptest.NewRequest(http.MethodGet, "/api/v1/ws", nil)
	newReq.Header.Set("Authorization", "Bearer "+newToken)
	newRR := httptest.NewRecorder()
	env.RequireAgentAuth(okHandler).ServeHTTP(newRR, newReq)
	if newRR.Code != http.StatusOK {
		t.Errorf("new token: status = %d; want 200", newRR.Code)
	}
}

func TestHandleTokenRefresh_MethodNotAllowed(t *testing.T) {
	env := newTestEnv(t)
	rr := getRequest(t, env.HandleTokenRefresh, "/api/v1/token/refresh", "")
	if rr.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d; want 405", rr.Code)
	}
}

// ── Opaque token format ───────────────────────────────────────────────────────

func TestHandleRegister_TokenHasKpktPrefix(t *testing.T) {
	env := newTestEnv(t)
	_, token := registerAgent(t, env, "SN-PREFIX-01", "prefix-host")
	if !strings.HasPrefix(token, "kpkt_") {
		t.Errorf("token = %q; want kpkt_ prefix", token)
	}
	// 5 (prefix) + 64 (32 bytes hex-encoded) = 69 chars
	if len(token) != 69 {
		t.Errorf("token length = %d; want 69 (kpkt_ + 64 hex chars)", len(token))
	}
}

// ── Opaque token auth scenarios ──────────────────────────────────────────────

func TestRequireAgentAuth_RevokedTokenRejected(t *testing.T) {
	env := newTestEnv(t)
	agentID, token := registerAgent(t, env, "SN-REVOKE-01", "revoke-host")

	// Revoke the token directly via the store.
	tok, err := env.Store.GetAgentTokenByHash(sha256hex(token))
	if err != nil {
		t.Fatalf("GetAgentTokenByHash: %v", err)
	}
	if err := env.Store.RevokeAgentToken(tok.ID); err != nil {
		t.Fatalf("RevokeAgentToken: %v", err)
	}
	_ = agentID

	req := httptest.NewRequest(http.MethodGet, "/api/v1/ws", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rr := httptest.NewRecorder()
	env.RequireAgentAuth(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})).ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("revoked token: status = %d; want 401", rr.Code)
	}
}

func TestRequireAgentAuth_ExpiredTokenRejected(t *testing.T) {
	env := newTestEnv(t)
	agentID, _ := registerAgent(t, env, "SN-EXPIRED-01", "expired-host")

	// Insert an already-expired token directly into the store.
	rawToken := "kpkt_" + strings.Repeat("e", 64)
	pastExpiry := time.Now().Add(-1 * time.Hour)
	_ = env.Store.CreateAgentToken(&common.AgentToken{
		ID:        "tok-expired",
		AgentID:   agentID,
		TokenHash: sha256hex(rawToken),
		CreatedAt: time.Now().Add(-2 * time.Hour),
		ExpiresAt: &pastExpiry,
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/ws", nil)
	req.Header.Set("Authorization", "Bearer "+rawToken)
	rr := httptest.NewRecorder()
	env.RequireAgentAuth(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})).ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expired token: status = %d; want 401", rr.Code)
	}
}

func TestRequireAgentAuth_ValidTokenUpdatesAgentStatus(t *testing.T) {
	env := newTestEnv(t)
	agentID, token := registerAgent(t, env, "SN-STATUS-01", "status-host")

	// Set agent to offline so we can verify the transition.
	a, ok := env.Store.GetAgent(agentID)
	if !ok {
		t.Fatal("agent not found")
	}
	a.Status = common.AgentStatusOffline
	if err := env.Store.SaveAgent(a); err != nil {
		t.Fatalf("SaveAgent: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/ws", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rr := httptest.NewRecorder()
	env.RequireAgentAuth(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})).ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d; want 200", rr.Code)
	}
	updated, ok := env.Store.GetAgent(agentID)
	if !ok {
		t.Fatal("agent not found after auth")
	}
	if updated.Status == common.AgentStatusOffline {
		t.Error("expected agent status to change from Offline after successful auth")
	}
}

// ── RequireRootAuth — WWW-Authenticate behaviour ──────────────────────────────

func TestRequireRootAuth_WWWAuthenticateOnlyWhenNoAuthHeader(t *testing.T) {
	env := newTestEnv(t)
	okHandler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	t.Run("no Authorization header — WWW-Authenticate is set", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/status", nil)
		rr := httptest.NewRecorder()
		env.RequireRootAuth(okHandler).ServeHTTP(rr, req)
		if rr.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d; want 401", rr.Code)
		}
		if got := rr.Header().Get("WWW-Authenticate"); got == "" {
			t.Error("expected WWW-Authenticate header when no auth is provided")
		}
	})

	t.Run("expired Bearer token — no WWW-Authenticate header", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/status", nil)
		req.Header.Set("Authorization", "Bearer kpkt_expiredorinvalidtoken000000000000000000000000000000000000000000")
		rr := httptest.NewRecorder()
		env.RequireRootAuth(okHandler).ServeHTTP(rr, req)
		if rr.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d; want 401", rr.Code)
		}
		if got := rr.Header().Get("WWW-Authenticate"); got != "" {
			t.Errorf("WWW-Authenticate = %q; want empty when Bearer token was already presented", got)
		}
	})

	t.Run("wrong Basic auth credentials — WWW-Authenticate is set", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/status", nil)
		req.SetBasicAuth("root", "wrong-password")
		rr := httptest.NewRecorder()
		env.RequireRootAuth(okHandler).ServeHTTP(rr, req)
		if rr.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d; want 401", rr.Code)
		}
		// Basic auth requests do not have a Bearer header; WWW-Authenticate should be sent.
		if got := rr.Header().Get("WWW-Authenticate"); got == "" {
			t.Error("expected WWW-Authenticate header on failed Basic auth")
		}
	})

	// ── HTMX requests must never receive WWW-Authenticate ────────────────────
	// The browser would show a native Basic auth dialog if this header is present,
	// blocking the JS login modal from handling re-auth gracefully.

	t.Run("HTMX request — no auth — no WWW-Authenticate", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/status", nil)
		req.Header.Set("HX-Request", "true")
		rr := httptest.NewRecorder()
		env.RequireRootAuth(okHandler).ServeHTTP(rr, req)
		if rr.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d; want 401", rr.Code)
		}
		if got := rr.Header().Get("WWW-Authenticate"); got != "" {
			t.Errorf("WWW-Authenticate = %q; want empty for HTMX request", got)
		}
	})

	t.Run("HTMX request — wrong Basic auth — no WWW-Authenticate", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/status", nil)
		req.Header.Set("HX-Request", "true")
		req.SetBasicAuth("root", "wrong-password")
		rr := httptest.NewRecorder()
		env.RequireRootAuth(okHandler).ServeHTTP(rr, req)
		if rr.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d; want 401", rr.Code)
		}
		if got := rr.Header().Get("WWW-Authenticate"); got != "" {
			t.Errorf("WWW-Authenticate = %q; want empty for HTMX request", got)
		}
	})
}

// ── Re-registration revokes old tokens ───────────────────────────────────────

func TestHandleRegister_ReregistrationRevokesOldTokens(t *testing.T) {
	env := newTestEnv(t)
	agentID, oldToken := registerAgent(t, env, "SN-REREG-01", "rereg-host")

	// Verify old token works.
	req := httptest.NewRequest(http.MethodGet, "/api/v1/ws", nil)
	req.Header.Set("Authorization", "Bearer "+oldToken)
	rr := httptest.NewRecorder()
	env.RequireAgentAuth(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})).ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("old token before re-registration: status = %d; want 200", rr.Code)
	}

	// Re-register the same machine.
	_, newToken := registerAgent(t, env, "SN-REREG-01", "rereg-host")
	_ = agentID

	if newToken == oldToken {
		t.Fatal("expected a different token after re-registration")
	}

	// Old token must now be rejected.
	req2 := httptest.NewRequest(http.MethodGet, "/api/v1/ws", nil)
	req2.Header.Set("Authorization", "Bearer "+oldToken)
	rr2 := httptest.NewRecorder()
	env.RequireAgentAuth(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})).ServeHTTP(rr2, req2)
	if rr2.Code != http.StatusUnauthorized {
		t.Errorf("old token after re-registration: status = %d; want 401", rr2.Code)
	}

	// New token must be accepted.
	req3 := httptest.NewRequest(http.MethodGet, "/api/v1/ws", nil)
	req3.Header.Set("Authorization", "Bearer "+newToken)
	rr3 := httptest.NewRecorder()
	env.RequireAgentAuth(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})).ServeHTTP(rr3, req3)
	if rr3.Code != http.StatusOK {
		t.Errorf("new token after re-registration: status = %d; want 200", rr3.Code)
	}
}
