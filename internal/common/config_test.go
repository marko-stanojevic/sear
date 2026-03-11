package common_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/marko-stanojevic/sear/internal/common"
)

func TestLoadDaemonConfig(t *testing.T) {
	content := `
listen_addr: ":9090"
data_dir: "/tmp/test-data"
token_expiry_hours: 48
`
	path := writeTempFile(t, "config.yml", content)
	cfg, err := common.LoadDaemonConfig(path)
	if err != nil {
		t.Fatalf("LoadDaemonConfig: %v", err)
	}
	if cfg.ListenAddr != ":9090" {
		t.Errorf("ListenAddr = %q; want :9090", cfg.ListenAddr)
	}
	if cfg.DataDir != "/tmp/test-data" {
		t.Errorf("DataDir = %q; want /tmp/test-data", cfg.DataDir)
	}
	if cfg.TokenExpiryHours != 48 {
		t.Errorf("TokenExpiryHours = %d; want 48", cfg.TokenExpiryHours)
	}
}

func TestLoadDaemonSecrets(t *testing.T) {
	content := `
root_password: "s3cr3t"
registration_secrets:
  prod: "abc123"
client_secrets:
  DB_PASS: "hunter2"
`
	path := writeTempFile(t, "secrets.yml", content)
	sec, err := common.LoadDaemonSecrets(path)
	if err != nil {
		t.Fatalf("LoadDaemonSecrets: %v", err)
	}
	if sec.RootPassword != "s3cr3t" {
		t.Errorf("RootPassword = %q; want s3cr3t", sec.RootPassword)
	}
	if sec.RegistrationSecrets["prod"] != "abc123" {
		t.Errorf("RegistrationSecrets[prod] = %q; want abc123", sec.RegistrationSecrets["prod"])
	}
	if sec.ClientSecrets["DB_PASS"] != "hunter2" {
		t.Errorf("ClientSecrets[DB_PASS] = %q; want hunter2", sec.ClientSecrets["DB_PASS"])
	}
}

func TestLoadClientConfig(t *testing.T) {
	content := `
server_url: "http://sear:8080"
registration_secret: "reg-secret"
platform: "baremetal"
poll_interval_seconds: 5
log_batch_size: 50
`
	path := writeTempFile(t, "client.yml", content)
	cfg, err := common.LoadClientConfig(path)
	if err != nil {
		t.Fatalf("LoadClientConfig: %v", err)
	}
	if cfg.ServerURL != "http://sear:8080" {
		t.Errorf("ServerURL = %q", cfg.ServerURL)
	}
	if cfg.PollIntervalSeconds != 5 {
		t.Errorf("PollIntervalSeconds = %d; want 5", cfg.PollIntervalSeconds)
	}
}

func TestLoadPlaybook(t *testing.T) {
	content := `
name: test-playbook
jobs:
  setup:
    name: Setup
    steps:
      - name: Install
        run: echo hello
        shell: bash
      - name: Reboot
        uses: reboot
      - name: Download
        uses: download-artifact
        with:
          name: mybin
          path: /tmp
`
	path := writeTempFile(t, "playbook.yml", content)
	pb, err := common.LoadPlaybook(path)
	if err != nil {
		t.Fatalf("LoadPlaybook: %v", err)
	}
	if pb.Name != "test-playbook" {
		t.Errorf("Name = %q; want test-playbook", pb.Name)
	}
	job, ok := pb.Jobs["setup"]
	if !ok {
		t.Fatal("job 'setup' not found")
	}
	if len(job.Steps) != 3 {
		t.Errorf("len(steps) = %d; want 3", len(job.Steps))
	}
	if job.Steps[1].Uses != "reboot" {
		t.Errorf("step[1].Uses = %q; want reboot", job.Steps[1].Uses)
	}
	if job.Steps[2].With["name"] != "mybin" {
		t.Errorf("step[2].With[name] = %q; want mybin", job.Steps[2].With["name"])
	}
}

func TestLoadConfigMissing(t *testing.T) {
	_, err := common.LoadDaemonConfig("/nonexistent/path/config.yml")
	if err == nil {
		t.Error("expected error for missing file, got nil")
	}
}

func writeTempFile(t *testing.T, name, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("writing temp file: %v", err)
	}
	return path
}
