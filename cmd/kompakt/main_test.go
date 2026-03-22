package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/marko-stanojevic/kompakt/internal/common"
)

func TestApplyConfigDefaults(t *testing.T) {
	cfg := &common.ServerConfig{}
	applyConfigDefaults(cfg)

	if cfg.ListenAddress != "http://localhost:8080" {
		t.Fatalf("ListenAddress = %q; want http://localhost:8080", cfg.ListenAddress)
	}
	if cfg.DataDir != "kompakt-data" {
		t.Fatalf("DataDir = %q; want kompakt-data", cfg.DataDir)
	}
	if cfg.ArtifactsDir == "" || cfg.LogsDir == "" {
		t.Fatalf("expected non-empty artifacts/logs dirs: artifacts=%q logs=%q", cfg.ArtifactsDir, cfg.LogsDir)
	}
	if cfg.TokenExpiryHours != 720 {
		t.Fatalf("TokenExpiryHours = %d; want 720", cfg.TokenExpiryHours)
	}
}

func TestApplyConfigDefaults_PreservesExistingValues(t *testing.T) {
	cfg := &common.ServerConfig{
		ListenAddress:    "http://localhost:9090",
		DataDir:          "/custom/data",
		ArtifactsDir:     "/custom/artifacts",
		LogsDir:          "/custom/logs",
		TokenExpiryHours: 48,
	}
	applyConfigDefaults(cfg)

	if cfg.ListenAddress != "http://localhost:9090" {
		t.Errorf("ListenAddress overwritten: got %q", cfg.ListenAddress)
	}
	if cfg.DataDir != "/custom/data" {
		t.Errorf("DataDir overwritten: got %q", cfg.DataDir)
	}
	if cfg.ArtifactsDir != "/custom/artifacts" {
		t.Errorf("ArtifactsDir overwritten: got %q", cfg.ArtifactsDir)
	}
	if cfg.LogsDir != "/custom/logs" {
		t.Errorf("LogsDir overwritten: got %q", cfg.LogsDir)
	}
	if cfg.TokenExpiryHours != 48 {
		t.Errorf("TokenExpiryHours overwritten: got %d", cfg.TokenExpiryHours)
	}
}

func TestApplyConfigDefaults_DirsDerivFromDataDir(t *testing.T) {
	cfg := &common.ServerConfig{DataDir: "/my/data"}
	applyConfigDefaults(cfg)

	wantArtifacts := filepath.Join("/my/data", "artifacts")
	wantLogs := filepath.Join("/my/data", "logs")
	if cfg.ArtifactsDir != wantArtifacts {
		t.Errorf("ArtifactsDir = %q; want %q", cfg.ArtifactsDir, wantArtifacts)
	}
	if cfg.LogsDir != wantLogs {
		t.Errorf("LogsDir = %q; want %q", cfg.LogsDir, wantLogs)
	}
}

func TestServerURL(t *testing.T) {
	httpURL := serverURL(&common.ServerConfig{ListenAddress: "http://localhost:8080"})
	if httpURL != "http://localhost:8080" {
		t.Fatalf("serverURL = %q; want http://localhost:8080", httpURL)
	}
}

func TestMustGenerateHex(t *testing.T) {
	got := mustGenerateHex(16)
	if len(got) != 32 {
		t.Fatalf("hex length = %d; want 32", len(got))
	}
	for _, ch := range got {
		if !strings.ContainsRune("0123456789abcdef", ch) {
			t.Fatalf("non-hex char %q in %q", ch, got)
		}
	}
}

// ── loadOrCreateSecret ────────────────────────────────────────────────────────

func TestLoadOrCreateSecret_ExplicitValueReturned(t *testing.T) {
	got := loadOrCreateSecret("my-explicit-secret", "/nonexistent/path")
	if got != "my-explicit-secret" {
		t.Fatalf("got %q; want my-explicit-secret", got)
	}
}

func TestLoadOrCreateSecret_ReadsExistingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".secret")
	if err := os.WriteFile(path, []byte("persisted-secret\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	got := loadOrCreateSecret("", path)
	if got != "persisted-secret" {
		t.Fatalf("got %q; want persisted-secret", got)
	}
}

func TestLoadOrCreateSecret_GeneratesAndPersists(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".secret")
	got := loadOrCreateSecret("", path)

	if len(got) != 64 { // 32 bytes → 64 hex chars
		t.Fatalf("generated secret length = %d; want 64", len(got))
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("secret file not written: %v", err)
	}
	if string(data) != got {
		t.Fatalf("persisted value %q != returned value %q", string(data), got)
	}
}

func TestLoadOrCreateSecret_StableAcrossRestarts(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".secret")
	first := loadOrCreateSecret("", path)
	second := loadOrCreateSecret("", path)
	if first != second {
		t.Fatalf("secret changed between calls: %q vs %q", first, second)
	}
}

func TestLoadOrCreateSecret_EmptyFileGeneratesNew(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".secret")
	if err := os.WriteFile(path, []byte("   \n"), 0o600); err != nil {
		t.Fatal(err)
	}
	got := loadOrCreateSecret("", path)
	if got == "" {
		t.Fatal("expected a non-empty secret for blank file")
	}
}

func TestLoadOrCreateSecret_ExplicitTakesPriorityOverFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".secret")
	if err := os.WriteFile(path, []byte("file-secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	got := loadOrCreateSecret("explicit-wins", path)
	if got != "explicit-wins" {
		t.Fatalf("got %q; want explicit-wins", got)
	}
}

// ── printBox ──────────────────────────────────────────────────────────────────

func TestPrintBox(t *testing.T) {
	oldStderr := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	os.Stderr = w
	defer func() {
		os.Stderr = oldStderr
		_ = w.Close()
		_ = r.Close()
	}()

	common.PrintBannerMessage("TITLE", "secret-value")
	_ = w.Close()

	buf := make([]byte, 4096)
	n, _ := r.Read(buf)
	out := string(buf[:n])
	if !strings.Contains(out, "TITLE") || !strings.Contains(out, "secret-value") {
		t.Fatalf("box output missing expected content: %q", out)
	}
}
