//go:build integration

package server_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/marko-stanojevic/kompakt/internal/common"
	"github.com/marko-stanojevic/kompakt/internal/server"
	"github.com/marko-stanojevic/kompakt/internal/server/handlers"
	"github.com/marko-stanojevic/kompakt/internal/server/service"
	"github.com/marko-stanojevic/kompakt/internal/server/store"
)

const (
	intTestRootPassword = "integration-test-root-pass"
	intTestRegSecret    = "integration-test-reg-secret"
)

// integrationEnv holds a live httptest.Server wired with real routing.
type integrationEnv struct {
	srv     *httptest.Server
	handler *handlers.Handler
	client  *http.Client
}

func newIntegrationEnv(t *testing.T) *integrationEnv {
	t.Helper()
	st, err := store.New(t.TempDir(), "")
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	hub := handlers.NewHub()
	h := &handlers.Handler{
		Store:               st,
		UserJWTSecret:       []byte("user-secret-32-bytes-padding!!!!"),
		RootPassword:        intTestRootPassword,
		TokenExpiryHours:    24,
		ArtifactsDir:        t.TempDir(),
		RegistrationSecrets: map[string]string{"default": intTestRegSecret},
		Hub:                 hub,
	}
	h.Service = &service.Manager{Store: st, Hub: hub, ServerURL: "http://placeholder"}
	srv := httptest.NewServer(server.NewServer(h))
	t.Cleanup(srv.Close)
	h.ServerURL = srv.URL
	h.Service.ServerURL = srv.URL
	return &integrationEnv{srv: srv, handler: h, client: srv.Client()}
}

// rootUIToken logs in with the root password and returns a UI Bearer JWT.
func (e *integrationEnv) rootUIToken(t *testing.T) string {
	t.Helper()
	body, _ := json.Marshal(map[string]string{"password": intTestRootPassword})
	resp, err := e.client.Post(e.srv.URL+"/api/v1/ui/login", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("ui/login: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("ui/login status=%d body=%s", resp.StatusCode, b)
	}
	var result struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("decode login response: %v", err)
	}
	if result.Token == "" {
		t.Fatal("ui/login returned empty token")
	}
	return result.Token
}

// get issues a GET with an optional Authorization header.
func (e *integrationEnv) get(t *testing.T, path, authHeader string) *http.Response {
	t.Helper()
	req, _ := http.NewRequest(http.MethodGet, e.srv.URL+path, nil)
	if authHeader != "" {
		req.Header.Set("Authorization", authHeader)
	}
	resp, err := e.client.Do(req)
	if err != nil {
		t.Fatalf("GET %s: %v", path, err)
	}
	return resp
}

// basicAuthHeader returns an Authorization header value for root Basic auth.
func basicAuthHeader(password string) string {
	req, _ := http.NewRequest(http.MethodGet, "http://x", nil)
	req.SetBasicAuth("root", password)
	return req.Header.Get("Authorization")
}

// registerAgent registers a test agent and returns its opaque token.
func (e *integrationEnv) registerAgent(t *testing.T, hostname string) string {
	t.Helper()
	body, _ := json.Marshal(common.RegistrationRequest{
		Platform:           common.PlatformLinux,
		Hostname:           hostname,
		RegistrationSecret: intTestRegSecret,
	})
	resp, err := e.client.Post(e.srv.URL+"/api/v1/register", "application/json", bytes.NewReader(body))
	if err != nil || resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("register: status=%d err=%v body=%s", resp.StatusCode, err, b)
	}
	defer resp.Body.Close()
	var reg common.RegistrationResponse
	_ = json.NewDecoder(resp.Body).Decode(&reg)
	return reg.Token
}
