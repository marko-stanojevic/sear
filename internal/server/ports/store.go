package ports

import (
	"github.com/marko-stanojevic/kompakt/internal/common"
	"github.com/marko-stanojevic/kompakt/internal/server/store"
)

// Store defines persistence capabilities used by handler/service layers.
type Store interface {
	SaveAgent(a *common.Agent) error
	GetAgent(id string) (*common.Agent, bool)
	ListAgents() []*common.Agent
	DeleteAgent(id string) error

	SaveDeployment(d *common.DeploymentState) error
	GetDeployment(id string) (*common.DeploymentState, bool)
	GetActiveDeploymentForAgent(agentID string) (*common.DeploymentState, bool)
	ListDeployments() []*common.DeploymentState
	DeleteDeployment(id string) error

	SavePlaybook(p *store.PlaybookRecord) error
	GetPlaybook(id string) (*store.PlaybookRecord, bool)
	ListPlaybooks() []*store.PlaybookRecord
	DeletePlaybook(id string) error

	SaveArtifact(a *common.Artifact) error
	GetArtifact(id string) (*common.Artifact, bool)
	GetArtifactByName(name string) (*common.Artifact, bool)
	ListArtifacts() []*common.Artifact
	DeleteArtifact(id string) error

	SetSecret(name, value string) error
	GetSecret(name string) (string, bool)
	ListSecretNames() []string
	DeleteSecret(name string) error
	MergeSecrets(m map[string]string) error
	AllSecrets() map[string]string

	AppendLogs(entries []*common.LogEntry) error
	GetLogsForDeployment(deploymentID string) []*common.LogEntry
	GetLogsForAgent(agentID string) []*common.LogEntry
}

// Hub defines websocket coordination capabilities used by services.
type Hub interface {
	IsConnected(agentID string) bool
	Send(agentID string, msg common.WSMessage) bool
}
