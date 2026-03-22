package handlers

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/coder/websocket"
	"github.com/marko-stanojevic/kompakt/internal/common"
)

// HandleAgentWS upgrades GET /api/v1/ws to a WebSocket connection.
// Authentication uses the JWT bearer token passed as ?token=<jwt> query param
// (WebSocket agents cannot set arbitrary headers during the handshake in all
// environments, so the query param fallback is supported here).
func (e *Handler) HandleAgentWS(w http.ResponseWriter, r *http.Request) {
	agentID, err := e.agentIDFromToken(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err.Error())
		return
	}
	agent, ok := e.Store.GetAgent(agentID)
	if !ok {
		writeError(w, http.StatusNotFound, "agent not found")
		return
	}

	ws, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		InsecureSkipVerify: true, // origin is verified via JWT token, not HTTP origin header
	})
	if err != nil {
		return
	}

	conn := newAgentConn(agentID, ws)
	e.Hub.register(conn)

	agent.Status = common.AgentStatusConnected
	agent.IPAddress = requestIP(r)
	agent.LastActivityAt = time.Now()
	_ = e.Store.SaveAgent(agent)
	slog.Info("agent connected", "agent_id", agentID, "hostname", agent.Hostname, "ip", agent.IPAddress)

	defer func() {
		e.Hub.unregister(agentID)
		if a, ok := e.Store.GetAgent(agentID); ok {
			if a.Status == common.AgentStatusConnected {
				a.Status = common.AgentStatusOffline
			}
			_ = e.Store.SaveAgent(a)
		}
		ws.CloseNow()
		slog.Info("agent disconnected", "agent_id", agentID)
	}()

	// Push playbook immediately if one is assigned.
	if e.Service != nil {
		e.Service.PushPlaybookIfAssigned(agentID, false)
	}

	// Read loop. coder/websocket responds to pings automatically; keepalive
	// pings are sent from the write pump every 30 s.
	for {
		_, data, err := ws.Read(r.Context())
		if err != nil {
			return
		}
		e.handleWSMessage(agentID, data)
	}
}

// handleWSMessage dispatches an inbound WebSocket message from an agent.
func (e *Handler) handleWSMessage(agentID string, data []byte) {
	var envelope struct {
		Type common.WSMessageType `json:"type"`
		Data json.RawMessage      `json:"data"`
	}
	if err := json.Unmarshal(data, &envelope); err != nil {
		slog.Error("failed to parse websocket envelope", "agent_id", agentID, "err", err)
		return
	}

	switch envelope.Type {
	case common.WSMsgLog:
		var d common.WSLogData
		if err := json.Unmarshal(envelope.Data, &d); err != nil {
			slog.Error("failed to parse log message", "agent_id", agentID, "err", err)
			return
		}
		entry := &common.LogEntry{
			DeploymentID: d.DeploymentID,
			JobName:      d.JobName,
			StepIndex:    d.StepIndex,
			Level:        d.Level,
			Message:      d.Message,
			Timestamp:    time.Now(),
		}
		_ = e.Store.AppendLogs([]*common.LogEntry{entry})
		switch d.Level {
		case common.LogLevelError:
			slog.Error(d.Message, "agent_id", agentID, "deployment_id", d.DeploymentID, "job", d.JobName)
		case common.LogLevelWarn:
			slog.Warn(d.Message, "agent_id", agentID, "deployment_id", d.DeploymentID, "job", d.JobName)
		default:
			slog.Debug(d.Message, "agent_id", agentID, "deployment_id", d.DeploymentID, "job", d.JobName)
		}

	case common.WSMsgStepStart:
		var d common.WSStepData
		if err := json.Unmarshal(envelope.Data, &d); err != nil {
			slog.Error("failed to parse step-start message", "agent_id", agentID, "err", err)
			return
		}
		stepName := d.StepName
		if stepName == "" {
			stepName = fmt.Sprintf("step-%d", d.StepIndex)
		}
		if e.Service != nil {
			e.Service.AppendDeploymentLog(d.DeploymentID, d.JobName, d.StepIndex, common.LogLevelInfo,
				fmt.Sprintf("[%s / %s] starting", d.JobName, stepName))
		}
		slog.Info("step starting", "agent_id", agentID, "deployment_id", d.DeploymentID, "job", d.JobName, "step", stepName)
		e.updateDeploy(d.DeploymentID, func(dep *common.DeploymentState) {
			dep.Status = common.DeploymentStatusRunning
			dep.ResumeStepIndex = d.StepIndex
		})

	case common.WSMsgStepComplete:
		var d common.WSStepData
		if err := json.Unmarshal(envelope.Data, &d); err != nil {
			slog.Error("failed to parse step-complete message", "agent_id", agentID, "err", err)
			return
		}
		stepName := d.StepName
		if stepName == "" {
			stepName = fmt.Sprintf("step-%d", d.StepIndex)
		}
		if e.Service != nil {
			e.Service.AppendDeploymentLog(d.DeploymentID, d.JobName, d.StepIndex, common.LogLevelInfo,
				fmt.Sprintf("[%s / %s] completed", d.JobName, stepName))
		}
		slog.Info("step completed", "agent_id", agentID, "deployment_id", d.DeploymentID, "job", d.JobName, "step", stepName)
		e.updateDeploy(d.DeploymentID, func(dep *common.DeploymentState) {
			dep.ResumeStepIndex = d.StepIndex + 1
		})

	case common.WSMsgStepFailed:
		var d common.WSStepData
		if err := json.Unmarshal(envelope.Data, &d); err != nil {
			slog.Error("failed to parse step-failed message", "agent_id", agentID, "err", err)
			return
		}
		stepName := d.StepName
		if stepName == "" {
			stepName = fmt.Sprintf("step-%d", d.StepIndex)
		}
		slog.Error("step failed", "agent_id", agentID, "deployment_id", d.DeploymentID, "job", d.JobName, "step", stepName, "error", d.Error)
		e.updateDeploy(d.DeploymentID, func(dep *common.DeploymentState) {
			dep.Status = common.DeploymentStatusFailed
			dep.ErrorDetail = d.Error
		})

	case common.WSMsgReboot:
		var d common.WSRebootData
		if err := json.Unmarshal(envelope.Data, &d); err != nil {
			slog.Error("failed to parse reboot message", "agent_id", agentID, "err", err)
			return
		}
		slog.Info("agent rebooting", "agent_id", agentID, "deployment_id", d.DeploymentID, "resume_step", d.ResumeStepIndex)
		e.updateDeploy(d.DeploymentID, func(dep *common.DeploymentState) {
			dep.Status = common.DeploymentStatusRebooting
			dep.ResumeStepIndex = d.ResumeStepIndex
		})

	case common.WSMsgDeployDone:
		var d common.WSStepData
		if err := json.Unmarshal(envelope.Data, &d); err != nil {
			slog.Error("failed to parse deploy-done message", "agent_id", agentID, "err", err)
			return
		}
		playbookName := ""
		if e.Service != nil {
			playbookName = e.Service.ResolvePlaybookNameByDeployment(d.DeploymentID)
			e.Service.AppendDeploymentLog(d.DeploymentID, "", 0, common.LogLevelInfo,
				fmt.Sprintf("playbook %q completed successfully", playbookName))
		}
		slog.Info("playbook completed", "agent_id", agentID, "deployment_id", d.DeploymentID, "playbook", playbookName)
		now := time.Now()
		e.updateDeploy(d.DeploymentID, func(dep *common.DeploymentState) {
			dep.Status = common.DeploymentStatusCompleted
			dep.FinishedAt = &now
		})
		if a, ok := e.Store.GetAgent(agentID); ok {
			a.Status = common.AgentStatusCompleted
			_ = e.Store.SaveAgent(a)
		}

	case common.WSMsgDeployFailed:
		var d common.WSStepData
		if err := json.Unmarshal(envelope.Data, &d); err != nil {
			slog.Error("failed to parse deploy-failed message", "agent_id", agentID, "err", err)
			return
		}
		playbookName := "playbook"
		if e.Service != nil {
			playbookName = e.Service.ResolvePlaybookNameByDeployment(d.DeploymentID)
		}
		msg := fmt.Sprintf("playbook %q failed", playbookName)
		if d.Error != "" {
			msg = fmt.Sprintf("%s: %s", msg, d.Error)
		}
		if e.Service != nil {
			e.Service.AppendDeploymentLog(d.DeploymentID, d.JobName, d.StepIndex, common.LogLevelError, msg)
		}
		slog.Error("playbook failed", "agent_id", agentID, "deployment_id", d.DeploymentID, "playbook", playbookName, "error", d.Error)
		now := time.Now()
		e.updateDeploy(d.DeploymentID, func(dep *common.DeploymentState) {
			dep.Status = common.DeploymentStatusFailed
			dep.ErrorDetail = d.Error
			dep.FinishedAt = &now
		})
		if a, ok := e.Store.GetAgent(agentID); ok {
			a.Status = common.AgentStatusFailed
			_ = e.Store.SaveAgent(a)
		}

	case common.WSMsgCommandStream:
		var d common.WSCommandChunk
		if err := json.Unmarshal(envelope.Data, &d); err != nil {
			slog.Error("failed to parse command_stream", "agent_id", agentID, "err", err)
			return
		}
		if e.Commands != nil {
			e.Commands.AppendOutput(d.CmdID, d.Output)
		}

	case common.WSMsgCommandCompleted:
		var d common.WSCommandStatus
		if err := json.Unmarshal(envelope.Data, &d); err != nil {
			slog.Error("failed to parse command_completed", "agent_id", agentID, "err", err)
			return
		}
		slog.Info("remote command done", "agent_id", agentID, "cmd_id", d.CmdID, "exit_code", d.ExitCode)
		if e.Commands != nil {
			e.Commands.SetDone(d.CmdID, d.ExitCode, d.Error)
		}

	case common.WSMsgPong:
		// keepalive — handled by SetPongHandler
	}
}

func (e *Handler) updateDeploy(deploymentID string, fn func(*common.DeploymentState)) {
	dep, ok := e.Store.GetDeployment(deploymentID)
	if !ok {
		return
	}
	fn(dep)
	dep.UpdatedAt = time.Now()
	_ = e.Store.SaveDeployment(dep)
}

