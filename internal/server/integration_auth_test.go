//go:build integration

package server_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"testing"
)

func TestIntegration_UILogin_Success(t *testing.T) {
	env := newIntegrationEnv(t)
	token := env.rootUIToken(t) // fatals on failure
	if token == "" {
		t.Fatal("expected non-empty token")
	}
}

func TestIntegration_UILogin_WrongPassword(t *testing.T) {
	env := newIntegrationEnv(t)
	body, _ := json.Marshal(map[string]string{"password": "bad-password"})
	resp, err := env.client.Post(env.srv.URL+"/api/v1/ui/login", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("status = %d; want 401", resp.StatusCode)
	}
}

func TestIntegration_UILogin_MethodNotAllowed(t *testing.T) {
	env := newIntegrationEnv(t)
	resp, err := env.client.Get(env.srv.URL + "/api/v1/ui/login")
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("status = %d; want 405", resp.StatusCode)
	}
}

func TestIntegration_RootAPI_BasicAuth(t *testing.T) {
	env := newIntegrationEnv(t)
	resp := env.get(t, "/api/v1/status", basicAuthHeader(intTestRootPassword))
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Errorf("status = %d; want 200 (body: %s)", resp.StatusCode, b)
	}
}

func TestIntegration_RootAPI_WrongBasicAuth(t *testing.T) {
	env := newIntegrationEnv(t)
	resp := env.get(t, "/api/v1/status", basicAuthHeader("wrong-password"))
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("status = %d; want 401", resp.StatusCode)
	}
	const wantHeader = `Basic realm="kompakt-root"`
	if got := resp.Header.Get("WWW-Authenticate"); got != wantHeader {
		t.Errorf("WWW-Authenticate = %q; want %q", got, wantHeader)
	}
}

func TestIntegration_RootAPI_NoAuth(t *testing.T) {
	env := newIntegrationEnv(t)
	resp := env.get(t, "/api/v1/status", "")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("status = %d; want 401", resp.StatusCode)
	}
}

// The UI JWT obtained from /api/v1/ui/login must work for root-protected endpoints.
func TestIntegration_RootAPI_UIToken(t *testing.T) {
	env := newIntegrationEnv(t)
	token := env.rootUIToken(t)
	resp := env.get(t, "/api/v1/status", "Bearer "+token)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Errorf("status = %d; want 200 (body: %s)", resp.StatusCode, b)
	}
}

// An agent JWT must not be accepted by root-protected endpoints (audience mismatch).
func TestIntegration_RootAPI_AgentTokenRejected(t *testing.T) {
	env := newIntegrationEnv(t)
	agentToken := env.registerAgent(t, "test-host")
	resp := env.get(t, "/api/v1/status", "Bearer "+agentToken)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("agent token should not access root endpoint: status = %d; want 401", resp.StatusCode)
	}
}

// A UI JWT must not be accepted by agent-only endpoints.
func TestIntegration_AgentWS_UITokenRejected(t *testing.T) {
	env := newIntegrationEnv(t)
	token := env.rootUIToken(t)
	// WebSocket upgrade will fail, but we can verify the token is rejected at the HTTP layer.
	req, _ := http.NewRequest(http.MethodGet, env.srv.URL+"/api/v1/ws?token="+token, nil)
	resp, err := env.client.Do(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()
	// Should be 401 (bad token) or 400 (not a WS upgrade), not 101 (Switching Protocols).
	if resp.StatusCode == http.StatusSwitchingProtocols {
		t.Errorf("UI JWT should not be accepted for WebSocket upgrade")
	}
}
