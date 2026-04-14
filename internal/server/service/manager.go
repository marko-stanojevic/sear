package service

import (
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/marko-stanojevic/kompakt/internal/common"
	"github.com/marko-stanojevic/kompakt/internal/iso"
	"github.com/marko-stanojevic/kompakt/internal/server/ports"
)

// Sentinel errors returned by service operations.
var (
	// ErrAgentNotFound is returned when an agent ID does not match any registered agent.
	ErrAgentNotFound = errors.New("agent not found")
	// ErrPlaybookNotFound is returned when a playbook ID does not match any stored playbook.
	ErrPlaybookNotFound = errors.New("playbook not found")
)

// Manager hosts server application-level orchestration logic.
type Manager struct {
	Store        ports.Store
	Hub          ports.Hub
	ServerURL    string
	ISOBuilds    *iso.BuildStore
	ISOOutputDir string
}

func (m *Manager) StatusSnapshot() ([]*common.Agent, []*common.DeploymentState) {
	return m.Store.ListAgents(), m.Store.ListDeployments()
}

func (m *Manager) AssignPlaybookToAgent(playbookID, agentID string) error {
	agent, ok := m.Store.GetAgent(agentID)
	if !ok {
		return fmt.Errorf("assign playbook to agent: %w", ErrAgentNotFound)
	}
	if _, ok := m.Store.GetPlaybook(playbookID); !ok {
		return fmt.Errorf("assign playbook to agent: %w", ErrPlaybookNotFound)
	}
	agent.PlaybookID = playbookID
	if err := m.Store.SaveAgent(agent); err != nil {
		return fmt.Errorf("failed to save agent: %w", err)
	}
	if m.Hub.IsConnected(agentID) {
		m.PushPlaybookIfAssigned(agentID, true)
	}
	return nil
}

// PushPlaybookIfAssigned sends an assigned playbook to a connected agent.
// If force is false, it skips if the latest deployment for this playbook is already Done.
func (m *Manager) PushPlaybookIfAssigned(agentID string, force bool) {
	agent, ok := m.Store.GetAgent(agentID)
	if !ok || agent.PlaybookID == "" {
		return
	}

	dep, hasDep := m.Store.GetActiveDeploymentForAgent(agentID)
	pb, ok := m.Store.GetPlaybook(agent.PlaybookID)
	if !ok {
		return
	}

	var deploymentID string
	resumeStep := 0

	// Logic check:
	// 1. If latest deployment is for a DIFFERENT playbook, always start new.
	// 2. If latest deployment is for the SAME playbook:
	//    - If Running/Rebooting: resume (ignore force).
	//    - If Done/Failed: start new if force, otherwise skip.

	isSamePlaybook := hasDep && dep.PlaybookID == agent.PlaybookID

	if isSamePlaybook && (dep.Status == common.DeploymentStatusPending ||
		dep.Status == common.DeploymentStatusRunning ||
		dep.Status == common.DeploymentStatusRebooting) {
		// Resume existing
		deploymentID = dep.ID
		resumeStep = dep.ResumeStepIndex
		dep.Status = common.DeploymentStatusRunning
		dep.UpdatedAt = time.Now()
		if err := m.Store.SaveDeployment(dep); err != nil {
			slog.Error("failed to save deployment", "deployment_id", dep.ID, "agent_id", agentID, "err", err)
			return
		}
	} else if !isSamePlaybook || force {
		// Start new deployment
		deploymentID = common.NewID()
		newDep := &common.DeploymentState{
			ID:           deploymentID,
			AgentID:      agentID,
			Hostname:     agent.Hostname,
			PlaybookID:   agent.PlaybookID,
			PlaybookName: pb.Name,
			Status:          common.DeploymentStatusRunning,
			ResumeStepIndex: 0,
			StartedAt:       time.Now(),
			UpdatedAt:       time.Now(),
		}
		if pb.Playbook != nil && pb.Playbook.Name != "" {
			newDep.PlaybookName = pb.Playbook.Name
		}
		if err := m.Store.SaveDeployment(newDep); err != nil {
			slog.Error("failed to save new deployment", "deployment_id", deploymentID, "agent_id", agentID, "err", err)
			return
		}
	} else {
		// !force and (Done/Failed) and same playbook -> Skip
		m.AppendDeploymentLog(dep.ID, "", 0, common.LogLevelInfo,
			fmt.Sprintf("skipping playbook %q: already in status %q and force=false", pb.Name, dep.Status))
		return
	}

	agent.Status = common.AgentStatusDeploying
	if err := m.Store.SaveAgent(agent); err != nil {
		slog.Error("failed to update agent status to deploying", "agent_id", agentID, "err", err)
		return
	}

	playbookName := pb.Name
	if pb.Playbook != nil && pb.Playbook.Name != "" {
		playbookName = pb.Playbook.Name
	}
	m.AppendDeploymentLog(deploymentID, "", 0, common.LogLevelInfo,
		fmt.Sprintf("starting playbook %q (deployment %s, resume step %d)", playbookName, deploymentID, resumeStep))

	m.Hub.Send(agentID, common.WSMessage{
		Type:      common.WSMsgPlaybook,
		Timestamp: time.Now(),
		Data: common.WSPlaybookData{
			DeploymentID:     deploymentID,
			Playbook:         pb.Playbook,
			ResumeStepIndex:  resumeStep,
			Secrets:          m.Store.AllSecrets(),
			ArtifactsBaseURL: m.ServerURL + "/artifacts",
		},
	})
}

func (m *Manager) AppendDeploymentLog(deploymentID, jobName string, stepIndex int, level common.LogLevel, message string) {
	if deploymentID == "" || message == "" {
		return
	}
	_ = m.Store.AppendLogs([]*common.LogEntry{{
		DeploymentID: deploymentID,
		JobName:      jobName,
		StepIndex:    stepIndex,
		Level:        level,
		Message:      message,
		Timestamp:    time.Now(),
	}})
}

func (m *Manager) ResolvePlaybookNameByDeployment(deploymentID string) string {
	playbookName := "playbook"
	if dep, ok := m.Store.GetDeployment(deploymentID); ok {
		if pb, ok := m.Store.GetPlaybook(dep.PlaybookID); ok {
			if pb.Playbook != nil && pb.Playbook.Name != "" {
				playbookName = pb.Playbook.Name
			} else if pb.Name != "" {
				playbookName = pb.Name
			}
		}
	}
	return playbookName
}
