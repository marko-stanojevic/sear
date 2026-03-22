package handlers_test

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/marko-stanojevic/kompakt/internal/common"
	"github.com/marko-stanojevic/kompakt/internal/server/store"
)

// ── HandleRootDeployments: missing DELETE cases ───────────────────────────────

func TestHandleRootDeployments_Delete(t *testing.T) {
	env := newTestEnv(t)
	now := time.Now()
	_ = env.Store.SaveDeployment(&common.DeploymentState{
		ID:        "dep-del-1",
		AgentID:   "a1",
		Status:    common.DeploymentStatusCompleted,
		StartedAt: now,
		UpdatedAt: now,
	})

	t.Run("delete without id returns 400", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodDelete, "/api/v1/deployments", nil)
		req.URL.Path = "/api/v1/deployments"
		rr := httptest.NewRecorder()
		env.HandleRootDeployments(rr, req)
		if rr.Code != http.StatusBadRequest {
			t.Errorf("status = %d; want 400", rr.Code)
		}
	})

	t.Run("delete existing deployment returns 200", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodDelete, "/api/v1/deployments/dep-del-1", nil)
		req.URL.Path = "/api/v1/deployments/dep-del-1"
		rr := httptest.NewRecorder()
		env.HandleRootDeployments(rr, req)
		if rr.Code != http.StatusOK {
			t.Errorf("status = %d; want 200 (body: %s)", rr.Code, rr.Body.String())
		}
	})
}

// ── HandleRootPlaybooks: missing DELETE and PUT error cases ───────────────────

func TestHandleRootPlaybooks_Delete(t *testing.T) {
	env := newTestEnv(t)
	now := time.Now()
	_ = env.Store.SavePlaybook(&store.PlaybookRecord{
		ID:   "pb-del-1",
		Name: "to-delete",
		Playbook: &common.Playbook{
			Name: "to-delete",
			Jobs: []common.Job{{Name: "j1", Steps: []common.Step{{Name: "s1", Run: "echo x"}}}},
		},
		CreatedAt: now,
		UpdatedAt: now,
	})

	t.Run("delete without id returns 400", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodDelete, "/api/v1/playbooks", nil)
		req.URL.Path = "/api/v1/playbooks"
		rr := httptest.NewRecorder()
		env.HandleRootPlaybooks(rr, req)
		if rr.Code != http.StatusBadRequest {
			t.Errorf("status = %d; want 400", rr.Code)
		}
	})

	t.Run("delete existing playbook returns 200", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodDelete, "/api/v1/playbooks/pb-del-1", nil)
		req.URL.Path = "/api/v1/playbooks/pb-del-1"
		rr := httptest.NewRecorder()
		env.HandleRootPlaybooks(rr, req)
		if rr.Code != http.StatusOK {
			t.Errorf("status = %d; want 200", rr.Code)
		}
	})
}

func TestHandleRootPlaybooks_PutErrors(t *testing.T) {
	env := newTestEnv(t)
	now := time.Now()
	_ = env.Store.SavePlaybook(&store.PlaybookRecord{
		ID:   "pb-put-err",
		Name: "existing",
		Playbook: &common.Playbook{
			Name: "existing",
			Jobs: []common.Job{{Name: "j1", Steps: []common.Step{{Name: "s1", Run: "echo x"}}}},
		},
		CreatedAt: now,
		UpdatedAt: now,
	})

	t.Run("put missing playbook returns 404", func(t *testing.T) {
		body := `{"name":"x","playbook":{"name":"x","jobs":[{"name":"j1","steps":[{"name":"s1","run":"echo hi"}]}]}}`
		req := httptest.NewRequest(http.MethodPut, "/api/v1/playbooks/nonexistent", bytes.NewBufferString(body))
		req.URL.Path = "/api/v1/playbooks/nonexistent"
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()
		env.HandleRootPlaybooks(rr, req)
		if rr.Code != http.StatusNotFound {
			t.Errorf("status = %d; want 404", rr.Code)
		}
	})

	t.Run("put invalid JSON returns 400", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPut, "/api/v1/playbooks/pb-put-err", bytes.NewBufferString("{bad"))
		req.URL.Path = "/api/v1/playbooks/pb-put-err"
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()
		env.HandleRootPlaybooks(rr, req)
		if rr.Code != http.StatusBadRequest {
			t.Errorf("status = %d; want 400", rr.Code)
		}
	})

	t.Run("put with no jobs returns 400", func(t *testing.T) {
		body := `{"playbook":{"name":"empty","jobs":[]}}`
		req := httptest.NewRequest(http.MethodPut, "/api/v1/playbooks/pb-put-err", bytes.NewBufferString(body))
		req.URL.Path = "/api/v1/playbooks/pb-put-err"
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()
		env.HandleRootPlaybooks(rr, req)
		if rr.Code != http.StatusBadRequest {
			t.Errorf("status = %d; want 400", rr.Code)
		}
	})
}

// ── HandleRootAgents: missing PUT not-found case ──────────────────────────────

func TestHandleRootAgents_PutMissingAgent(t *testing.T) {
	env := newTestEnv(t)
	req := httptest.NewRequest(http.MethodPut, "/api/v1/agents/nonexistent", bytes.NewBufferString(`{"status":"connected"}`))
	req.URL.Path = "/api/v1/agents/nonexistent"
	rr := httptest.NewRecorder()
	env.HandleRootAgents(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Errorf("status = %d; want 404", rr.Code)
	}
}

// ── HandleSecrets: missing error paths ───────────────────────────────────────

func TestHandleSecrets_AdditionalErrors(t *testing.T) {
	env := newTestEnv(t)

	t.Run("put invalid JSON returns 400", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPut, "/api/v1/secrets/MY_KEY", bytes.NewBufferString("{bad"))
		req.URL.Path = "/api/v1/secrets/MY_KEY"
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()
		env.HandleSecrets(rr, req)
		if rr.Code != http.StatusBadRequest {
			t.Errorf("status = %d; want 400", rr.Code)
		}
	})

	t.Run("delete without name returns 400", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodDelete, "/api/v1/secrets", nil)
		req.URL.Path = "/api/v1/secrets"
		rr := httptest.NewRecorder()
		env.HandleSecrets(rr, req)
		if rr.Code != http.StatusBadRequest {
			t.Errorf("status = %d; want 400", rr.Code)
		}
	})
}

// ── HandlePartialArtifacts: search filter path ────────────────────────────────

func TestHandlePartialArtifacts_WithSearchQuery(t *testing.T) {
	env := newTestEnv(t)
	_ = env.Store.SaveArtifact(&common.Artifact{
		ID:       "art-search-1",
		Name:     "myapp",
		FileName: "myapp.bin",
	})
	_ = env.Store.SaveArtifact(&common.Artifact{
		ID:       "art-search-2",
		Name:     "otherapp",
		FileName: "otherapp.bin",
	})

	req := httptest.NewRequest(http.MethodGet, "/ui/partials/artifacts?q=myapp", nil)
	rr := httptest.NewRecorder()
	env.HandlePartialArtifacts(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("status = %d; want 200", rr.Code)
	}
}

// ── HandlePartialDeployments: search filter path ──────────────────────────────

func TestHandlePartialDeployments_WithSearchQuery(t *testing.T) {
	env := newTestEnv(t)
	now := time.Now()
	_ = env.Store.SaveDeployment(&common.DeploymentState{
		ID:        "dep-search-1",
		AgentID:   "a1",
		Hostname:  "host-search",
		Status:    common.DeploymentStatusCompleted,
		StartedAt: now,
		UpdatedAt: now,
	})

	req := httptest.NewRequest(http.MethodGet, "/ui/partials/deployments?q=host-search", nil)
	rr := httptest.NewRecorder()
	env.HandlePartialDeployments(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("status = %d; want 200", rr.Code)
	}
}

// ── HandleStatus: method not allowed path ─────────────────────────────────────

func TestHandleStatus_MethodNotAllowed(t *testing.T) {
	env := newTestEnv(t)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/status", nil)
	rr := httptest.NewRecorder()
	env.HandleStatus(rr, req)
	if rr.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d; want 405", rr.Code)
	}
}
