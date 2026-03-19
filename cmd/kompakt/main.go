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
	"github.com/marko-stanojevic/kompakt/internal/terminal"
)

func main() {
	configPath := flag.String("config", "config.yml", "path to server config file")
	secretsPath := flag.String("secrets", "secrets.yml", "path to server secrets file")
	debug := flag.Bool("debug", false, "log all HTTP requests (default: WebSocket and errors only)")
	flag.Parse()

	terminal.Setup(*debug)

	cfg, err := common.LoadServerConfig(*configPath)
	if err != nil {
		slog.Error("config load failed", "error", err)
		os.Exit(1)
	}
	applyConfigDefaults(cfg)

	sec, err := common.LoadServerSecrets(*secretsPath)
	if err != nil {
		// secrets.yml is optional on first run — we'll auto-generate what we need.
		slog.Warn("could not load secrets file, using generated credentials", "path", *secretsPath, "error", err)
		sec = &common.ServerSecrets{}
	}

	// ── Ensure data directory exists before loading persisted secrets ────────
	if err := os.MkdirAll(cfg.DataDir, 0o700); err != nil {
		slog.Error("mkdir failed", "path", cfg.DataDir, "error", err)
		os.Exit(1)
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
			slog.Error("mkdir failed", "path", dir, "error", err)
			os.Exit(1)
		}
	}

	// ── Store ─────────────────────────────────────────────────────────────────
	st, err := store.New(cfg.DataDir, cfg.LogsDir)
	if err != nil {
		slog.Error("store init failed", "error", err)
		os.Exit(1)
	}
	// Seed secrets from secrets.yml.
	if len(sec.ClientSecrets) > 0 {
		if err := st.MergeSecrets(sec.ClientSecrets); err != nil {
			slog.Error("seeding secrets failed", "error", err)
			os.Exit(1)
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

	slog.Info("kompakt listening", "addr", cfg.ListenAddr)
	if cfg.TLSCertFile != "" {
		slog.Info("TLS enabled")
	}

	// Graceful shutdown on SIGINT/SIGTERM.
	go func() {
		ch := make(chan os.Signal, 1)
		signal.Notify(ch, syscall.SIGINT, syscall.SIGTERM)
		<-ch
		slog.Info("shutting down...")
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = srv.Shutdown(ctx)
	}()

	if cfg.TLSCertFile != "" && cfg.TLSKeyFile != "" {
		if err := srv.ListenAndServeTLS(cfg.TLSCertFile, cfg.TLSKeyFile); err != nil && err != http.ErrServerClosed {
			slog.Error("server error", "error", err)
			os.Exit(1)
		}
	} else {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("server error", "error", err)
			os.Exit(1)
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
		slog.Warn("could not persist secret", "path", path, "error", err)
	}
	return s
}

func mustGenerateHex(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		slog.Error("rand failed", "error", err)
		os.Exit(1)
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
