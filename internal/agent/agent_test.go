package agent_test

import (
	"testing"

	"github.com/marko-stanojevic/kompakt/internal/agent"
	"github.com/marko-stanojevic/kompakt/internal/common"
)

func newTestAgent(serverURL string) *agent.Agent {
	cfg := &common.AgentConfig{
		ServerURL:          serverURL,
		RegistrationSecret: "test-secret",
		Platform:           "auto",
	}
	return agent.New(cfg)
}

func TestNew_Defaults(t *testing.T) {
	c := newTestAgent("http://localhost:8080")
	_ = c // Ensure construction does not panic.
}

func TestAgentDefaults(t *testing.T) {
	cfg := &common.AgentConfig{
		ServerURL: "http://kompakt:8080",
	}
	_ = agent.New(cfg)
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
