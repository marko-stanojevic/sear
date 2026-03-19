//go:build integration

package server_test

import (
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/marko-stanojevic/kompakt/internal/common"
	"github.com/marko-stanojevic/kompakt/internal/server/store"
)

var allPartialPaths = []string{
	"/ui/partials/home-stats",
	"/ui/partials/agents",
	"/ui/partials/playbooks",
	"/ui/partials/deployments",
	"/ui/partials/artifacts",
	"/ui/partials/vault",
}

// Every HTMX partial must return 401 without authentication.
func TestIntegration_Partials_RequireAuth(t *testing.T) {
	env := newIntegrationEnv(t)
	for _, path := range allPartialPaths {
		t.Run(path, func(t *testing.T) {
			resp := env.get(t, path, "")
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusUnauthorized {
				t.Errorf("status = %d; want 401", resp.StatusCode)
			}
		})
	}
}

// Every partial must render valid HTML (200 + text/html) even when the DB is empty,
// and must include the expected marker rather than a blank or broken response.
func TestIntegration_Partials_EmptyState(t *testing.T) {
	env := newIntegrationEnv(t)
	auth := "Bearer " + env.rootUIToken(t)

	// home-stats has no empty state — it always renders a dashboard grid.
	// The remaining partials all render a class="empty" div when the DB is empty.
	t.Run("/ui/partials/home-stats", func(t *testing.T) {
		resp := env.get(t, "/ui/partials/home-stats", auth)
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			b, _ := io.ReadAll(resp.Body)
			t.Fatalf("status = %d; want 200 (body: %s)", resp.StatusCode, b)
		}
		if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
			t.Errorf("Content-Type = %q; want text/html", ct)
		}
		body, _ := io.ReadAll(resp.Body)
		if !strings.Contains(string(body), `class="dashboard-grid"`) {
			t.Errorf("missing dashboard-grid marker in body:\n%s", body)
		}
	})

	for _, path := range allPartialPaths[1:] { // skip /ui/partials/home-stats
		t.Run(path, func(t *testing.T) {
			resp := env.get(t, path, auth)
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				b, _ := io.ReadAll(resp.Body)
				t.Fatalf("status = %d; want 200 (body: %s)", resp.StatusCode, b)
			}
			if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
				t.Errorf("Content-Type = %q; want text/html", ct)
			}
			body, _ := io.ReadAll(resp.Body)
			if !strings.Contains(string(body), `class="empty"`) {
				t.Errorf("missing empty-state marker in body:\n%s", body)
			}
		})
	}
}

// When the DB contains records, each table partial must render the data rows
// rather than falling through to the empty state.
func TestIntegration_Partials_WithData(t *testing.T) {
	env := newIntegrationEnv(t)

	agent := &common.Agent{
		ID:             common.NewID(),
		Hostname:       "seed-host",
		Platform:       common.PlatformLinux,
		Status:         common.AgentStatusConnected,
		RegisteredAt:   time.Now(),
		LastActivityAt: time.Now(),
	}
	if err := env.handler.Store.SaveAgent(agent); err != nil {
		t.Fatalf("save agent: %v", err)
	}

	pb := &store.PlaybookRecord{
		ID:        common.NewID(),
		Name:      "seed-playbook",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	if err := env.handler.Store.SavePlaybook(pb); err != nil {
		t.Fatalf("save playbook: %v", err)
	}

	dep := &common.DeploymentState{
		ID:         common.NewID(),
		AgentID:    agent.ID,
		PlaybookID: pb.ID,
		Status:     common.DeploymentStatusRunning,
	}
	if err := env.handler.Store.SaveDeployment(dep); err != nil {
		t.Fatalf("save deployment: %v", err)
	}

	if err := env.handler.Store.SetSecret("TEST_KEY", "secret-value"); err != nil {
		t.Fatalf("set secret: %v", err)
	}

	art := &common.Artifact{
		ID:           common.NewID(),
		Name:         "seed-artifact",
		FileName:     "seed.bin",
		ContentType:  "application/octet-stream",
		Size:         1024,
		UploadedAt:   time.Now(),
		AccessPolicy: common.AccessPublic,
	}
	if err := env.handler.Store.SaveArtifact(art); err != nil {
		t.Fatalf("save artifact: %v", err)
	}

	auth := "Bearer " + env.rootUIToken(t)

	tableTests := []struct {
		path   string
		marker string
	}{
		{"/ui/partials/agents", `class="list list-agents"`},
		{"/ui/partials/playbooks", `class="list list-playbooks"`},
		{"/ui/partials/deployments", `class="list list-deployments"`},
		{"/ui/partials/vault", `class="list list-secrets"`},
		{"/ui/partials/artifacts", `class="list list-artifacts"`},
	}

	for _, tc := range tableTests {
		t.Run(tc.path, func(t *testing.T) {
			resp := env.get(t, tc.path, auth)
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				b, _ := io.ReadAll(resp.Body)
				t.Fatalf("status = %d; want 200 (body: %s)", resp.StatusCode, b)
			}
			body, _ := io.ReadAll(resp.Body)
			if !strings.Contains(string(body), tc.marker) {
				t.Errorf("missing table marker %q in body:\n%s", tc.marker, body)
			}
		})
	}
}
