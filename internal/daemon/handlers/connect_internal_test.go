package handlers

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/marko-stanojevic/sear/internal/common"
	"github.com/marko-stanojevic/sear/internal/daemon/store"
)

func newConnectTestEnv(t *testing.T) *Env {
	t.Helper()
	st, err := store.New(t.TempDir(), "")
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	return &Env{Store: st, Hub: NewHub()}
}

func saveClient(t *testing.T, e *Env, id string) {
	t.Helper()
	now := time.Now()
	err := e.Store.SaveClient(&common.Client{
		ID:             id,
		Hostname:       id,
		Platform:       common.PlatformLinux,
		Status:         common.ClientStatusConnected,
		RegisteredAt:   now,
		LastActivityAt: now,
	})
	if err != nil {
		t.Fatalf("SaveClient: %v", err)
	}
}

func saveDeployment(t *testing.T, e *Env, depID, clientID string, status common.DeploymentStatus) {
	t.Helper()
	now := time.Now()
	err := e.Store.SaveDeployment(&common.DeploymentState{
		ID:         depID,
		ClientID:   clientID,
		PlaybookID: "pb-1",
		Status:     status,
		StartedAt:  now,
		UpdatedAt:  now,
	})
	if err != nil {
		t.Fatalf("SaveDeployment: %v", err)
	}
}

func sendWS(t *testing.T, e *Env, clientID string, msgType common.WSMessageType, data any) {
	t.Helper()
	b, err := json.Marshal(common.WSMessage{Type: msgType, Data: data})
	if err != nil {
		t.Fatalf("marshal message: %v", err)
	}
	e.handleWSMessage(clientID, b)
}

func TestHandleWSMessage_LogAndLifecycleUpdates(t *testing.T) {
	e := newConnectTestEnv(t)
	saveClient(t, e, "c1")
	saveDeployment(t, e, "dep-1", "c1", common.DeploymentStatusPending)

	sendWS(t, e, "c1", common.WSMsgLog, common.WSLogData{
		DeploymentID: "dep-1",
		JobName:      "job1",
		StepIndex:    0,
		Level:        common.LogLevelInfo,
		Message:      "hello",
	})
	logs := e.Store.GetLogsForDeployment("dep-1")
	if len(logs) != 1 {
		t.Fatalf("logs len = %d; want 1", len(logs))
	}

	sendWS(t, e, "c1", common.WSMsgStepStart, common.WSStepData{
		DeploymentID: "dep-1",
		JobName:      "job1",
		StepName:     "s1",
		StepIndex:    2,
	})
	dep, _ := e.Store.GetDeployment("dep-1")
	if dep.Status != common.DeploymentStatusRunning || dep.ResumeStepIndex != 2 {
		t.Fatalf("after step_start: status=%q resume=%d", dep.Status, dep.ResumeStepIndex)
	}

	sendWS(t, e, "c1", common.WSMsgStepComplete, common.WSStepData{
		DeploymentID: "dep-1",
		JobName:      "job1",
		StepName:     "s1",
		StepIndex:    2,
	})
	dep, _ = e.Store.GetDeployment("dep-1")
	if dep.ResumeStepIndex != 3 {
		t.Fatalf("after step_complete resume=%d; want 3", dep.ResumeStepIndex)
	}

	sendWS(t, e, "c1", common.WSMsgReboot, common.WSRebootData{
		DeploymentID:    "dep-1",
		ResumeStepIndex: 5,
		Reason:          "reboot",
	})
	dep, _ = e.Store.GetDeployment("dep-1")
	if dep.Status != common.DeploymentStatusRebooting || dep.ResumeStepIndex != 5 {
		t.Fatalf("after reboot: status=%q resume=%d", dep.Status, dep.ResumeStepIndex)
	}

	sendWS(t, e, "c1", common.WSMsgDeployDone, common.WSStepData{DeploymentID: "dep-1"})
	dep, _ = e.Store.GetDeployment("dep-1")
	if dep.Status != common.DeploymentStatusDone || dep.FinishedAt == nil {
		t.Fatalf("after deploy_done: status=%q finished_at_nil=%v", dep.Status, dep.FinishedAt == nil)
	}
	client, _ := e.Store.GetClient("c1")
	if client.Status != common.ClientStatusDone {
		t.Fatalf("client status = %q; want %q", client.Status, common.ClientStatusDone)
	}
}

func TestHandleWSMessage_DeployFailedAndInvalidMessages(t *testing.T) {
	e := newConnectTestEnv(t)
	saveClient(t, e, "c2")
	saveDeployment(t, e, "dep-2", "c2", common.DeploymentStatusRunning)

	before, _ := e.Store.GetDeployment("dep-2")
	e.handleWSMessage("c2", []byte("not-json"))
	e.handleWSMessage("c2", []byte(`{"type":"step_start","data":{`))
	afterInvalid, _ := e.Store.GetDeployment("dep-2")
	if before.Status != afterInvalid.Status || before.ResumeStepIndex != afterInvalid.ResumeStepIndex {
		t.Fatal("invalid messages should not mutate deployment state")
	}

	sendWS(t, e, "c2", common.WSMsgDeployFailed, common.WSStepData{
		DeploymentID: "dep-2",
		JobName:      "job1",
		StepIndex:    7,
		Error:        "boom",
	})
	dep, _ := e.Store.GetDeployment("dep-2")
	if dep.Status != common.DeploymentStatusFailed || dep.ErrorDetail != "boom" || dep.FinishedAt == nil {
		t.Fatalf("after deploy_failed: status=%q error=%q finished_at_nil=%v", dep.Status, dep.ErrorDetail, dep.FinishedAt == nil)
	}
	client, _ := e.Store.GetClient("c2")
	if client.Status != common.ClientStatusFailed {
		t.Fatalf("client status = %q; want %q", client.Status, common.ClientStatusFailed)
	}
}
