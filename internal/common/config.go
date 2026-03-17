// Package common defines shared types used by both the daemon and client.
package common

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

// ── Daemon config (config.yml) ────────────────────────────────────────────────

// DaemonConfig is the main daemon configuration file.
type DaemonConfig struct {
	// ListenAddr is the address to bind the HTTP server (default ":8080").
	ListenAddr string `yaml:"listen_addr"`

	// DataDir is the directory where the daemon stores its state.
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

// ── Daemon secrets (secrets.yml) ─────────────────────────────────────────────

// DaemonSecrets holds sensitive configuration loaded from secrets.yml.
type DaemonSecrets struct {
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

// ── Client config (client.config.yml) ────────────────────────────────────────

// ClientConfig is the configuration file used by kompakt-agent.
type ClientConfig struct {
	// ServerURL is the base URL of the kompakt daemon (e.g. "http://kompakt:8080").
	ServerURL string `yaml:"server_url"`

	// RegistrationSecret must match one of the daemon's registration_secrets values.
	RegistrationSecret string `yaml:"registration_secret"`

	// Platform hint: linux | mac | windows | auto (default).
	// When "auto", the client detects the platform from the OS.
	Platform string `yaml:"platform"`

	// StateFile is where the client persists its token and resume position
	// so that deployment can be resumed after a reboot.
	// Default: /var/lib/kompakt/state.json (Linux), C:\ProgramData\kompakt\state.json (Windows).
	StateFile string `yaml:"state_file"`

	// WorkDir is the directory where shell steps are executed.
	WorkDir string `yaml:"work_dir"`

	// ReconnectIntervalSeconds is how long the client waits before retrying
	// a failed WebSocket connection (default 10).
	ReconnectIntervalSeconds int `yaml:"reconnect_interval_seconds"`

	// LogBatchSize is the maximum number of log lines buffered before a
	// forced flush (default 100).
	LogBatchSize int `yaml:"log_batch_size"`
}

// ── Generic YAML loader ───────────────────────────────────────────────────────

// LoadDaemonConfig reads and unmarshals a DaemonConfig from path.
func LoadDaemonConfig(path string) (*DaemonConfig, error) {
	return loadYAML[DaemonConfig](path)
}

// LoadDaemonSecrets reads and unmarshals DaemonSecrets from path.
func LoadDaemonSecrets(path string) (*DaemonSecrets, error) {
	return loadYAML[DaemonSecrets](path)
}

// LoadClientConfig reads and unmarshals a ClientConfig from path.
func LoadClientConfig(path string) (*ClientConfig, error) {
	cfg, err := loadYAML[ClientConfig](path)
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
func (c *ClientConfig) Validate() error {
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
