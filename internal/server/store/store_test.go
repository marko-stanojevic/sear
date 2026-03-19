package store_test

import (
	"testing"
	"time"

	"github.com/marko-stanojevic/kompakt/internal/common"
	"github.com/marko-stanojevic/kompakt/internal/server/store"
)

func newTestStore(t *testing.T) *store.Store {
	t.Helper()
	s, err := store.New(t.TempDir(), "")
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

// ── Agents ────────────────────────────────────────────────────────────────────

func TestAgentCRUD(t *testing.T) {
	s := newTestStore(t)

	a := &common.Agent{
		ID:             "agent-1",
		Hostname:       "edge-01",
		Platform:       common.PlatformLinux,
		Status:         common.AgentStatusRegistered,
		RegisteredAt:   time.Now(),
		LastActivityAt: time.Now(),
	}
	if err := s.SaveAgent(a); err != nil {
		t.Fatalf("SaveAgent: %v", err)
	}

	got, ok := s.GetAgent("agent-1")
	if !ok {
		t.Fatal("GetAgent: not found")
	}
	if got.Hostname != "edge-01" {
		t.Errorf("Hostname = %q; want edge-01", got.Hostname)
	}

	list := s.ListAgents()
	if len(list) != 1 {
		t.Errorf("ListAgents len = %d; want 1", len(list))
	}

	if err := s.DeleteAgent("agent-1"); err != nil {
		t.Fatalf("DeleteAgent: %v", err)
	}
	_, ok = s.GetAgent("agent-1")
	if ok {
		t.Error("expected agent to be deleted")
	}
}

func TestListAgentsStableOrder(t *testing.T) {
	s := newTestStore(t)
	now := time.Now()

	agents := []*common.Agent{
		{ID: "id-z", Hostname: "zeta", Status: common.AgentStatusRegistered, RegisteredAt: now, LastActivityAt: now},
		{ID: "id-a", Hostname: "alpha", Status: common.AgentStatusRegistered, RegisteredAt: now, LastActivityAt: now},
		{ID: "id-b", Hostname: "", Status: common.AgentStatusRegistered, RegisteredAt: now, LastActivityAt: now},
		{ID: "id-a2", Hostname: "alpha", Status: common.AgentStatusRegistered, RegisteredAt: now, LastActivityAt: now},
	}
	for _, a := range agents {
		if err := s.SaveAgent(a); err != nil {
			t.Fatalf("SaveAgent(%s): %v", a.ID, err)
		}
	}

	got := s.ListAgents()
	if len(got) != 4 {
		t.Fatalf("ListAgents len = %d; want 4", len(got))
	}

	wantIDs := []string{"id-a", "id-a2", "id-z", "id-b"}
	for i, want := range wantIDs {
		if got[i].ID != want {
			t.Fatalf("ListAgents[%d].ID = %q; want %q", i, got[i].ID, want)
		}
	}

	// Call repeatedly to ensure order remains deterministic.
	for n := 0; n < 3; n++ {
		again := s.ListAgents()
		for i, want := range wantIDs {
			if again[i].ID != want {
				t.Fatalf("ListAgents call %d index %d ID = %q; want %q", n+2, i, again[i].ID, want)
			}
		}
	}
}

// ── Deployments ───────────────────────────────────────────────────────────────

func TestDeploymentCRUD(t *testing.T) {
	s := newTestStore(t)

	d := &common.DeploymentState{
		ID:              "dep-1",
		AgentID:         "agent-1",
		PlaybookID:      "pb-1",
		Status:          common.DeploymentStatusRunning,
		ResumeStepIndex: 2,
		StartedAt:       time.Now(),
		UpdatedAt:       time.Now(),
	}
	if err := s.SaveDeployment(d); err != nil {
		t.Fatalf("SaveDeployment: %v", err)
	}

	got, ok := s.GetDeployment("dep-1")
	if !ok {
		t.Fatal("GetDeployment: not found")
	}
	if got.ResumeStepIndex != 2 {
		t.Errorf("ResumeStepIndex = %d; want 2", got.ResumeStepIndex)
	}

	dep, ok := s.GetActiveDeploymentForAgent("agent-1")
	if !ok {
		t.Fatal("GetActiveDeploymentForAgent: not found")
	}
	if dep.ID != "dep-1" {
		t.Errorf("deployment ID = %q; want dep-1", dep.ID)
	}

	list := s.ListDeployments()
	if len(list) != 1 {
		t.Errorf("ListDeployments len = %d; want 1", len(list))
	}
}

func TestGetActiveDeploymentForAgentMostRecent(t *testing.T) {
	s := newTestStore(t)
	now := time.Now()

	older := &common.DeploymentState{
		ID:        "dep-old",
		AgentID:   "agent-1",
		Status:    common.DeploymentStatusRunning,
		StartedAt: now.Add(-10 * time.Minute),
		UpdatedAt: now.Add(-10 * time.Minute),
	}
	newer := &common.DeploymentState{
		ID:        "dep-new",
		AgentID:   "agent-1",
		Status:    common.DeploymentStatusRunning,
		StartedAt: now,
		UpdatedAt: now,
	}

	if err := s.SaveDeployment(older); err != nil {
		t.Fatalf("SaveDeployment older: %v", err)
	}
	if err := s.SaveDeployment(newer); err != nil {
		t.Fatalf("SaveDeployment newer: %v", err)
	}

	got, ok := s.GetActiveDeploymentForAgent("agent-1")
	if !ok {
		t.Fatal("GetActiveDeploymentForAgent: not found")
	}
	if got.ID != "dep-new" {
		t.Fatalf("deployment ID = %q; want dep-new", got.ID)
	}
}

func TestGetActiveDeploymentForAgentNotFound(t *testing.T) {
	s := newTestStore(t)

	if _, ok := s.GetActiveDeploymentForAgent("missing-agent"); ok {
		t.Fatal("expected no active deployment for unknown agent")
	}
}

// ── Playbooks ─────────────────────────────────────────────────────────────────

func TestPlaybookCRUD(t *testing.T) {
	s := newTestStore(t)

	pb := &store.PlaybookRecord{
		ID:   "pb-1",
		Name: "test",
		Playbook: &common.Playbook{
			Name: "test-playbook",
			Jobs: []common.Job{
				{Name: "setup", Steps: []common.Step{
					{Name: "Install", Run: "echo hi"},
				}},
			},
		},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	if err := s.SavePlaybook(pb); err != nil {
		t.Fatalf("SavePlaybook: %v", err)
	}

	got, ok := s.GetPlaybook("pb-1")
	if !ok {
		t.Fatal("GetPlaybook: not found")
	}
	if got.Playbook.Name != "test-playbook" {
		t.Errorf("Playbook.Name = %q; want test-playbook", got.Playbook.Name)
	}
	if len(got.Playbook.Jobs) != 1 || got.Playbook.Jobs[0].Name != "setup" {
		t.Errorf("unexpected jobs: %+v", got.Playbook.Jobs)
	}

	list := s.ListPlaybooks()
	if len(list) != 1 {
		t.Errorf("ListPlaybooks len = %d; want 1", len(list))
	}

	if err := s.DeletePlaybook("pb-1"); err != nil {
		t.Fatalf("DeletePlaybook: %v", err)
	}
	_, ok = s.GetPlaybook("pb-1")
	if ok {
		t.Error("expected playbook to be deleted")
	}
}

// ── Artifacts ─────────────────────────────────────────────────────────────────

func TestArtifactCRUD(t *testing.T) {
	s := newTestStore(t)

	a := &common.Artifact{
		ID:         "art-1",
		Name:       "myapp",
		Filename:   "myapp.tar.gz",
		Size:       1024,
		UploadedAt: time.Now(),
	}
	if err := s.SaveArtifact(a); err != nil {
		t.Fatalf("SaveArtifact: %v", err)
	}

	got, ok := s.GetArtifact("art-1")
	if !ok {
		t.Fatal("GetArtifact: not found")
	}
	if got.Filename != "myapp.tar.gz" {
		t.Errorf("Filename = %q; want myapp.tar.gz", got.Filename)
	}

	byName, ok := s.GetArtifactByName("myapp")
	if !ok {
		t.Fatal("GetArtifactByName: not found")
	}
	if byName.ID != "art-1" {
		t.Errorf("ID = %q; want art-1", byName.ID)
	}

	all := s.ListArtifacts()
	if len(all) != 1 || all[0].ID != "art-1" {
		t.Fatalf("ListArtifacts = %+v; want one artifact with ID art-1", all)
	}

	if err := s.DeleteArtifact("art-1"); err != nil {
		t.Fatalf("DeleteArtifact: %v", err)
	}
}

// ── Secrets ───────────────────────────────────────────────────────────────────

func TestSecretsCRUD(t *testing.T) {
	s := newTestStore(t)

	if err := s.SetSecret("DB_PASS", "hunter2"); err != nil {
		t.Fatalf("SetSecret: %v", err)
	}

	val, ok := s.GetSecret("DB_PASS")
	if !ok {
		t.Fatal("GetSecret: not found")
	}
	if val != "hunter2" {
		t.Errorf("value = %q; want hunter2", val)
	}

	names := s.ListSecretNames()
	if len(names) != 1 || names[0] != "DB_PASS" {
		t.Errorf("ListSecretNames = %v; want [DB_PASS]", names)
	}

	all := s.AllSecrets()
	if all["DB_PASS"] != "hunter2" {
		t.Errorf("AllSecrets[DB_PASS] = %q; want hunter2", all["DB_PASS"])
	}

	if err := s.DeleteSecret("DB_PASS"); err != nil {
		t.Fatalf("DeleteSecret: %v", err)
	}
	_, ok = s.GetSecret("DB_PASS")
	if ok {
		t.Error("expected secret to be deleted")
	}
}

func TestMergeSecrets(t *testing.T) {
	s := newTestStore(t)
	m := map[string]string{"A": "1", "B": "2"}
	if err := s.MergeSecrets(m); err != nil {
		t.Fatalf("MergeSecrets: %v", err)
	}
	if v, _ := s.GetSecret("A"); v != "1" {
		t.Errorf("A = %q; want 1", v)
	}
}

func TestAllSecretsReturnsCopy(t *testing.T) {
	s := newTestStore(t)
	if err := s.SetSecret("DB_PASS", "hunter2"); err != nil {
		t.Fatalf("SetSecret: %v", err)
	}

	all := s.AllSecrets()
	all["DB_PASS"] = "changed"
	all["NEW"] = "value"

	v, ok := s.GetSecret("DB_PASS")
	if !ok {
		t.Fatal("GetSecret(DB_PASS): not found")
	}
	if v != "hunter2" {
		t.Fatalf("store value mutated via AllSecrets copy: got %q; want hunter2", v)
	}
	if _, ok := s.GetSecret("NEW"); ok {
		t.Fatal("store should not include key added only to AllSecrets copy")
	}
}

// ── Logs — per-deployment files ───────────────────────────────────────────────

func TestLogsQueryByDeploymentAndAgent(t *testing.T) {
	s := newTestStore(t)
	now := time.Now()

	_ = s.SaveDeployment(&common.DeploymentState{
		ID:        "dep-1",
		AgentID:   "agent-1",
		StartedAt: now,
		UpdatedAt: now,
	})

	entries := []*common.LogEntry{
		{DeploymentID: "dep-1", Level: common.LogLevelInfo, Message: "hello", Timestamp: now},
		{DeploymentID: "dep-2", Level: common.LogLevelError, Message: "other", Timestamp: now},
	}
	if err := s.AppendLogs(entries); err != nil {
		t.Fatalf("AppendLogs: %v", err)
	}

	dep1Logs := s.GetLogsForDeployment("dep-1")
	if len(dep1Logs) != 1 {
		t.Errorf("GetLogsForDeployment(dep-1) len = %d; want 1", len(dep1Logs))
	}

	agentLogs := s.GetLogsForAgent("agent-1")
	if len(agentLogs) != 1 {
		t.Errorf("GetLogsForAgent(agent-1) len = %d; want 1", len(agentLogs))
	}
}

// ── Persistence ───────────────────────────────────────────────────────────────

func TestPersistence_ReopenPreservesAgentAndSecret(t *testing.T) {
	dir := t.TempDir()

	s1, err := store.New(dir, "")
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	_ = s1.SaveAgent(&common.Agent{
		ID:       "a1",
		Hostname: "host-1",
		Status:   common.AgentStatusRegistered,
	})
	_ = s1.SetSecret("K", "V")
	_ = s1.Close()

	// Reopen and verify.
	s2, err := store.New(dir, "")
	if err != nil {
		t.Fatalf("store.New (reopen): %v", err)
	}
	t.Cleanup(func() { _ = s2.Close() })
	a, ok := s2.GetAgent("a1")
	if !ok {
		t.Fatal("agent not found after reopen")
	}
	if a.Hostname != "host-1" {
		t.Errorf("Hostname = %q after reopen; want host-1", a.Hostname)
	}
	v, ok := s2.GetSecret("K")
	if !ok || v != "V" {
		t.Errorf("secret after reopen: ok=%v val=%q", ok, v)
	}
}

func TestDeleteDeployment(t *testing.T) {
	s := newTestStore(t)
	now := time.Now()

	dep := &common.DeploymentState{
		ID:        "dep-del-1",
		AgentID:   "agent-del",
		Status:    common.DeploymentStatusRunning,
		StartedAt: now,
		UpdatedAt: now,
	}
	if err := s.SaveDeployment(dep); err != nil {
		t.Fatalf("SaveDeployment: %v", err)
	}
	// Append some logs so the log file exists.
	_ = s.AppendLogs([]*common.LogEntry{
		{DeploymentID: "dep-del-1", Level: common.LogLevelInfo, Message: "test", Timestamp: now},
	})

	if err := s.DeleteDeployment("dep-del-1"); err != nil {
		t.Fatalf("DeleteDeployment: %v", err)
	}
	if _, ok := s.GetDeployment("dep-del-1"); ok {
		t.Error("deployment should be deleted")
	}
	// Logs for that deployment should now be empty.
	if logs := s.GetLogsForDeployment("dep-del-1"); len(logs) != 0 {
		t.Errorf("expected 0 logs after delete; got %d", len(logs))
	}
}

func TestLogsPersistAcrossReopen(t *testing.T) {
	dir := t.TempDir()
	now := time.Now()

	s1, err := store.New(dir, "")
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	_ = s1.AppendLogs([]*common.LogEntry{
		{DeploymentID: "dep-x", Level: common.LogLevelInfo, Message: "sentinellogline", Timestamp: now},
	})
	_ = s1.Close()

	s2, err := store.New(dir, "")
	if err != nil {
		t.Fatalf("store.New (reopen): %v", err)
	}
	t.Cleanup(func() { _ = s2.Close() })

	logs := s2.GetLogsForDeployment("dep-x")
	if len(logs) != 1 {
		t.Fatalf("expected 1 log after reopen; got %d", len(logs))
	}
	if logs[0].Message != "sentinellogline" {
		t.Errorf("message = %q; want sentinellogline", logs[0].Message)
	}
}
