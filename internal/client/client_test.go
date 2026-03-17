package client_test

import (
	"testing"

	"github.com/marko-stanojevic/kompakt/internal/client"
	"github.com/marko-stanojevic/kompakt/internal/common"
)

func newTestClient(serverURL string) *client.Client {
	cfg := &common.ClientConfig{
		ServerURL:          serverURL,
		RegistrationSecret: "test-secret",
		Platform:           "auto",
	}
	return client.New(cfg)
}

func TestNew_Defaults(t *testing.T) {
	c := newTestClient("http://localhost:8080")
	_ = c // Ensure construction does not panic.
}

func TestClientDefaults(t *testing.T) {
	cfg := &common.ClientConfig{
		ServerURL: "http://kompakt:8080",
	}
	_ = client.New(cfg)
	if cfg.ReconnectIntervalSeconds == 0 {
		t.Error("ReconnectIntervalSeconds should be defaulted")
	}
	if cfg.LogBatchSize == 0 {
		t.Error("LogBatchSize should be defaulted")
	}
	if cfg.StateFile == "" {
		t.Error("StateFile should be defaulted")
	}
}
