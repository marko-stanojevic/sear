package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/marko-stanojevic/kompakt/internal/common"
)

func writeClientConfig(t *testing.T, path string) {
	t.Helper()
	content := "server_url: \"http://localhost:8080\"\nregistration_secret: \"reg-secret\"\nplatform: \"auto\"\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
}

func TestRunWithConfig(t *testing.T) {
	cfgPath := filepath.Join(t.TempDir(), "client.config.yml")
	writeClientConfig(t, cfgPath)

	oldRunClient := runClient
	defer func() { runClient = oldRunClient }()

	t.Run("success", func(t *testing.T) {
		called := false
		runClient = func(ctx context.Context, cfg *common.ClientConfig) error {
			called = true
			if ctx == nil {
				t.Fatal("expected non-nil context")
			}
			return nil
		}
		if err := runWithConfig(cfgPath); err != nil {
			t.Fatalf("runWithConfig: %v", err)
		}
		if !called {
			t.Fatal("expected runClient to be called")
		}
	})

	t.Run("context canceled is ignored", func(t *testing.T) {
		runClient = func(ctx context.Context, cfg *common.ClientConfig) error {
			return context.Canceled
		}
		if err := runWithConfig(cfgPath); err != nil {
			t.Fatalf("expected nil for context canceled, got %v", err)
		}
	})

	t.Run("runner error is returned", func(t *testing.T) {
		runClient = func(ctx context.Context, cfg *common.ClientConfig) error {
			return errors.New("boom")
		}
		err := runWithConfig(cfgPath)
		if err == nil || err.Error() != "boom" {
			t.Fatalf("expected boom error, got %v", err)
		}
	})

	t.Run("missing config returns error", func(t *testing.T) {
		err := runWithConfig(filepath.Join(t.TempDir(), "missing.yml"))
		if err == nil {
			t.Fatal("expected missing config error")
		}
	})
}
