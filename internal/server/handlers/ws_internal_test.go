package handlers

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/marko-stanojevic/kompakt/internal/common"
	"github.com/marko-stanojevic/kompakt/internal/server/store"
)

// ── Test infrastructure ───────────────────────────────────────────────────────

func newWSTestEnv(t *testing.T) *Handler {
	t.Helper()
	st, err := store.New(t.TempDir(), "")
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return &Handler{
		Store:    st,
		Hub:      NewHub(),
		Commands: NewCommandStore(),
	}
}

func saveAgent(t *testing.T, e *Handler, id string) {
	t.Helper()
	now := time.Now()
	err := e.Store.SaveAgent(&common.Agent{
		ID:             id,
		Hostname:       id,
		Platform:       common.PlatformLinux,
		Status:         common.AgentStatusConnected,
		RegisteredAt:   now,
		LastActivityAt: now,
	})
	if err != nil {
		t.Fatalf("SaveAgent: %v", err)
	}
}

func saveDeployment(t *testing.T, e *Handler, depID, agentID string, status common.DeploymentStatus) {
	t.Helper()
	now := time.Now()
	err := e.Store.SaveDeployment(&common.DeploymentState{
		ID:         depID,
		AgentID:    agentID,
		PlaybookID: "pb-1",
		Status:     status,
		StartedAt:  now,
		UpdatedAt:  now,
	})
	if err != nil {
		t.Fatalf("SaveDeployment: %v", err)
	}
}

func sendWS(t *testing.T, e *Handler, agentID string, msgType common.WSMessageType, data any) {
	t.Helper()
	b, err := json.Marshal(common.WSMessage{Type: msgType, Data: data})
	if err != nil {
		t.Fatalf("marshal message: %v", err)
	}
	e.handleWSMessage(agentID, b)
}

// ── Deployment lifecycle ──────────────────────────────────────────────────────

func TestHandleWSMessage_LogAppendedToStore(t *testing.T) {
	e := newWSTestEnv(t)
	saveAgent(t, e, "c1")
	saveDeployment(t, e, "dep-1", "c1", common.DeploymentStatusPending)

	sendWS(t, e, "c1", common.WSMsgLog, common.WSLogData{
		DeploymentID: "dep-1",
		JobName:      "job1",
		Level:        common.LogLevelInfo,
		Message:      "hello",
	})

	logs := e.Store.GetLogsForDeployment("dep-1")
	if len(logs) != 1 || logs[0].Message != "hello" {
		t.Fatalf("expected 1 log entry with message 'hello', got %v", logs)
	}
}

func TestHandleWSMessage_LogLevelVariants_AllStoredCorrectly(t *testing.T) {
	e := newWSTestEnv(t)
	saveAgent(t, e, "c-log")
	saveDeployment(t, e, "dep-log", "c-log", common.DeploymentStatusRunning)

	for _, level := range []common.LogLevel{common.LogLevelWarn, common.LogLevelError} {
		sendWS(t, e, "c-log", common.WSMsgLog, common.WSLogData{
			DeploymentID: "dep-log",
			Level:        level,
			Message:      "msg-" + string(level),
		})
	}

	logs := e.Store.GetLogsForDeployment("dep-log")
	if len(logs) != 2 {
		t.Fatalf("expected 2 log entries, got %d", len(logs))
	}
}

func TestHandleWSMessage_StepStart_SetsRunningStatusAndResumeIndex(t *testing.T) {
	e := newWSTestEnv(t)
	saveAgent(t, e, "c1")
	saveDeployment(t, e, "dep-1", "c1", common.DeploymentStatusPending)

	sendWS(t, e, "c1", common.WSMsgStepStart, common.WSStepData{
		DeploymentID: "dep-1",
		JobName:      "job1",
		StepName:     "s1",
		StepIndex:    2,
	})

	dep, _ := e.Store.GetDeployment("dep-1")
	if dep.Status != common.DeploymentStatusRunning {
		t.Errorf("status = %q; want running", dep.Status)
	}
	if dep.ResumeStepIndex != 2 {
		t.Errorf("ResumeStepIndex = %d; want 2", dep.ResumeStepIndex)
	}
}

func TestHandleWSMessage_StepStart_EmptyNameUsesStepNFallback(t *testing.T) {
	e := newWSTestEnv(t)
	saveAgent(t, e, "c-start-empty")
	saveDeployment(t, e, "dep-start-empty", "c-start-empty", common.DeploymentStatusPending)

	sendWS(t, e, "c-start-empty", common.WSMsgStepStart, common.WSStepData{
		DeploymentID: "dep-start-empty",
		JobName:      "job1",
		StepIndex:    0,
		// StepName intentionally empty
	})

	dep, _ := e.Store.GetDeployment("dep-start-empty")
	if dep.Status != common.DeploymentStatusRunning {
		t.Fatalf("status = %q; want running", dep.Status)
	}
}

func TestHandleWSMessage_StepComplete_AdvancesResumeIndex(t *testing.T) {
	e := newWSTestEnv(t)
	saveAgent(t, e, "c1")
	saveDeployment(t, e, "dep-1", "c1", common.DeploymentStatusRunning)

	sendWS(t, e, "c1", common.WSMsgStepComplete, common.WSStepData{
		DeploymentID: "dep-1",
		JobName:      "job1",
		StepName:     "s1",
		StepIndex:    2,
	})

	dep, _ := e.Store.GetDeployment("dep-1")
	if dep.ResumeStepIndex != 3 {
		t.Errorf("ResumeStepIndex = %d; want 3 (StepIndex+1)", dep.ResumeStepIndex)
	}
}

func TestHandleWSMessage_StepFailed_SetsFailedStatus(t *testing.T) {
	e := newWSTestEnv(t)
	saveAgent(t, e, "c-fail")
	saveDeployment(t, e, "dep-fail", "c-fail", common.DeploymentStatusRunning)

	sendWS(t, e, "c-fail", common.WSMsgStepFailed, common.WSStepData{
		DeploymentID: "dep-fail",
		JobName:      "job1",
		StepName:     "install",
		StepIndex:    1,
		Error:        "exit status 1",
	})

	dep, _ := e.Store.GetDeployment("dep-fail")
	if dep.Status != common.DeploymentStatusFailed {
		t.Errorf("status = %q; want failed", dep.Status)
	}
	if dep.ErrorDetail != "exit status 1" {
		t.Errorf("ErrorDetail = %q; want 'exit status 1'", dep.ErrorDetail)
	}
}

func TestHandleWSMessage_StepFailed_EmptyNameDoesNotPanic(t *testing.T) {
	e := newWSTestEnv(t)
	saveAgent(t, e, "c-fail2")
	saveDeployment(t, e, "dep-fail2", "c-fail2", common.DeploymentStatusRunning)

	sendWS(t, e, "c-fail2", common.WSMsgStepFailed, common.WSStepData{
		DeploymentID: "dep-fail2",
		JobName:      "job1",
		StepIndex:    3,
		Error:        "boom",
	})

	dep, _ := e.Store.GetDeployment("dep-fail2")
	if dep.Status != common.DeploymentStatusFailed {
		t.Fatalf("status = %q; want failed", dep.Status)
	}
}

func TestHandleWSMessage_Reboot_SetsRebootingStatusAndResumeIndex(t *testing.T) {
	e := newWSTestEnv(t)
	saveAgent(t, e, "c1")
	saveDeployment(t, e, "dep-1", "c1", common.DeploymentStatusRunning)

	sendWS(t, e, "c1", common.WSMsgReboot, common.WSRebootData{
		DeploymentID:    "dep-1",
		ResumeStepIndex: 5,
		Reason:          "test reboot",
	})

	dep, _ := e.Store.GetDeployment("dep-1")
	if dep.Status != common.DeploymentStatusRebooting {
		t.Errorf("status = %q; want rebooting", dep.Status)
	}
	if dep.ResumeStepIndex != 5 {
		t.Errorf("ResumeStepIndex = %d; want 5", dep.ResumeStepIndex)
	}
}

func TestHandleWSMessage_DeployDone_SetsCompletedStatusAndTimestamp(t *testing.T) {
	e := newWSTestEnv(t)
	saveAgent(t, e, "c1")
	saveDeployment(t, e, "dep-1", "c1", common.DeploymentStatusRunning)

	sendWS(t, e, "c1", common.WSMsgDeployDone, common.WSStepData{DeploymentID: "dep-1"})

	dep, _ := e.Store.GetDeployment("dep-1")
	if dep.Status != common.DeploymentStatusCompleted {
		t.Errorf("status = %q; want completed", dep.Status)
	}
	if dep.FinishedAt == nil {
		t.Error("FinishedAt should be set")
	}
	agent, _ := e.Store.GetAgent("c1")
	if agent.Status != common.AgentStatusCompleted {
		t.Errorf("agent status = %q; want completed", agent.Status)
	}
}

func TestHandleWSMessage_DeployFailed_SetsFailedStatusAndTimestamp(t *testing.T) {
	e := newWSTestEnv(t)
	saveAgent(t, e, "c2")
	saveDeployment(t, e, "dep-2", "c2", common.DeploymentStatusRunning)

	sendWS(t, e, "c2", common.WSMsgDeployFailed, common.WSStepData{
		DeploymentID: "dep-2",
		JobName:      "job1",
		StepIndex:    7,
		Error:        "boom",
	})

	dep, _ := e.Store.GetDeployment("dep-2")
	if dep.Status != common.DeploymentStatusFailed {
		t.Errorf("status = %q; want failed", dep.Status)
	}
	if dep.ErrorDetail != "boom" {
		t.Errorf("ErrorDetail = %q; want 'boom'", dep.ErrorDetail)
	}
	if dep.FinishedAt == nil {
		t.Error("FinishedAt should be set")
	}
	agent, _ := e.Store.GetAgent("c2")
	if agent.Status != common.AgentStatusFailed {
		t.Errorf("agent status = %q; want failed", agent.Status)
	}
}

// ── Command streaming ─────────────────────────────────────────────────────────

func TestHandleWSMessage_CommandStream_AppendsOutputToSession(t *testing.T) {
	e := newWSTestEnv(t)
	e.Commands.Create("cmd-1", "c3", "echo hi")
	saveAgent(t, e, "c3")

	sendWS(t, e, "c3", common.WSMsgCommandStream, common.WSCommandChunk{
		CmdID:  "cmd-1",
		Output: "output line 1",
	})
	sendWS(t, e, "c3", common.WSMsgCommandStream, common.WSCommandChunk{
		CmdID:  "cmd-1",
		Output: "output line 2",
	})

	sess, ok := e.Commands.Get("cmd-1")
	if !ok {
		t.Fatal("command session not found")
	}
	out, done, _, _ := sess.Snapshot()
	if len(out) != 2 {
		t.Fatalf("expected 2 output lines, got %d: %v", len(out), out)
	}
	if out[0] != "output line 1" || out[1] != "output line 2" {
		t.Errorf("output = %v", out)
	}
	if done {
		t.Error("session should not be done after stream messages")
	}
}

func TestHandleWSMessage_CommandStream_UnknownSessionIsNoOp(t *testing.T) {
	e := newWSTestEnv(t)
	// No session created — should not panic.
	sendWS(t, e, "c3", common.WSMsgCommandStream, common.WSCommandChunk{
		CmdID:  "nonexistent",
		Output: "line",
	})
}

func TestHandleWSMessage_CommandCompleted_MarksSessionDone(t *testing.T) {
	e := newWSTestEnv(t)
	e.Commands.Create("cmd-2", "c4", "ls")
	saveAgent(t, e, "c4")

	sendWS(t, e, "c4", common.WSMsgCommandCompleted, common.WSCommandStatus{
		CmdID:    "cmd-2",
		ExitCode: 42,
		Error:    "something went wrong",
	})

	sess, _ := e.Commands.Get("cmd-2")
	_, done, exitCode, errMsg := sess.Snapshot()
	if !done {
		t.Error("session should be done")
	}
	if exitCode != 42 {
		t.Errorf("exit code = %d; want 42", exitCode)
	}
	if errMsg != "something went wrong" {
		t.Errorf("errMsg = %q; want 'something went wrong'", errMsg)
	}
}

func TestHandleWSMessage_CommandCompleted_SuccessfulCommand(t *testing.T) {
	e := newWSTestEnv(t)
	e.Commands.Create("cmd-ok", "c5", "true")
	saveAgent(t, e, "c5")

	sendWS(t, e, "c5", common.WSMsgCommandCompleted, common.WSCommandStatus{
		CmdID:    "cmd-ok",
		ExitCode: 0,
		Error:    "",
	})

	sess, _ := e.Commands.Get("cmd-ok")
	_, done, exitCode, errMsg := sess.Snapshot()
	if !done || exitCode != 0 || errMsg != "" {
		t.Errorf("unexpected state: done=%v exitCode=%d errMsg=%q", done, exitCode, errMsg)
	}
}

// ── Invalid / unknown messages ────────────────────────────────────────────────

func TestHandleWSMessage_InvalidJSON_DoesNotMutateState(t *testing.T) {
	e := newWSTestEnv(t)
	saveAgent(t, e, "c2")
	saveDeployment(t, e, "dep-2", "c2", common.DeploymentStatusRunning)

	before, _ := e.Store.GetDeployment("dep-2")
	e.handleWSMessage("c2", []byte("not-json"))
	e.handleWSMessage("c2", []byte(`{"type":"step_start","data":{`))
	after, _ := e.Store.GetDeployment("dep-2")

	if before.Status != after.Status || before.ResumeStepIndex != after.ResumeStepIndex {
		t.Fatal("invalid messages must not mutate deployment state")
	}
}

func TestHandleWSMessage_UnknownType_DoesNotMutateState(t *testing.T) {
	e := newWSTestEnv(t)
	saveAgent(t, e, "c-unknown")
	saveDeployment(t, e, "dep-unknown", "c-unknown", common.DeploymentStatusRunning)

	before, _ := e.Store.GetDeployment("dep-unknown")
	sendWS(t, e, "c-unknown", "totally_unknown_type_xyz", nil)
	after, _ := e.Store.GetDeployment("dep-unknown")

	if before.Status != after.Status || before.ResumeStepIndex != after.ResumeStepIndex {
		t.Error("unknown message type must not mutate deployment state")
	}
}

func TestHandleWSMessage_Pong_IsNoOp(t *testing.T) {
	e := newWSTestEnv(t)
	// No state setup needed — just verify it doesn't panic.
	sendWS(t, e, "any-agent", common.WSMsgPong, nil)
}
