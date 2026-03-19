//go:build integration

package server_test

import (
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestIntegration_Healthz(t *testing.T) {
	env := newIntegrationEnv(t)
	resp, err := env.client.Get(env.srv.URL + "/healthz")
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d; want 200", resp.StatusCode)
	}
	b, _ := io.ReadAll(resp.Body)
	if string(b) != "ok" {
		t.Errorf("body = %q; want ok", b)
	}
}

// UI page shells are public — no auth required. The in-page JS handles auth
// for API calls. A 401 or 500 here means a regression in template rendering.
func TestIntegration_UIPages_Accessible(t *testing.T) {
	env := newIntegrationEnv(t)
	pages := []string{
		"/ui", "/ui/agents", "/ui/playbooks",
		"/ui/deployments", "/ui/vault", "/ui/artifacts",
	}
	for _, path := range pages {
		t.Run(path, func(t *testing.T) {
			resp := env.get(t, path, "")
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				b, _ := io.ReadAll(resp.Body)
				t.Errorf("status = %d; want 200 (body: %s)", resp.StatusCode, b)
			}
			if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
				t.Errorf("Content-Type = %q; want text/html", ct)
			}
		})
	}
}
