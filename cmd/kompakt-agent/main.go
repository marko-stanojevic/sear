// Command kompakt-agent is the kompakt deployment agent.
//
// Usage:
//
//	kompakt-agent -config /etc/kompakt/client.config.yml
package main

import (
	"context"
	"flag"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/marko-stanojevic/kompakt/internal/agent"
	"github.com/marko-stanojevic/kompakt/internal/common"
	"github.com/marko-stanojevic/kompakt/internal/terminal"
)

var runAgent = func(ctx context.Context, cfg *common.AgentConfig) error {
	c, err := agent.New(cfg)
	if err != nil {
		return err
	}
	return c.Run(ctx)
}

func main() {
	configPath := flag.String("config", "client.config.yml", "path to client config file")
	debug := flag.Bool("debug", false, "enable verbose debug logging")
	flag.Parse()

	terminal.Setup(*debug)

	if err := runWithConfig(*configPath); err != nil {
		slog.Error("agent error", "error", err)
		os.Exit(1)
	}
	slog.Info("kompakt-agent stopped")
}

func runWithConfig(configPath string) error {
	cfg, err := common.LoadAgentConfig(configPath)
	if err != nil {
		return err
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	if err := runAgent(ctx, cfg); err != nil && err != context.Canceled {
		return err
	}
	return nil
}
