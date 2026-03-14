// Package common defines shared types used by both the daemon and client.
package common

import (
	"strings"
	"time"
)

// ── Playbook / workflow types ─────────────────────────────────────────────────

// Playbook is the top-level deployment descriptor.
// Jobs are a slice (not a map) so execution order is always deterministic
// and matches the order they are written in the YAML file.
type Playbook struct {
	Name string            `yaml:"name" json:"name"`
	Env  map[string]string `yaml:"env,omitempty" json:"env,omitempty"` // playbook-level vars
	Jobs []Job             `yaml:"jobs" json:"jobs"`
}

// Job groups a set of related steps.
type Job struct {
	Name  string `yaml:"name" json:"name"`
	Steps []Step `yaml:"steps" json:"steps"`
}

// Step is a single unit of work inside a job.
// Exactly one of Run or Uses must be set.
type Step struct {
	// Human-readable label (required).
	Name string `yaml:"name" json:"name"`

	// Run contains a shell script.
	Run   string `yaml:"run,omitempty"   json:"run,omitempty"`
	Shell string `yaml:"shell,omitempty" json:"shell,omitempty"` // bash (default) | sh | pwsh | cmd | python

	// Uses specifies a built-in action:
	//   reboot | download-artifact | upload-artifact | upload-logs
	Uses string            `yaml:"uses,omitempty" json:"uses,omitempty"`
	With map[string]string `yaml:"with,omitempty" json:"with,omitempty"`

	// Env overlays environment variables for this step only.
	// Values may contain ${{ secrets.NAME }} references.
	Env map[string]string `yaml:"env,omitempty" json:"env,omitempty"`

	// ContinueOnError allows the job to proceed even if this step fails.
	ContinueOnError bool `yaml:"continue-on-error,omitempty" json:"continue_on_error,omitempty"`

	// TimeoutMinutes kills the step process after this many minutes.
	TimeoutMinutes int `yaml:"timeout-minutes,omitempty" json:"timeout_minutes,omitempty"`
}

// ── FlatStep — ordered execution view ────────────────────────────────────────

// FlatStep is a Step annotated with its position across the whole playbook.
// GlobalIndex is what the daemon persists as the resume point after a reboot.
type FlatStep struct {
	Step
	JobName     string
	JobIndex    int
	StepIndex   int // position within the job
	GlobalIndex int // monotonically increasing across all jobs and steps
}

// FlattenPlaybook returns all steps in execution order with global indices.
func FlattenPlaybook(pb *Playbook) []FlatStep {
	var out []FlatStep
	global := 0
	for ji, job := range pb.Jobs {
		for si, step := range job.Steps {
			out = append(out, FlatStep{
				Step:        step,
				JobName:     job.Name,
				JobIndex:    ji,
				StepIndex:   si,
				GlobalIndex: global,
			})
			global++
		}
	}
	return out
}

// ── Secret resolution ─────────────────────────────────────────────────────────

// ResolveSecrets replaces every ${{ secrets.NAME }} occurrence in s with the
// corresponding value from secrets. Unknown references are replaced with "".
func ResolveSecrets(s string, secrets map[string]string) string {
	const prefix = "${{ secrets."
	const suffix = " }}"
	result := s
	for {
		start := strings.Index(result, prefix)
		if start == -1 {
			break
		}
		end := strings.Index(result[start:], suffix)
		if end == -1 {
			break
		}
		end += start
		name := strings.TrimSpace(result[start+len(prefix) : end])
		result = result[:start] + secrets[name] + result[end+len(suffix):]
	}
	return result
}

// ResolveEnvSecrets applies ResolveSecrets to every value in an env map and
// returns a new map.
func ResolveEnvSecrets(env map[string]string, secrets map[string]string) map[string]string {
	if len(env) == 0 {
		return env
	}
	out := make(map[string]string, len(env))
	for k, v := range env {
		out[k] = ResolveSecrets(v, secrets)
	}
	return out
}

// ── Client / registration types ───────────────────────────────────────────────

// PlatformType identifies the operating system platform.
type PlatformType string

const (
	PlatformLinux   PlatformType = "linux"
	PlatformMac     PlatformType = "mac"
	PlatformWindows PlatformType = "windows"
)

// RegistrationRequest is sent by a client to POST /api/v1/register.
type RegistrationRequest struct {
	Platform           PlatformType      `json:"platform"`
	Hostname           string            `json:"hostname"`
	Model              string            `json:"model,omitempty"`
	Vendor             string            `json:"vendor,omitempty"`
	RegistrationSecret string            `json:"registration_secret"`
	Metadata           map[string]string `json:"metadata,omitempty"`
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
	ID            string            `json:"id"`
	Hostname      string            `json:"hostname"`
	Platform      PlatformType      `json:"platform"`
	OS            string            `json:"os,omitempty"`
	Model         string            `json:"model,omitempty"`
	Vendor        string            `json:"vendor,omitempty"`
	IPAddress     string            `json:"ip_address,omitempty"`
	Metadata      map[string]string `json:"metadata,omitempty"`
	Status        ClientStatus      `json:"status"`
	PlaybookID    string            `json:"playbook_id,omitempty"`
	RegisteredAt  time.Time         `json:"registered_at"`
	LastSeenAt    time.Time         `json:"last_seen_at"`
}

// ── Deployment / state types ──────────────────────────────────────────────────

// DeploymentStatus is the lifecycle state of a deployment.
type DeploymentStatus string

const (
	DeploymentStatusPending   DeploymentStatus = "pending"
	DeploymentStatusRunning   DeploymentStatus = "running"
	DeploymentStatusRebooting DeploymentStatus = "rebooting"
	DeploymentStatusDone      DeploymentStatus = "done"
	DeploymentStatusFailed    DeploymentStatus = "failed"
)

// DeploymentState is persisted on the server for each active deployment.
// ResumeStepIndex is the flat global step index the client should start from
// on the next connect (used after a reboot).
type DeploymentState struct {
	ID              string           `json:"id"`
	ClientID        string           `json:"client_id"`
	PlaybookID      string           `json:"playbook_id"`
	Status          DeploymentStatus `json:"status"`
	ResumeStepIndex int              `json:"resume_step_index"`
	StartedAt       time.Time        `json:"started_at"`
	UpdatedAt       time.Time        `json:"updated_at"`
	FinishedAt      *time.Time       `json:"finished_at,omitempty"`
	ErrorDetail     string           `json:"error_detail,omitempty"`
}

// ── Log types ─────────────────────────────────────────────────────────────────

// LogLevel is the severity of a log entry.
type LogLevel string

const (
	LogLevelDebug LogLevel = "debug"
	LogLevelInfo  LogLevel = "info"
	LogLevelWarn  LogLevel = "warn"
	LogLevelError LogLevel = "error"
)

// LogEntry represents a single log line from a client step.
type LogEntry struct {
	DeploymentID string    `json:"deployment_id"`
	JobName      string    `json:"job_name"`
	StepIndex    int       `json:"step_index"`
	Level        LogLevel  `json:"level"`
	Message      string    `json:"message"`
	Timestamp    time.Time `json:"timestamp"`
}

// LogBatch is a list of log entries sent in a single HTTP request
// (used by the admin log-retrieval endpoints).
type LogBatch struct {
	Entries []LogEntry `json:"entries"`
}

// ── Artifact types ────────────────────────────────────────────────────────────

// Artifact is metadata for a file stored on the daemon.
type Artifact struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Filename    string    `json:"filename"`
	Size        int64     `json:"size"`
	ContentType string    `json:"content_type"`
	UploadedAt  time.Time `json:"uploaded_at"`
}

// ── WebSocket protocol types ──────────────────────────────────────────────────

// WSMessageType is the discriminator for WebSocket envelope messages.
type WSMessageType string

const (
	// Server → Client
	WSMsgPlaybook WSMessageType = "playbook" // push deployment instructions
	WSMsgPing     WSMessageType = "ping"

	// Client → Server
	WSMsgLog          WSMessageType = "log"           // stream a log line
	WSMsgStepStart    WSMessageType = "step_start"    // step beginning
	WSMsgStepComplete WSMessageType = "step_complete" // step succeeded
	WSMsgStepFailed   WSMessageType = "step_failed"   // step error
	WSMsgReboot       WSMessageType = "reboot"        // client about to reboot
	WSMsgDeployDone   WSMessageType = "deploy_done"   // all steps complete
	WSMsgDeployFailed WSMessageType = "deploy_failed" // fatal deployment error
	WSMsgPong         WSMessageType = "pong"
)

// WSMessage is the JSON envelope for every WebSocket message.
type WSMessage struct {
	Type      WSMessageType `json:"type"`
	Timestamp time.Time     `json:"timestamp"`
	Data      interface{}   `json:"data,omitempty"`
}

// WSPlaybookData is the payload of a WSMsgPlaybook message.
type WSPlaybookData struct {
	DeploymentID     string            `json:"deployment_id"`
	Playbook         *Playbook         `json:"playbook"`
	ResumeStepIndex  int               `json:"resume_step_index"`
	Secrets          map[string]string `json:"secrets"`
	ArtifactsBaseURL string            `json:"artifacts_base_url"`
}

// WSLogData is the payload of a WSMsgLog message.
type WSLogData struct {
	DeploymentID string   `json:"deployment_id"`
	JobName      string   `json:"job_name"`
	StepIndex    int      `json:"step_index"`
	Level        LogLevel `json:"level"`
	Message      string   `json:"message"`
}

// WSStepData is the payload of WSMsgStepStart / Complete / Failed messages.
type WSStepData struct {
	DeploymentID string `json:"deployment_id"`
	JobName      string `json:"job_name"`
	StepName     string `json:"step_name"`
	StepIndex    int    `json:"step_index"`
	Error        string `json:"error,omitempty"`
}

// WSRebootData is the payload of a WSMsgReboot message.
type WSRebootData struct {
	DeploymentID    string `json:"deployment_id"`
	ResumeStepIndex int    `json:"resume_step_index"`
	Reason          string `json:"reason"`
}
