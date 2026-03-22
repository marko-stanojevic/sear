package agent_test

import (
	"testing"

	"github.com/marko-stanojevic/kompakt/internal/agent"
	"github.com/marko-stanojevic/kompakt/internal/common"
)

func newTestAgent(t *testing.T, serverURL string) *agent.Agent {
	t.Helper()
	cfg := &common.AgentConfig{
		ServerURL:          serverURL,
		RegistrationSecret: "test-secret",
	}
	c, err := agent.New(cfg)
	if err != nil {
		t.Fatalf("agent.New: %v", err)
	}
	return c
}

func TestNew_Defaults(t *testing.T) {
	c := newTestAgent(t, "http://localhost:8080")
	_ = c // Ensure construction does not panic.
}

func TestAgentDefaults(t *testing.T) {
	cfg := &common.AgentConfig{
		ServerURL: "http://kompakt:8080",
	}
	if _, err := agent.New(cfg); err != nil {
		t.Fatalf("agent.New: %v", err)
	}
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
