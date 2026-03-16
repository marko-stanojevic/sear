package service_test

import (
	"errors"
	"testing"
	"time"

	"github.com/marko-stanojevic/sear/internal/common"
	"github.com/marko-stanojevic/sear/internal/daemon/service"
	"github.com/marko-stanojevic/sear/internal/daemon/store"
)

type sentMessage struct {
	clientID string
	msg      common.WSMessage
}

type fakeHub struct {
	connected map[string]bool
	sent      []sentMessage
}

func (h *fakeHub) IsConnected(clientID string) bool {
	return h.connected[clientID]
}

func (h *fakeHub) Send(clientID string, msg common.WSMessage) bool {
	h.sent = append(h.sent, sentMessage{clientID: clientID, msg: msg})
	return true
}

func newTestManager(t *testing.T) (*service.Manager, *store.Store, *fakeHub) {
	t.Helper()
	st, err := store.New(t.TempDir(), "")
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	hub := &fakeHub{connected: map[string]bool{}}
	mgr := &service.Manager{
		Store:     st,
		Hub:       hub,
		ServerURL: "http://localhost:8080",
	}
	return mgr, st, hub
}

func saveClientAndPlaybook(t *testing.T, st *store.Store, clientID, playbookID string) {
	t.Helper()
	now := time.Now()
	if err := st.SaveClient(&common.Client{
		ID:             clientID,
		Hostname:       "edge-1",
		Platform:       common.PlatformLinux,
		Status:         common.ClientStatusRegistered,
		RegisteredAt:   now,
		LastActivityAt: now,
	}); err != nil {
		t.Fatalf("SaveClient: %v", err)
	}
	if err := st.SavePlaybook(&store.PlaybookRecord{
		ID:   playbookID,
		Name: "deploy",
		Playbook: &common.Playbook{
			Name: "deploy-playbook",
			Jobs: []common.Job{{
				Name: "job1",
				Steps: []common.Step{{
					Name: "step1",
					Run:  "echo ok",
				}},
			}},
		},
		CreatedAt: now,
		UpdatedAt: now,
	}); err != nil {
		t.Fatalf("SavePlaybook: %v", err)
	}
}

func TestStatusSnapshot(t *testing.T) {
	mgr, st, _ := newTestManager(t)
	now := time.Now()
	_ = st.SaveClient(&common.Client{ID: "c1", Hostname: "h1", Platform: common.PlatformLinux, Status: common.ClientStatusRegistered, RegisteredAt: now, LastActivityAt: now})
	_ = st.SaveDeployment(&common.DeploymentState{ID: "d1", ClientID: "c1", PlaybookID: "p1", Status: common.DeploymentStatusRunning, StartedAt: now, UpdatedAt: now})

	clients, deployments := mgr.StatusSnapshot()
	if len(clients) != 1 {
		t.Fatalf("clients len = %d; want 1", len(clients))
	}
	if len(deployments) != 1 {
		t.Fatalf("deployments len = %d; want 1", len(deployments))
	}
}

func TestAssignPlaybookToClientErrors(t *testing.T) {
	mgr, st, _ := newTestManager(t)

	err := mgr.AssignPlaybookToClient("pb-1", "missing-client")
	if !errors.Is(err, service.ErrClientNotFound) {
		t.Fatalf("expected ErrClientNotFound, got %v", err)
	}

	now := time.Now()
	_ = st.SaveClient(&common.Client{ID: "c1", Hostname: "h1", Platform: common.PlatformLinux, Status: common.ClientStatusRegistered, RegisteredAt: now, LastActivityAt: now})
	err = mgr.AssignPlaybookToClient("missing-playbook", "c1")
	if !errors.Is(err, service.ErrPlaybookNotFound) {
		t.Fatalf("expected ErrPlaybookNotFound, got %v", err)
	}
}

func TestAssignPlaybookToClientConnectedPushesPlaybook(t *testing.T) {
	mgr, st, hub := newTestManager(t)
	saveClientAndPlaybook(t, st, "c1", "pb-1")
	hub.connected["c1"] = true

	if err := mgr.AssignPlaybookToClient("pb-1", "c1"); err != nil {
		t.Fatalf("AssignPlaybookToClient: %v", err)
	}

	client, ok := st.GetClient("c1")
	if !ok {
		t.Fatal("client not found")
	}
	if client.PlaybookID != "pb-1" {
		t.Fatalf("PlaybookID = %q; want pb-1", client.PlaybookID)
	}
	if client.Status != common.ClientStatusDeploying {
		t.Fatalf("Status = %q; want %q", client.Status, common.ClientStatusDeploying)
	}

	if len(hub.sent) != 1 {
		t.Fatalf("sent messages = %d; want 1", len(hub.sent))
	}
	if hub.sent[0].msg.Type != common.WSMsgPlaybook {
		t.Fatalf("message type = %q; want %q", hub.sent[0].msg.Type, common.WSMsgPlaybook)
	}

	deployments := st.ListDeployments()
	if len(deployments) != 1 {
		t.Fatalf("deployments len = %d; want 1", len(deployments))
	}
	if deployments[0].Status != common.DeploymentStatusRunning {
		t.Fatalf("deployment status = %q; want %q", deployments[0].Status, common.DeploymentStatusRunning)
	}
}

func TestPushPlaybookIfAssignedResumesActiveDeployment(t *testing.T) {
	mgr, st, hub := newTestManager(t)
	saveClientAndPlaybook(t, st, "c1", "pb-1")

	client, _ := st.GetClient("c1")
	client.PlaybookID = "pb-1"
	_ = st.SaveClient(client)

	now := time.Now()
	_ = st.SaveDeployment(&common.DeploymentState{
		ID:              "dep-1",
		ClientID:        "c1",
		PlaybookID:      "pb-1",
		Status:          common.DeploymentStatusRunning,
		ResumeStepIndex: 3,
		StartedAt:       now.Add(-time.Minute),
		UpdatedAt:       now.Add(-time.Minute),
	})

	mgr.PushPlaybookIfAssigned("c1", false)

	if len(hub.sent) != 1 {
		t.Fatalf("sent messages = %d; want 1", len(hub.sent))
	}
	data, ok := hub.sent[0].msg.Data.(common.WSPlaybookData)
	if !ok {
		t.Fatalf("message data type = %T; want common.WSPlaybookData", hub.sent[0].msg.Data)
	}
	if data.DeploymentID != "dep-1" {
		t.Fatalf("deployment_id = %q; want dep-1", data.DeploymentID)
	}
	if data.ResumeStepIndex != 3 {
		t.Fatalf("resume_step_index = %d; want 3", data.ResumeStepIndex)
	}
}

func TestPushPlaybookIfAssignedSkipsCompleted(t *testing.T) {
	mgr, st, hub := newTestManager(t)
	saveClientAndPlaybook(t, st, "c1", "pb-1")

	client, _ := st.GetClient("c1")
	client.PlaybookID = "pb-1"
	_ = st.SaveClient(client)

	now := time.Now()
	_ = st.SaveDeployment(&common.DeploymentState{
		ID:         "dep-done",
		ClientID:   "c1",
		PlaybookID: "pb-1",
		Status:     common.DeploymentStatusDone,
		StartedAt:  now.Add(-time.Hour),
		FinishedAt: &now,
	})

	// 1. Reconnect (force=false) -> Should skip
	mgr.PushPlaybookIfAssigned("c1", false)
	if len(hub.sent) != 0 {
		t.Fatalf("sent messages = %d; want 0 (skipped)", len(hub.sent))
	}

	// 2. Manual re-run (force=true) -> Should start new
	mgr.PushPlaybookIfAssigned("c1", true)
	if len(hub.sent) != 1 {
		t.Fatalf("sent messages = %d; want 1 (new deployment)", len(hub.sent))
	}
	data := hub.sent[0].msg.Data.(common.WSPlaybookData)
	if data.DeploymentID == "dep-done" {
		t.Fatal("expected new deployment ID, got the old one")
	}
}

func TestAppendDeploymentLogAndResolvePlaybookName(t *testing.T) {
	mgr, st, _ := newTestManager(t)

	mgr.AppendDeploymentLog("", "job1", 0, common.LogLevelInfo, "ignored")
	if got := st.GetLogsForDeployment("dep-1"); len(got) != 0 {
		t.Fatalf("unexpected logs for empty deployment id: %d", len(got))
	}

	mgr.AppendDeploymentLog("dep-1", "job1", 1, common.LogLevelInfo, "hello")
	logs := st.GetLogsForDeployment("dep-1")
	if len(logs) != 1 {
		t.Fatalf("logs len = %d; want 1", len(logs))
	}
	if logs[0].Message != "hello" {
		t.Fatalf("log message = %q; want hello", logs[0].Message)
	}

	if got := mgr.ResolvePlaybookNameByDeployment("missing"); got != "playbook" {
		t.Fatalf("default playbook name = %q; want playbook", got)
	}

	now := time.Now()
	_ = st.SavePlaybook(&store.PlaybookRecord{
		ID:   "pb-2",
		Name: "record-name",
		Playbook: &common.Playbook{
			Name: "yaml-name",
			Jobs: []common.Job{{Name: "j", Steps: []common.Step{{Name: "s", Run: "echo hi"}}}},
		},
		CreatedAt: now,
		UpdatedAt: now,
	})
	_ = st.SaveDeployment(&common.DeploymentState{ID: "dep-2", ClientID: "c1", PlaybookID: "pb-2", Status: common.DeploymentStatusRunning, StartedAt: now, UpdatedAt: now})

	if got := mgr.ResolvePlaybookNameByDeployment("dep-2"); got != "yaml-name" {
		t.Fatalf("playbook name = %q; want yaml-name", got)
	}
}
