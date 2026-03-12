// Command sear-client is the sear deployment agent.
//
// Usage:
//
//	sear-client -config /etc/sear/client.config.yml
package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/sear-project/sear/internal/client"
	"github.com/sear-project/sear/internal/common"
)

func main() {
	configPath := flag.String("config", "client.config.yml", "path to client config file")
	flag.Parse()

	cfg, err := common.LoadClientConfig(*configPath)
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	if cfg.ServerURL == "" {
		log.Fatal("config: server_url is required")
	}
	if cfg.RegistrationSecret == "" {
		log.Fatal("config: registration_secret is required")
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	c := client.New(cfg)
	if err := c.Run(ctx); err != nil && err != context.Canceled {
		log.Fatalf("client: %v", err)
	}
	log.Println("sear-client stopped")
	os.Exit(0)
}
