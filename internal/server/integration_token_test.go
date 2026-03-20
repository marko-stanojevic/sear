//go:build integration

package server_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"testing"
)

// ── Token lifecycle ───────────────────────────────────────────────────────────

func TestIntegration_TokenRefresh_NewTokenWorks(t *testing.T) {
	env := newIntegrationEnv(t)
	oldToken := env.registerAgent(t, "refresh-host")

	// Call /api/v1/token/refresh with the current token.
	req, _ := http.NewRequest(http.MethodPost, env.srv.URL+"/api/v1/token/refresh", nil)
	req.Header.Set("Authorization", "Bearer "+oldToken)
	resp, err := env.client.Do(req)
	if err != nil {
		t.Fatalf("token/refresh: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("token/refresh status = %d; body = %s", resp.StatusCode, b)
	}
	var result struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("decode refresh response: %v", err)
	}
	if result.Token == "" {
		t.Fatal("expected non-empty token in refresh response")
	}
	if result.Token == oldToken {
		t.Error("refreshed token must differ from the original")
	}
}

func TestIntegration_TokenRefresh_OldTokenInvalidated(t *testing.T) {
	env := newIntegrationEnv(t)
	oldToken := env.registerAgent(t, "refresh-host-2")

	// Rotate the token.
	req, _ := http.NewRequest(http.MethodPost, env.srv.URL+"/api/v1/token/refresh", nil)
	req.Header.Set("Authorization", "Bearer "+oldToken)
	resp, err := env.client.Do(req)
	if err != nil {
		t.Fatalf("token/refresh: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("token/refresh status = %d; body = %s", resp.StatusCode, b)
	}

	// The old token must now be rejected on a protected endpoint.
	wsReq, _ := http.NewRequest(http.MethodGet, env.srv.URL+"/api/v1/ws?token="+oldToken, nil)
	wsResp, err := env.client.Do(wsReq)
	if err != nil {
		t.Fatalf("ws with old token: %v", err)
	}
	defer wsResp.Body.Close()
	if wsResp.StatusCode == http.StatusSwitchingProtocols {
		t.Error("old token should be rejected after rotation")
	}
	// Expect 401 (bad/revoked token before WebSocket upgrade).
	if wsResp.StatusCode != http.StatusUnauthorized {
		t.Errorf("old token: status = %d; want 401", wsResp.StatusCode)
	}
}

func TestIntegration_TokenRefresh_RequiresAuth(t *testing.T) {
	env := newIntegrationEnv(t)

	req, _ := http.NewRequest(http.MethodPost, env.srv.URL+"/api/v1/token/refresh", nil)
	resp, err := env.client.Do(req)
	if err != nil {
		t.Fatalf("token/refresh no auth: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("status = %d; want 401 when no auth provided", resp.StatusCode)
	}
}

func TestIntegration_TokenRefresh_MethodNotAllowed(t *testing.T) {
	env := newIntegrationEnv(t)
	token := env.registerAgent(t, "refresh-method-host")

	req, _ := http.NewRequest(http.MethodGet, env.srv.URL+"/api/v1/token/refresh", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := env.client.Do(req)
	if err != nil {
		t.Fatalf("GET token/refresh: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("status = %d; want 405", resp.StatusCode)
	}
}

// ── Re-registration revokes old tokens ───────────────────────────────────────

func TestIntegration_Reregistration_RevokesOldToken(t *testing.T) {
	env := newIntegrationEnv(t)

	// Register the same machine twice (same hostname, no machine_id — treated as two separate agents in integration env).
	// For the revocation test, register first, then register again with the same machine_id.
	body1, _ := json.Marshal(map[string]any{
		"platform":            "linux",
		"hostname":            "reregister-host",
		"registration_secret": intTestRegSecret,
		"metadata":            map[string]string{"machine_id": "test-machine-abc"},
	})
	resp1, err := env.client.Post(env.srv.URL+"/api/v1/register", "application/json", bytes.NewReader(body1))
	if err != nil {
		t.Fatalf("register 1: %v", err)
	}
	defer resp1.Body.Close()
	if resp1.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp1.Body)
		t.Fatalf("register 1 status = %d; body = %s", resp1.StatusCode, b)
	}
	var reg1 struct {
		Token string `json:"token"`
	}
	json.NewDecoder(resp1.Body).Decode(&reg1)
	if reg1.Token == "" {
		t.Fatal("first registration returned empty token")
	}

	// Second registration for the same machine_id.
	body2, _ := json.Marshal(map[string]any{
		"platform":            "linux",
		"hostname":            "reregister-host",
		"registration_secret": intTestRegSecret,
		"metadata":            map[string]string{"machine_id": "test-machine-abc"},
	})
	resp2, err := env.client.Post(env.srv.URL+"/api/v1/register", "application/json", bytes.NewReader(body2))
	if err != nil {
		t.Fatalf("register 2: %v", err)
	}
	defer resp2.Body.Close()
	if resp2.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp2.Body)
		t.Fatalf("register 2 status = %d; body = %s", resp2.StatusCode, b)
	}

	// Old token must now be rejected.
	wsReq, _ := http.NewRequest(http.MethodGet, env.srv.URL+"/api/v1/ws?token="+reg1.Token, nil)
	wsResp, err := env.client.Do(wsReq)
	if err != nil {
		t.Fatalf("ws with old token: %v", err)
	}
	defer wsResp.Body.Close()
	if wsResp.StatusCode != http.StatusUnauthorized {
		t.Errorf("old token after re-registration: status = %d; want 401", wsResp.StatusCode)
	}
}

// ── WWW-Authenticate behaviour ────────────────────────────────────────────────

func TestIntegration_RootAPI_BearerExpired_NoWWWAuthenticate(t *testing.T) {
	env := newIntegrationEnv(t)

	// Send a clearly invalid Bearer token (simulates an expired UI JWT).
	resp := env.get(t, "/api/v1/status", "Bearer invalid-token-here")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d; want 401", resp.StatusCode)
	}
	if got := resp.Header.Get("WWW-Authenticate"); got != "" {
		t.Errorf("WWW-Authenticate = %q; want empty when Bearer token was already presented", got)
	}
}

func TestIntegration_RootAPI_NoAuth_HasWWWAuthenticate(t *testing.T) {
	env := newIntegrationEnv(t)
	resp := env.get(t, "/api/v1/status", "")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d; want 401", resp.StatusCode)
	}
	const wantHeader = `Basic realm="kompakt-root"`
	if got := resp.Header.Get("WWW-Authenticate"); got != wantHeader {
		t.Errorf("WWW-Authenticate = %q; want %q", got, wantHeader)
	}
}
