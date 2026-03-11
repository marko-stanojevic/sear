package client_test

import (
	"testing"

	"github.com/marko-stanojevic/sear/internal/common"
	"github.com/marko-stanojevic/sear/internal/client"
)

// newTestClient returns a client with all defaults applied.
func newTestClient(serverURL string) *client.Client {
	cfg := &common.ClientConfig{
		ServerURL:          serverURL,
		RegistrationSecret: "test-secret",
		Platform:           "baremetal",
	}
	return client.New(cfg)
}

func TestNew_Defaults(t *testing.T) {
	c := newTestClient("http://localhost:8080")
	_ = c // Just ensure it constructs without panic.
}

// TestClientDefaults verifies that zero-value config fields get sensible
// defaults applied by New().
func TestClientDefaults(t *testing.T) {
	cfg := &common.ClientConfig{
		ServerURL: "http://sear:8080",
	}
	c := client.New(cfg)
	_ = c
	if cfg.PollIntervalSeconds == 0 {
		t.Error("PollIntervalSeconds should be defaulted")
	}
	if cfg.LogBatchSize == 0 {
		t.Error("LogBatchSize should be defaulted")
	}
	if cfg.StateFile == "" {
		t.Error("StateFile should be defaulted")
	}
}
