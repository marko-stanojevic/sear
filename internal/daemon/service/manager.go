package service

import (
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/marko-stanojevic/sear/internal/common"
	"github.com/marko-stanojevic/sear/internal/daemon/ports"
)

// Sentinel errors returned by service operations.
var (
	// ErrClientNotFound is returned when a client ID does not match any registered client.
	ErrClientNotFound = errors.New("client not found")
	// ErrPlaybookNotFound is returned when a playbook ID does not match any stored playbook.
	ErrPlaybookNotFound = errors.New("playbook not found")
)

// Manager hosts daemon application-level orchestration logic.
type Manager struct {
	Store     ports.Store
	Hub       ports.Hub
	ServerURL string
}

func (m *Manager) StatusSnapshot() ([]*common.Client, []*common.DeploymentState) {
	return m.Store.ListClients(), m.Store.ListDeployments()
}

func (m *Manager) AssignPlaybookToClient(playbookID, clientID string) error {
	client, ok := m.Store.GetClient(clientID)
	if !ok {
		return fmt.Errorf("assign playbook to client: %w", ErrClientNotFound)
	}
	if _, ok := m.Store.GetPlaybook(playbookID); !ok {
		return fmt.Errorf("assign playbook to client: %w", ErrPlaybookNotFound)
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

	var deploymentID string
	resumeStep := 0

	if hasDep &&
		(dep.Status == common.DeploymentStatusPending ||
			dep.Status == common.DeploymentStatusRunning ||
			dep.Status == common.DeploymentStatusRebooting) {
		deploymentID = dep.ID
		resumeStep = dep.ResumeStepIndex
		dep.Status = common.DeploymentStatusRunning
		dep.UpdatedAt = time.Now()
		if err := m.Store.SaveDeployment(dep); err != nil {
			fmt.Printf("failed to save deployment %s for client %s: %v\n", dep.ID, clientID, err)
			return
		}
	} else if !hasDep ||
		dep.Status == common.DeploymentStatusDone ||
		dep.Status == common.DeploymentStatusFailed {
		deploymentID = uuid.New().String()
		newDep := &common.DeploymentState{
			ID:              deploymentID,
			ClientID:        clientID,
			PlaybookID:      client.PlaybookID,
			Status:          common.DeploymentStatusRunning,
			ResumeStepIndex: 0,
			StartedAt:       time.Now(),
			UpdatedAt:       time.Now(),
		}
		if err := m.Store.SaveDeployment(newDep); err != nil {
			fmt.Printf("failed to save new deployment %s for client %s: %v\n", deploymentID, clientID, err)
			return
		}
	} else {
		return
	}

	client.Status = common.ClientStatusDeploying
	if err := m.Store.SaveClient(client); err != nil {
		fmt.Printf("failed to update client %s status to deploying: %v\n", clientID, err)
		return
	}

	playbookName := pb.Name
	if pb.Playbook != nil && pb.Playbook.Name != "" {
		playbookName = pb.Playbook.Name
	}
	m.AppendDeploymentLog(deploymentID, "", 0, common.LogLevelInfo,
		fmt.Sprintf("starting playbook %q (deployment %s, resume step %d)", playbookName, deploymentID, resumeStep))

	m.Hub.Send(clientID, common.WSMessage{
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


