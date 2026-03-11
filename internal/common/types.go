// Package common defines shared types used by both the daemon and client.
package common

import "time"

// ---- Playbook / workflow types -----------------------------------------------

// Playbook represents a GitHub-Actions-style deployment workflow.
type Playbook struct {
	Name string         `yaml:"name" json:"name"`
	Jobs map[string]Job `yaml:"jobs" json:"jobs"`
}

// Job is a collection of sequential steps.
type Job struct {
	Name  string `yaml:"name"  json:"name"`
	Steps []Step `yaml:"steps" json:"steps"`
}

// Step is a single unit of work inside a job.
// Exactly one of Run or Uses must be set.
type Step struct {
	// Human-readable label.
	Name string `yaml:"name" json:"name"`

	// Shell command to execute (bash / pwsh).
	Run   string `yaml:"run,omitempty"   json:"run,omitempty"`
	Shell string `yaml:"shell,omitempty" json:"shell,omitempty"` // "bash" (default) | "pwsh"

	// Built-in action name: "download-artifact", "upload-artifact",
	// "upload-logs", "reboot".
	Uses string            `yaml:"uses,omitempty" json:"uses,omitempty"`
	With map[string]string `yaml:"with,omitempty" json:"with,omitempty"`

	// Env overrides injected for this step.
	Env map[string]string `yaml:"env,omitempty" json:"env,omitempty"`

	// ContinueOnError allows the job to proceed even if this step fails.
	ContinueOnError bool `yaml:"continue-on-error,omitempty" json:"continue_on_error,omitempty"`
}

// ---- Client / registration types -------------------------------------------

// PlatformType identifies the hardware / cloud platform used during
// client registration.
type PlatformType string

const (
	PlatformBaremetal PlatformType = "baremetal"
	PlatformAzure     PlatformType = "azure"
	PlatformAWS       PlatformType = "aws"
	PlatformGCP       PlatformType = "gcp"
	PlatformCustom    PlatformType = "custom"
)

// RegistrationRequest is sent by a client to /api/v1/register.
type RegistrationRequest struct {
	// Platform of the registering client.
	Platform PlatformType `json:"platform"`

	// PlatformID is the hardware/cloud-specific unique identifier
	// (e.g. serial number, instance-id).
	PlatformID string `json:"platform_id"`

	// Hostname of the client machine.
	Hostname string `json:"hostname"`

	// RegistrationSecret is a pre-shared secret that must match an entry
	// in the server's secrets configuration.
	RegistrationSecret string `json:"registration_secret"`

	// Metadata contains additional platform-specific key/value pairs.
	Metadata map[string]string `json:"metadata,omitempty"`
}

// RegistrationResponse is returned to a successfully registered client.
type RegistrationResponse struct {
	ClientID string `json:"client_id"`
	Token    string `json:"token"`
}

// ClientStatus represents the lifecycle state of a registered client.
type ClientStatus string

const (
	ClientStatusRegistered ClientStatus = "registered"
	ClientStatusConnected  ClientStatus = "connected"
	ClientStatusDeploying  ClientStatus = "deploying"
	ClientStatusDone       ClientStatus = "done"
	ClientStatusFailed     ClientStatus = "failed"
	ClientStatusOffline    ClientStatus = "offline"
)

// Client holds server-side information about a registered client machine.
type Client struct {
	ID           string            `json:"id"`
	Hostname     string            `json:"hostname"`
	Platform     PlatformType      `json:"platform"`
	PlatformID   string            `json:"platform_id"`
	Metadata     map[string]string `json:"metadata,omitempty"`
	Status       ClientStatus      `json:"status"`
	PlaybookID   string            `json:"playbook_id,omitempty"`
	RegisteredAt time.Time         `json:"registered_at"`
	LastSeenAt   time.Time         `json:"last_seen_at"`
}

// ---- Deployment / state types -----------------------------------------------

// DeploymentStatus is the lifecycle state of a deployment.
type DeploymentStatus string

const (
	DeploymentStatusPending   DeploymentStatus = "pending"
	DeploymentStatusRunning   DeploymentStatus = "running"
	DeploymentStatusRebooting DeploymentStatus = "rebooting"
	DeploymentStatusDone      DeploymentStatus = "done"
	DeploymentStatusFailed    DeploymentStatus = "failed"
)

// DeploymentState is the server-persisted progress for a single deployment.
type DeploymentState struct {
	ID         string           `json:"id"`
	ClientID   string           `json:"client_id"`
	PlaybookID string           `json:"playbook_id"`
	Status     DeploymentStatus `json:"status"`

	// CurrentJob and CurrentStep are the indices into the playbook that the
	// client should execute next (used to resume after a reboot).
	CurrentJobName  string `json:"current_job_name"`
	CurrentStepIndex int   `json:"current_step_index"`

	StartedAt   time.Time  `json:"started_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
	FinishedAt  *time.Time `json:"finished_at,omitempty"`
	ErrorDetail string     `json:"error_detail,omitempty"`
}

// StateUpdateRequest is sent by the client to report its current progress.
type StateUpdateRequest struct {
	DeploymentID    string           `json:"deployment_id"`
	Status          DeploymentStatus `json:"status"`
	CurrentJobName  string           `json:"current_job_name"`
	CurrentStepIndex int             `json:"current_step_index"`
	ErrorDetail     string           `json:"error_detail,omitempty"`
}

// ConnectResponse is returned by /api/v1/connect to give the client its
// next instructions.
type ConnectResponse struct {
	// Action tells the client what to do.
	// "deploy"  – execute the attached playbook from ResumeJobName/ResumeStepIndex.
	// "wait"    – nothing to do; poll again later.
	// "reboot"  – reboot the machine now.
	Action string `json:"action"`

	DeploymentID     string            `json:"deployment_id,omitempty"`
	PlaybookID       string            `json:"playbook_id,omitempty"`
	Playbook         *Playbook         `json:"playbook,omitempty"`
	ResumeJobName    string            `json:"resume_job_name,omitempty"`
	ResumeStepIndex  int               `json:"resume_step_index,omitempty"`
	Secrets          map[string]string `json:"secrets,omitempty"`
	ArtifactsBaseURL string            `json:"artifacts_base_url,omitempty"`
}

// ---- Log types ---------------------------------------------------------------

// LogLevel is the severity of a log entry.
type LogLevel string

const (
	LogLevelDebug LogLevel = "debug"
	LogLevelInfo  LogLevel = "info"
	LogLevelWarn  LogLevel = "warn"
	LogLevelError LogLevel = "error"
)

// LogEntry represents a single log line pushed from a client.
type LogEntry struct {
	DeploymentID string    `json:"deployment_id"`
	JobName      string    `json:"job_name"`
	StepIndex    int       `json:"step_index"`
	Level        LogLevel  `json:"level"`
	Message      string    `json:"message"`
	Timestamp    time.Time `json:"timestamp"`
}

// LogBatch is a list of log entries sent in a single request.
type LogBatch struct {
	Entries []LogEntry `json:"entries"`
}

// ---- Artifact types ----------------------------------------------------------

// Artifact is metadata for a file stored on the server.
type Artifact struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Filename    string    `json:"filename"`
	Size        int64     `json:"size"`
	ContentType string    `json:"content_type"`
	UploadedAt  time.Time `json:"uploaded_at"`
}
