package store_test

import (
	"testing"
	"time"

	"github.com/marko-stanojevic/kompakt/internal/common"
	"github.com/marko-stanojevic/kompakt/internal/server/store"
)

// agentForToken inserts a minimal agent row so FK constraints are satisfied.
func agentForToken(t *testing.T, s *store.Store, id string) {
	t.Helper()
	now := time.Now()
	if err := s.SaveAgent(&common.Agent{
		ID:             id,
		Hostname:       id,
		Status:         common.AgentStatusRegistered,
		RegisteredAt:   now,
		LastActivityAt: now,
	}); err != nil {
		t.Fatalf("SaveAgent(%s): %v", id, err)
	}
}

// ── CreateAgentToken / GetAgentTokenByHash ─────────────────────────────────

func TestAgentTokenCRUD(t *testing.T) {
	s := newTestStore(t)
	agentForToken(t, s, "a1")

	exp := time.Now().Add(24 * time.Hour)
	tok := &common.AgentToken{
		ID:        "tok-1",
		AgentID:   "a1",
		TokenHash: "sha256hashvalue",
		CreatedAt: time.Now(),
		ExpiresAt: &exp,
	}
	if err := s.CreateAgentToken(tok); err != nil {
		t.Fatalf("CreateAgentToken: %v", err)
	}

	got, err := s.GetAgentTokenByHash("sha256hashvalue")
	if err != nil {
		t.Fatalf("GetAgentTokenByHash: %v", err)
	}
	if got.ID != "tok-1" {
		t.Errorf("ID = %q; want tok-1", got.ID)
	}
	if got.AgentID != "a1" {
		t.Errorf("AgentID = %q; want a1", got.AgentID)
	}
	if got.RevokedAt != nil {
		t.Error("expected RevokedAt to be nil on fresh token")
	}
	if got.ExpiresAt == nil {
		t.Error("expected ExpiresAt to be persisted")
	}
}

func TestAgentToken_UnknownHashReturnsError(t *testing.T) {
	s := newTestStore(t)
	if _, err := s.GetAgentTokenByHash("nonexistent-hash"); err == nil {
		t.Error("expected error for unknown hash; got nil")
	}
}

// ── RevokeAgentToken ────────────────────────────────────────────────────────

func TestAgentToken_Revoke(t *testing.T) {
	s := newTestStore(t)
	agentForToken(t, s, "a2")

	tok := &common.AgentToken{
		ID:        "tok-2",
		AgentID:   "a2",
		TokenHash: "hash-revoke",
		CreatedAt: time.Now(),
	}
	if err := s.CreateAgentToken(tok); err != nil {
		t.Fatalf("CreateAgentToken: %v", err)
	}

	if err := s.RevokeAgentToken("tok-2"); err != nil {
		t.Fatalf("RevokeAgentToken: %v", err)
	}

	got, err := s.GetAgentTokenByHash("hash-revoke")
	if err != nil {
		t.Fatalf("GetAgentTokenByHash after revoke: %v", err)
	}
	if got.RevokedAt == nil {
		t.Error("expected RevokedAt to be set after revocation")
	}
}

// ── RevokeAllAgentTokens ────────────────────────────────────────────────────

func TestAgentToken_RevokeAll(t *testing.T) {
	s := newTestStore(t)
	agentForToken(t, s, "a3")

	hashes := []string{"hash-all-a", "hash-all-b", "hash-all-c"}
	for i, h := range hashes {
		tok := &common.AgentToken{
			ID:        "tok-all-" + string(rune('a'+i)),
			AgentID:   "a3",
			TokenHash: h,
			CreatedAt: time.Now(),
		}
		if err := s.CreateAgentToken(tok); err != nil {
			t.Fatalf("CreateAgentToken(%s): %v", h, err)
		}
	}

	if err := s.RevokeAllAgentTokens("a3"); err != nil {
		t.Fatalf("RevokeAllAgentTokens: %v", err)
	}

	for _, h := range hashes {
		got, err := s.GetAgentTokenByHash(h)
		if err != nil {
			t.Errorf("GetAgentTokenByHash(%s): %v", h, err)
			continue
		}
		if got.RevokedAt == nil {
			t.Errorf("token %s: expected RevokedAt to be set after RevokeAll", h)
		}
	}
}

func TestAgentToken_RevokeAllOnlyAffectsTargetAgent(t *testing.T) {
	s := newTestStore(t)
	agentForToken(t, s, "a4")
	agentForToken(t, s, "a5")

	_ = s.CreateAgentToken(&common.AgentToken{ID: "tok-a4", AgentID: "a4", TokenHash: "hash-a4", CreatedAt: time.Now()})
	_ = s.CreateAgentToken(&common.AgentToken{ID: "tok-a5", AgentID: "a5", TokenHash: "hash-a5", CreatedAt: time.Now()})

	if err := s.RevokeAllAgentTokens("a4"); err != nil {
		t.Fatalf("RevokeAllAgentTokens(a4): %v", err)
	}

	// a4's token should be revoked.
	got4, _ := s.GetAgentTokenByHash("hash-a4")
	if got4.RevokedAt == nil {
		t.Error("a4 token: expected RevokedAt to be set")
	}

	// a5's token must remain untouched.
	got5, _ := s.GetAgentTokenByHash("hash-a5")
	if got5.RevokedAt != nil {
		t.Error("a5 token: should not be revoked when revoking a4's tokens")
	}
}

// ── ON DELETE CASCADE ────────────────────────────────────────────────────────

func TestAgentToken_DeleteAgentCascade(t *testing.T) {
	s := newTestStore(t)
	agentForToken(t, s, "a6")

	_ = s.CreateAgentToken(&common.AgentToken{
		ID:        "tok-cascade",
		AgentID:   "a6",
		TokenHash: "hash-cascade",
		CreatedAt: time.Now(),
	})

	if _, err := s.GetAgentTokenByHash("hash-cascade"); err != nil {
		t.Fatalf("token should exist before agent delete: %v", err)
	}

	if err := s.DeleteAgent("a6"); err != nil {
		t.Fatalf("DeleteAgent: %v", err)
	}

	if _, err := s.GetAgentTokenByHash("hash-cascade"); err == nil {
		t.Error("expected token to be deleted with agent (ON DELETE CASCADE); got nil error")
	}
}

// ── Persistence across reopen ────────────────────────────────────────────────

func TestAgentToken_Persistence(t *testing.T) {
	dir := t.TempDir()

	s1, err := store.New(dir, "")
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	now := time.Now()
	_ = s1.SaveAgent(&common.Agent{ID: "a7", Hostname: "h", Status: common.AgentStatusRegistered, RegisteredAt: now, LastActivityAt: now})
	exp := now.Add(24 * time.Hour)
	_ = s1.CreateAgentToken(&common.AgentToken{
		ID:        "tok-persist",
		AgentID:   "a7",
		TokenHash: "hash-persist",
		CreatedAt: now,
		ExpiresAt: &exp,
	})
	_ = s1.Close()

	s2, err := store.New(dir, "")
	if err != nil {
		t.Fatalf("store.New (reopen): %v", err)
	}
	t.Cleanup(func() { _ = s2.Close() })

	got, err := s2.GetAgentTokenByHash("hash-persist")
	if err != nil {
		t.Fatalf("GetAgentTokenByHash after reopen: %v", err)
	}
	if got.ID != "tok-persist" {
		t.Errorf("ID = %q; want tok-persist", got.ID)
	}
	if got.AgentID != "a7" {
		t.Errorf("AgentID = %q; want a7", got.AgentID)
	}
	if got.ExpiresAt == nil {
		t.Error("ExpiresAt should be persisted across reopen")
	}
	if got.RevokedAt != nil {
		t.Error("RevokedAt should be nil on freshly created token")
	}
}
