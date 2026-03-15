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

	"github.com/marko-stanojevic/sear/internal/client"
	"github.com/marko-stanojevic/sear/internal/common"
)

var runClient = func(ctx context.Context, cfg *common.ClientConfig) error {
	c := client.New(cfg)
	return c.Run(ctx)
}

func main() {
	configPath := flag.String("config", "client.config.yml", "path to client config file")
	flag.Parse()
	if err := runWithConfig(*configPath); err != nil {
		log.Fatalf("client: %v", err)
	}
	log.Println("sear-client stopped")
	os.Exit(0)
}

func runWithConfig(configPath string) error {
	cfg, err := common.LoadClientConfig(configPath)
	if err != nil {
		return err
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	if err := runClient(ctx, cfg); err != nil && err != context.Canceled {
		return err
	}
	return nil
}
