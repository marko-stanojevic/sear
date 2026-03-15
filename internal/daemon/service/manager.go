package service

import (
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/marko-stanojevic/sear/internal/common"
	"github.com/marko-stanojevic/sear/internal/daemon/ports"
	"github.com/marko-stanojevic/sear/internal/daemon/store"
)

// Manager hosts daemon application-level orchestration logic.
type Manager struct {
	Store     ports.StorePort
	Hub       ports.HubPort
	ServerURL string
}

func (m *Manager) StatusSnapshot() ([]*common.Client, []*common.DeploymentState) {
	return m.Store.ListClients(), m.Store.ListDeployments()
}

func (m *Manager) AssignPlaybookToClient(playbookID, clientID string) error {
	client, ok := m.Store.GetClient(clientID)
	if !ok {
		return fmt.Errorf("client not found")
	}
	if _, ok := m.Store.GetPlaybook(playbookID); !ok {
		return fmt.Errorf("playbook not found")
	}
	client.PlaybookID = playbookID
	if err := m.Store.SaveClient(client); err != nil {
		return fmt.Errorf("failed to save client: %w", err)
	}
	if m.Hub.IsConnected(clientID) {
		m.PushPlaybookIfAssigned(clientID)
	}
	return nil
}

// PushPlaybookIfAssigned sends an assigned playbook to a connected client.
func (m *Manager) PushPlaybookIfAssigned(clientID string) {
	client, ok := m.Store.GetClient(clientID)
	if !ok || client.PlaybookID == "" {
		return
	}

	dep, hasDep := m.Store.GetActiveDeploymentForClient(clientID)
	pb, ok := m.Store.GetPlaybook(client.PlaybookID)
	if !ok {
		return
	}

	var depID string
	resumeStep := 0

	if hasDep &&
		(dep.Status == common.DeploymentStatusPending ||
			dep.Status == common.DeploymentStatusRunning ||
			dep.Status == common.DeploymentStatusRebooting) {
		depID = dep.ID
		resumeStep = dep.ResumeStepIndex
		dep.Status = common.DeploymentStatusRunning
		dep.UpdatedAt = time.Now()
		_ = m.Store.SaveDeployment(dep)
	} else if !hasDep ||
		dep.Status == common.DeploymentStatusDone ||
		dep.Status == common.DeploymentStatusFailed {
		depID = uuid.New().String()
		newDep := &common.DeploymentState{
			ID:              depID,
			ClientID:        clientID,
			PlaybookID:      client.PlaybookID,
			Status:          common.DeploymentStatusRunning,
			ResumeStepIndex: 0,
			StartedAt:       time.Now(),
			UpdatedAt:       time.Now(),
		}
		_ = m.Store.SaveDeployment(newDep)
	} else {
		return
	}

	client.Status = common.ClientStatusDeploying
	_ = m.Store.SaveClient(client)

	pbName := pb.Name
	if pb.Playbook != nil && pb.Playbook.Name != "" {
		pbName = pb.Playbook.Name
	}
	m.AppendDeploymentLog(depID, "", 0, common.LogLevelInfo,
		fmt.Sprintf("starting playbook %q (deployment %s, resume step %d)", pbName, depID, resumeStep))

	m.Hub.Send(clientID, common.WSMessage{
		Type:      common.WSMsgPlaybook,
		Timestamp: time.Now(),
		Data: common.WSPlaybookData{
			DeploymentID:     depID,
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
	pbName := "playbook"
	if dep, ok := m.Store.GetDeployment(deploymentID); ok {
		if pb, ok := m.Store.GetPlaybook(dep.PlaybookID); ok {
			if pb.Playbook != nil && pb.Playbook.Name != "" {
				pbName = pb.Playbook.Name
			} else if pb.Name != "" {
				pbName = pb.Name
			}
		}
	}
	return pbName
}

var _ ports.StorePort = (*store.Store)(nil)
