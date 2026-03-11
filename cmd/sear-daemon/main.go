// sear-daemon is the server component of the Sear deployment framework.
//
// Usage:
//
//	sear-daemon [--config config.yml] [--secrets secrets.yml]
package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/marko-stanojevic/sear/internal/common"
	"github.com/marko-stanojevic/sear/internal/daemon"
	"github.com/marko-stanojevic/sear/internal/daemon/handlers"
	"github.com/marko-stanojevic/sear/internal/daemon/store"
)

func main() {
	configPath := flag.String("config", "config.yml", "path to daemon config file")
	secretsPath := flag.String("secrets", "secrets.yml", "path to secrets file")
	flag.Parse()

	cfg, err := loadConfig(*configPath)
	if err != nil {
		log.Fatalf("config: %v", err)
	}
	sec, err := loadSecrets(*secretsPath)
	if err != nil {
		log.Fatalf("secrets: %v", err)
	}

	// ---- Directories -------------------------------------------------------
	if cfg.DataDir == "" {
		cfg.DataDir = "/var/lib/sear/data"
	}
	if cfg.ArtifactsDir == "" {
		cfg.ArtifactsDir = "/var/lib/sear/artifacts"
	}
	if cfg.LogsDir == "" {
		cfg.LogsDir = "/var/lib/sear/logs"
	}
	for _, d := range []string{cfg.DataDir, cfg.ArtifactsDir, cfg.LogsDir} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			log.Fatalf("mkdir %s: %v", d, err)
		}
	}

	// ---- Storage -----------------------------------------------------------
	st, err := store.New(cfg.DataDir)
	if err != nil {
		log.Fatalf("store: %v", err)
	}
	// Merge client secrets from secrets.yml into the store.
	if len(sec.ClientSecrets) > 0 {
		if err := st.MergeSecrets(sec.ClientSecrets); err != nil {
			log.Printf("warn: could not merge client secrets: %v", err)
		}
	}

	// ---- JWT secret --------------------------------------------------------
	jwtSecret := []byte(cfg.JWTSecret)
	if len(jwtSecret) == 0 {
		b := make([]byte, 32)
		if _, err := rand.Read(b); err != nil {
			log.Fatalf("generating jwt secret: %v", err)
		}
		jwtSecret = b
	}

	// ---- Root password -----------------------------------------------------
	rootPass := sec.RootPassword
	if rootPass == "" {
		rootPass, err = randHex(16)
		if err != nil {
			log.Fatalf("generating root password: %v", err)
		}
		fmt.Printf("\n⚠  No root_password found in secrets.yml.\n")
		fmt.Printf("   Generated root password: %s\n\n", rootPass)
	}

	// ---- Listen address ----------------------------------------------------
	addr := cfg.ListenAddr
	if addr == "" {
		addr = ":8080"
	}

	serverURL := os.Getenv("SEAR_SERVER_URL")
	if serverURL == "" {
		serverURL = "http://localhost" + addr
	}

	// ---- Registration secrets ----------------------------------------------
	regSecrets := sec.RegistrationSecrets
	if len(regSecrets) == 0 {
		generatedSecret, err := randHex(16)
		if err != nil {
			log.Fatalf("generating registration secret: %v", err)
		}
		regSecrets = map[string]string{"default": generatedSecret}
		fmt.Printf("   Generated registration secret: %s\n", generatedSecret)
		fmt.Printf("   Server URL: %s\n\n", serverURL)
	}

	// ---- Build handler env -------------------------------------------------
	env := &handlers.Env{
		Store:               st,
		JWTSecret:           jwtSecret,
		RootPassword:        rootPass,
		TokenExpiryHours:    cfg.TokenExpiryHours,
		ArtifactsDir:        cfg.ArtifactsDir,
		ServerURL:           serverURL,
		RegistrationSecrets: regSecrets,
	}

	// ---- HTTP server -------------------------------------------------------
	srv := &http.Server{
		Addr:         addr,
		Handler:      daemon.NewServer(env),
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 60 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	// Graceful shutdown on SIGINT/SIGTERM.
	go func() {
		ch := make(chan os.Signal, 1)
		signal.Notify(ch, syscall.SIGINT, syscall.SIGTERM)
		<-ch
		log.Println("shutting down...")
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = srv.Shutdown(ctx)
	}()

	if cfg.TLSCertFile != "" && cfg.TLSKeyFile != "" {
		log.Printf("sear-daemon listening on %s (TLS)", addr)
		if err := srv.ListenAndServeTLS(cfg.TLSCertFile, cfg.TLSKeyFile); err != nil && err != http.ErrServerClosed {
			log.Fatalf("server: %v", err)
		}
	} else {
		log.Printf("sear-daemon listening on %s", addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("server: %v", err)
		}
	}
}

// loadConfig reads config.yml; returns defaults if the file does not exist.
func loadConfig(path string) (*common.DaemonConfig, error) {
	cfg, err := common.LoadDaemonConfig(path)
	if os.IsNotExist(err) {
		return &common.DaemonConfig{}, nil
	}
	return cfg, err
}

// loadSecrets reads secrets.yml; returns an empty struct if the file does
// not exist (root password and registration secrets will be auto-generated).
func loadSecrets(path string) (*common.DaemonSecrets, error) {
	sec, err := common.LoadDaemonSecrets(path)
	if os.IsNotExist(err) {
		return &common.DaemonSecrets{}, nil
	}
	return sec, err
}

func randHex(bytes int) (string, error) {
	b := make([]byte, bytes)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
