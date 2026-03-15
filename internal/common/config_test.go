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
  staging: "xyz789"
client_secrets:
  DB_PASS: "hunter2"
  API_KEY: "key-abc"
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
platform: "auto"
reconnect_interval_seconds: 5
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
	if cfg.ReconnectIntervalSeconds != 5 {
		t.Errorf("ReconnectIntervalSeconds = %d; want 5", cfg.ReconnectIntervalSeconds)
	}
}

func TestLoadPlaybook(t *testing.T) {
	// Jobs are now a YAML sequence (ordered slice), not a map.
	content := `
name: test-playbook
env:
  DEPLOY_ENV: production
jobs:
  - name: setup
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
	if pb.Env["DEPLOY_ENV"] != "production" {
		t.Errorf("Env[DEPLOY_ENV] = %q; want production", pb.Env["DEPLOY_ENV"])
	}
	if len(pb.Jobs) != 1 {
		t.Fatalf("len(jobs) = %d; want 1", len(pb.Jobs))
	}
	job := pb.Jobs[0]
	if job.Name != "setup" {
		t.Errorf("job.Name = %q; want setup", job.Name)
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

func TestFlattenPlaybook(t *testing.T) {
	pb := &common.Playbook{
		Name: "flat-test",
		Jobs: []common.Job{
			{Name: "j1", Steps: []common.Step{
				{Name: "s1", Run: "echo 1"},
				{Name: "s2", Run: "echo 2"},
			}},
			{Name: "j2", Steps: []common.Step{
				{Name: "s3", Run: "echo 3"},
			}},
		},
	}
	flat := common.FlattenPlaybook(pb)
	if len(flat) != 3 {
		t.Fatalf("len(flat) = %d; want 3", len(flat))
	}
	if flat[0].GlobalIndex != 0 || flat[0].JobName != "j1" || flat[0].Name != "s1" {
		t.Errorf("flat[0] = %+v", flat[0])
	}
	if flat[2].GlobalIndex != 2 || flat[2].JobName != "j2" || flat[2].Name != "s3" {
		t.Errorf("flat[2] = %+v", flat[2])
	}
}

func TestResolveSecrets(t *testing.T) {
	secrets := map[string]string{
		"DB_PASS": "hunter2",
		"API_KEY": "abc-123",
	}
	tests := []struct {
		input string
		want  string
	}{
		{"no secrets here", "no secrets here"},
		{"pass=${{ secrets.DB_PASS }}", "pass=hunter2"},
		{"key=${{ secrets.API_KEY }} pass=${{ secrets.DB_PASS }}", "key=abc-123 pass=hunter2"},
		{"${{ secrets.UNKNOWN }}", ""},
	}
	for _, tc := range tests {
		got := common.ResolveSecrets(tc.input, secrets)
		if got != tc.want {
			t.Errorf("ResolveSecrets(%q) = %q; want %q", tc.input, got, tc.want)
		}
	}
}

func TestResolveEnvSecrets(t *testing.T) {
	input := map[string]string{
		"PLAIN": "value",
		"PASS":  "${{ secrets.DB_PASS }}",
	}
	secrets := map[string]string{"DB_PASS": "hunter2"}

	resolved := common.ResolveEnvSecrets(input, secrets)
	if resolved["PLAIN"] != "value" {
		t.Fatalf("PLAIN = %q; want value", resolved["PLAIN"])
	}
	if resolved["PASS"] != "hunter2" {
		t.Fatalf("PASS = %q; want hunter2", resolved["PASS"])
	}

	// Ensure a new map is returned when input is non-empty.
	resolved["PASS"] = "changed"
	if input["PASS"] != "${{ secrets.DB_PASS }}" {
		t.Fatalf("input map should not be mutated, got %q", input["PASS"])
	}

	empty := common.ResolveEnvSecrets(nil, secrets)
	if empty != nil {
		t.Fatalf("nil input should return nil, got %#v", empty)
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
