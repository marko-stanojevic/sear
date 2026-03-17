// Command kompakt is the kompakt deployment server.
//
// Usage:
//
//	kompakt -config config.yml -secrets secrets.yml
package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"flag"
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/marko-stanojevic/kompakt/internal/common"
	server "github.com/marko-stanojevic/kompakt/internal/server"
	"github.com/marko-stanojevic/kompakt/internal/server/handlers"
	"github.com/marko-stanojevic/kompakt/internal/server/service"
	"github.com/marko-stanojevic/kompakt/internal/server/store"
)

func main() {
	configPath := flag.String("config", "config.yml", "path to server config file")
	secretsPath := flag.String("secrets", "secrets.yml", "path to server secrets file")
	flag.Parse()

	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, nil)))

	cfg, err := common.LoadServerConfig(*configPath)
	if err != nil {
		log.Fatalf("config: %v", err)
	}
	applyConfigDefaults(cfg)

	sec, err := common.LoadServerSecrets(*secretsPath)
	if err != nil {
		// secrets.yml is optional on first run — we'll auto-generate what we need.
		log.Printf("note: could not load %s (%v); using generated credentials", *secretsPath, err)
		sec = &common.ServerSecrets{}
	}

	// ── Ensure data directory exists before loading persisted secrets ────────
	if err := os.MkdirAll(cfg.DataDir, 0o700); err != nil {
		log.Fatalf("mkdir %s: %v", cfg.DataDir, err)
	}

	// ── JWT secrets (agent + UI, persisted across restarts) ──────────────────
	cfg.AgentJWTSecret = loadOrCreateSecret(cfg.AgentJWTSecret, filepath.Join(cfg.DataDir, ".agent-jwt-secret"))
	cfg.UserJWTSecret = loadOrCreateSecret(cfg.UserJWTSecret, filepath.Join(cfg.DataDir, ".ui-jwt-secret"))

	// ── Root password ────────────────────────────────────────────────────────
	if sec.RootPassword == "" {
		sec.RootPassword = mustGenerateHex(16)
		printBox("GENERATED ROOT PASSWORD", sec.RootPassword)
	}

	// ── Registration secrets ──────────────────────────────────────────────────
	if len(sec.RegistrationSecrets) == 0 {
		secret := mustGenerateHex(16)
		sec.RegistrationSecrets = map[string]string{"default": secret}
		printBox("GENERATED REGISTRATION SECRET", secret)
	}

	// ── Ensure directories ────────────────────────────────────────────────────
	for _, dir := range []string{cfg.ArtifactsDir, cfg.LogsDir} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			log.Fatalf("mkdir %s: %v", dir, err)
		}
	}

	// ── Store ─────────────────────────────────────────────────────────────────
	st, err := store.New(cfg.DataDir, cfg.LogsDir)
	if err != nil {
		log.Fatalf("store: %v", err)
	}
	// Seed secrets from secrets.yml.
	if len(sec.ClientSecrets) > 0 {
		if err := st.MergeSecrets(sec.ClientSecrets); err != nil {
			log.Fatalf("seeding secrets: %v", err)
		}
	}

	// ── Handler environment ───────────────────────────────────────────────────
	hub := handlers.NewHub()
	svc := &service.Manager{Store: st, Hub: hub, ServerURL: serverURL(cfg)}
	env := &handlers.Handler{
		Store:               st,
		AgentJWTSecret:      []byte(cfg.AgentJWTSecret),
		UserJWTSecret:       []byte(cfg.UserJWTSecret),
		RootPassword:        sec.RootPassword,
		TokenExpiryHours:    cfg.TokenExpiryHours,
		ArtifactsDir:        cfg.ArtifactsDir,
		ServerURL:           serverURL(cfg),
		RegistrationSecrets: sec.RegistrationSecrets,
		Hub:                 hub,
		Service:             svc,
	}

	// ── HTTP server ───────────────────────────────────────────────────────────
	handler := server.NewServer(env)
	srv := &http.Server{
		Addr:         cfg.ListenAddr,
		Handler:      handler,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 60 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	log.Printf("kompakt listening on %s", cfg.ListenAddr)
	log.Printf("status UI: http://localhost%s/status/ui", cfg.ListenAddr)

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
		log.Printf("TLS enabled")
		if err := srv.ListenAndServeTLS(cfg.TLSCertFile, cfg.TLSKeyFile); err != nil && err != http.ErrServerClosed {
			log.Fatalf("server: %v", err)
		}
	} else {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("server: %v", err)
		}
	}
}

func applyConfigDefaults(cfg *common.ServerConfig) {
	if cfg.ListenAddr == "" {
		cfg.ListenAddr = ":8080"
	}
	if cfg.DataDir == "" {
		cfg.DataDir = "kompakt-data"
	}
	if cfg.ArtifactsDir == "" {
		cfg.ArtifactsDir = filepath.Join(cfg.DataDir, "artifacts")
	}
	if cfg.LogsDir == "" {
		cfg.LogsDir = filepath.Join(cfg.DataDir, "logs")
	}
	if cfg.TokenExpiryHours == 0 {
		cfg.TokenExpiryHours = 720 // 30 days
	}
}

func serverURL(cfg *common.ServerConfig) string {
	if cfg.TLSCertFile != "" {
		return "https://localhost" + cfg.ListenAddr
	}
	return "http://localhost" + cfg.ListenAddr
}

// loadOrCreateSecret returns the explicit value if non-empty. Otherwise it
// tries to read the secret from path; if the file does not exist it generates
// a new secret, writes it to path, and returns it. This ensures secrets
// survive server restarts without requiring manual config.
func loadOrCreateSecret(explicit, path string) string {
	if explicit != "" {
		return explicit
	}
	if data, err := os.ReadFile(path); err == nil {
		if s := strings.TrimSpace(string(data)); s != "" {
			return s
		}
	}
	s := mustGenerateHex(32)
	if err := os.WriteFile(path, []byte(s), 0o600); err != nil {
		log.Printf("warn: could not persist secret to %s: %v", path, err)
	}
	return s
}

func mustGenerateHex(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		log.Fatalf("rand: %v", err)
	}
	return hex.EncodeToString(b)
}

func printBox(title, content string) {
	maxLen := len(content)
	if len(title) > maxLen {
		maxLen = len(title)
	}
	width := maxLen + 4
	bar := strings.Repeat("─", width)

	titlePadTotal := width - len(title)
	if titlePadTotal < 0 {
		titlePadTotal = 0
	}
	leftPad := titlePadTotal / 2
	rightPad := titlePadTotal - leftPad
	leftPadStr := strings.Repeat(" ", leftPad)
	rightPadStr := strings.Repeat(" ", rightPad)

	fmt.Fprintf(os.Stderr, "\n┌%s┐\n│%s%s%s│\n│  %s  │\n└%s┘\n\n",
		bar, leftPadStr, title, rightPadStr, content, bar)
}
