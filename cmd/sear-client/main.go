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

func main() {
	configPath := flag.String("config", "client.config.yml", "path to client config file")
	flag.Parse()

	cfg, err := common.LoadClientConfig(*configPath)
	if err != nil {
		log.Fatalf("config: %v", err)
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
