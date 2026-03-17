// Command kompakt-agent is the kompakt deployment agent.
//
// Usage:
//
//	kompakt-agent -config /etc/kompakt/client.config.yml
package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/marko-stanojevic/kompakt/internal/agent"
	"github.com/marko-stanojevic/kompakt/internal/common"
)

var runAgent = func(ctx context.Context, cfg *common.AgentConfig) error {
	c := agent.New(cfg)
	return c.Run(ctx)
}

func main() {
	configPath := flag.String("config", "client.config.yml", "path to client config file")
	flag.Parse()
	if err := runWithConfig(*configPath); err != nil {
		log.Fatalf("agent: %v", err)
	}
	log.Println("kompakt-agent stopped")
	os.Exit(0)
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
