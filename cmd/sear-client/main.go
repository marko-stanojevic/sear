// sear-client is the deployment client component of the Sear framework.
//
// It registers with the sear-daemon, polls for playbook instructions,
// executes steps, and persists state across reboots.
//
// Usage:
//
//	sear-client [--config config.yml]
package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/marko-stanojevic/sear/internal/client"
	"github.com/marko-stanojevic/sear/internal/common"
)

func main() {
	configPath := flag.String("config", "config.yml", "path to client config file")
	flag.Parse()

	cfg, err := loadConfig(*configPath)
	if err != nil {
		log.Fatalf("config: %v", err)
	}
	if cfg.ServerURL == "" {
		log.Fatal("server_url must be set in config.yml")
	}

	c := client.New(cfg)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		ch := make(chan os.Signal, 1)
		signal.Notify(ch, syscall.SIGINT, syscall.SIGTERM)
		<-ch
		log.Println("shutting down client...")
		cancel()
	}()

	if err := c.Run(ctx); err != nil && err != context.Canceled {
		log.Fatalf("client: %v", err)
	}
}

func loadConfig(path string) (*common.ClientConfig, error) {
	cfg, err := common.LoadClientConfig(path)
	if os.IsNotExist(err) {
		return &common.ClientConfig{}, nil
	}
	return cfg, err
}
