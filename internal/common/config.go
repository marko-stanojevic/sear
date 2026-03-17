// Package common defines shared types used by both the server and agent.
package common

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

// ── Server config (config.yml) ────────────────────────────────────────────────

// ServerConfig is the main server configuration file.
type ServerConfig struct {
	// ListenAddr is the address to bind the HTTP server (default ":8080").
	ListenAddr string `yaml:"listen_addr"`

	// DataDir is the directory where the server stores its state.
	DataDir string `yaml:"data_dir"`

	// ArtifactsDir is the directory where uploaded artifacts are stored.
	// Defaults to DataDir/artifacts when empty.
	ArtifactsDir string `yaml:"artifacts_dir"`

	// LogsDir is the directory where per-deployment log files are stored.
	// Defaults to DataDir/logs when empty.
	LogsDir string `yaml:"logs_dir"`

	// TLSCertFile and TLSKeyFile enable TLS when both are set.
	TLSCertFile string `yaml:"tls_cert_file"`
	TLSKeyFile  string `yaml:"tls_key_file"`

	// JWTSecret is used to sign client tokens.
	// If empty, a random secret is generated on startup.
	JWTSecret string `yaml:"jwt_secret"`

	// TokenExpiryHours is the JWT token lifetime in hours (default 720 = 30 days).
	TokenExpiryHours int `yaml:"token_expiry_hours"`
}

// ── Server secrets (secrets.yml) ─────────────────────────────────────────────

// ServerSecrets holds sensitive configuration loaded from secrets.yml.
type ServerSecrets struct {
	// RootPassword authenticates admin API calls.
	// If empty, a random password is generated on startup and printed.
	RootPassword string `yaml:"root_password"`

	// RegistrationSecrets maps a friendly name to the pre-shared secret value.
	// Clients must present one of these values during /api/v1/register.
	// Using named entries lets operators manage secrets independently
	// (e.g., rotate a single datacenter's PSK without touching others).
	RegistrationSecrets map[string]string `yaml:"registration_secrets"`

	// ClientSecrets are named key/value pairs injected into playbooks via
	// ${{ secrets.NAME }} syntax.
	ClientSecrets map[string]string `yaml:"client_secrets"`
}

// ── Agent config (client.config.yml) ────────────────────────────────────────

// AgentConfig is the configuration file used by kompakt-agent.
type AgentConfig struct {
	// ServerURL is the base URL of the kompakt server (e.g. "http://kompakt:8080").
	ServerURL string `yaml:"server_url"`

	// RegistrationSecret must match one of the server's registration_secrets values.
	RegistrationSecret string `yaml:"registration_secret"`

	// Platform hint: linux | mac | windows | auto (default).
	// When "auto", the agent detects the platform from the OS.
	Platform string `yaml:"platform"`

	// StateFile is where the agent persists its token and resume position
	// so that deployment can be resumed after a reboot.
	// Default: /var/lib/kompakt/state.json (Linux), C:\ProgramData\kompakt\state.json (Windows).
	StateFile string `yaml:"state_file"`

	// WorkDir is the directory where shell steps are executed.
	WorkDir string `yaml:"work_dir"`

	// ReconnectIntervalSeconds is how long the agent waits before retrying
	// a failed WebSocket connection (default 10).
	ReconnectIntervalSeconds int `yaml:"reconnect_interval_seconds"`

	// LogBatchSize is the maximum number of log lines buffered before a
	// forced flush (default 100).
	LogBatchSize int `yaml:"log_batch_size"`
}

// ── Generic YAML loader ───────────────────────────────────────────────────────

// LoadServerConfig reads and unmarshals a ServerConfig from path.
func LoadServerConfig(path string) (*ServerConfig, error) {
	return loadYAML[ServerConfig](path)
}

// LoadServerSecrets reads and unmarshals ServerSecrets from path.
func LoadServerSecrets(path string) (*ServerSecrets, error) {
	return loadYAML[ServerSecrets](path)
}

// LoadAgentConfig reads and unmarshals an AgentConfig from path.
func LoadAgentConfig(path string) (*AgentConfig, error) {
	cfg, err := loadYAML[AgentConfig](path)
	if err != nil {
		return nil, err
	}
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("validating %s: %w", path, err)
	}
	return cfg, nil
}

// LoadPlaybook reads and unmarshals a Playbook from a YAML file.
func LoadPlaybook(path string) (*Playbook, error) {
	return loadYAML[Playbook](path)
}

func loadYAML[T any](path string) (*T, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}
	var v T
	if err := yaml.Unmarshal(data, &v); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}
	return &v, nil
}

// Validate checks that the client config contains the required fields and
// uses a supported platform hint.
func (c *AgentConfig) Validate() error {
	if strings.TrimSpace(c.ServerURL) == "" {
		return fmt.Errorf("server_url is required")
	}
	if strings.TrimSpace(c.RegistrationSecret) == "" {
		return fmt.Errorf("registration_secret is required")
	}
	platform := strings.ToLower(strings.TrimSpace(c.Platform))
	switch platform {
	case "", "auto", "linux", "mac", "windows":
		return nil
	default:
		return fmt.Errorf("platform must be one of auto, linux, mac, or windows")
	}
}
