// Package common defines shared types used by both the daemon and client.
package common

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// ---- Daemon config (config.yml) --------------------------------------------

// DaemonConfig is the main daemon configuration file.
type DaemonConfig struct {
	// ListenAddr is the address to bind the HTTP server (default ":8080").
	ListenAddr string `yaml:"listen_addr"`

	// DataDir is the directory where the daemon stores its state.
	DataDir string `yaml:"data_dir"`

	// ArtifactsDir is the directory where uploaded artifacts are stored.
	ArtifactsDir string `yaml:"artifacts_dir"`

	// LogsDir is the directory where client logs are stored.
	LogsDir string `yaml:"logs_dir"`

	// TLSCertFile and TLSKeyFile enable TLS when both are set.
	TLSCertFile string `yaml:"tls_cert_file"`
	TLSKeyFile  string `yaml:"tls_key_file"`

	// JWTSecret is used to sign client tokens.  If empty, a random secret is
	// generated on startup.
	JWTSecret string `yaml:"jwt_secret"`

	// TokenExpiry is the JWT token expiry in hours (default 720 = 30 days).
	TokenExpiryHours int `yaml:"token_expiry_hours"`
}

// ---- Daemon secrets (secrets.yml) ------------------------------------------

// DaemonSecrets holds sensitive configuration loaded from secrets.yml.
type DaemonSecrets struct {
	// RootPassword is used to authenticate admin API calls.
	// If empty, a random password is generated on startup and printed.
	RootPassword string `yaml:"root_password"`

	// RegistrationSecrets is a map of secret-name → secret-value.
	// Clients must present one of these values during registration.
	RegistrationSecrets map[string]string `yaml:"registration_secrets"`

	// ClientSecrets are injected into playbooks as environment variables.
	ClientSecrets map[string]string `yaml:"client_secrets"`
}

// ---- Client config (config.yml) --------------------------------------------

// ClientConfig is the configuration file used by the sear-client.
type ClientConfig struct {
	// ServerURL is the base URL of the sear-daemon (e.g. "http://sear:8080").
	ServerURL string `yaml:"server_url"`

	// RegistrationSecret must match one of the daemon's registration_secrets.
	RegistrationSecret string `yaml:"registration_secret"`

	// Platform hint; when "auto" (default) the client auto-detects.
	Platform string `yaml:"platform"`

	// StateFile is the path where the client persists its own state so that
	// it can resume after a reboot (default "/var/lib/sear/state.json").
	StateFile string `yaml:"state_file"`

	// PollIntervalSeconds is how often the client polls /api/v1/connect
	// (default 10).
	PollIntervalSeconds int `yaml:"poll_interval_seconds"`

	// LogBatchSize is the maximum number of log lines sent per request
	// (default 100).
	LogBatchSize int `yaml:"log_batch_size"`
}

// ---- Loader helpers --------------------------------------------------------

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
	return loadYAML[ClientConfig](path)
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
