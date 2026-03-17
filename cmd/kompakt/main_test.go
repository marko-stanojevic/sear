package main

import (
	"os"
	"strings"
	"testing"

	"github.com/marko-stanojevic/kompakt/internal/common"
)

func TestApplyConfigDefaults(t *testing.T) {
	cfg := &common.ServerConfig{}
	applyConfigDefaults(cfg)

	if cfg.ListenAddr != ":8080" {
		t.Fatalf("ListenAddr = %q; want :8080", cfg.ListenAddr)
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

func TestServerURL(t *testing.T) {
	httpURL := serverURL(&common.ServerConfig{ListenAddr: ":8080"})
	if httpURL != "http://localhost:8080" {
		t.Fatalf("http serverURL = %q; want http://localhost:8080", httpURL)
	}

	httpsURL := serverURL(&common.ServerConfig{ListenAddr: ":8443", TLSCertFile: "cert.pem"})
	if httpsURL != "https://localhost:8443" {
		t.Fatalf("https serverURL = %q; want https://localhost:8443", httpsURL)
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

	printBox("TITLE", "secret-value")
	_ = w.Close()

	buf := make([]byte, 4096)
	n, _ := r.Read(buf)
	out := string(buf[:n])
	if !strings.Contains(out, "TITLE") || !strings.Contains(out, "secret-value") {
		t.Fatalf("box output missing expected content: %q", out)
	}
}
