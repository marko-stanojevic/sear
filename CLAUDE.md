# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Build & test commands

```bash
# Build both binaries
go build -o bin/kompakt ./cmd/kompakt
go build -o bin/kompakt-agent ./cmd/kompakt-agent
# Or via Make
make build

# Run all tests
go test ./... -v -count=1

# Run tests with race detector
go test ./... -race -count=1

# Run a single package's tests
go test ./internal/daemon/handlers/... -v -count=1

# Lint (requires golangci-lint)
golangci-lint run ./...

# Tidy dependencies
go mod tidy
```

## Run locally

```bash
# Daemon (generates credentials on first run)
go run ./cmd/kompakt -config examples/config.yml -secrets examples/secrets.yml

# Agent
go run ./cmd/kompakt-agent -config examples/client.config.yml
```

## Architecture

The project is a two-binary deployment automation system:

- **`cmd/kompakt`** — the central server/daemon. Exposes an HTTP API and web UI. Manages clients, playbooks, deployments, artifacts, and secrets via a JSON-file-backed store (`internal/daemon/store`).
- **`cmd/kompakt-agent`** — the edge execution agent. Registers with the daemon, receives playbooks over WebSocket, executes them step-by-step, and streams logs back.

### Key packages

| Package | Role |
| --- | --- |
| `internal/common` | Shared types, config loading, playbook model, secret resolution |
| `internal/daemon` | HTTP server wiring, route definitions |
| `internal/daemon/handlers` | All HTTP/WebSocket handlers; auth middleware (JWT for agents, HTTP Basic for root) |
| `internal/daemon/service` | Business logic (deployment dispatch, hub management) |
| `internal/daemon/store` | JSON-file persistence for all daemon state |
| `internal/client` | Agent run loop: registration, WebSocket connect/reconnect, playbook execution |
| `internal/client/executor` | Step execution engine (shell, reboot, download/upload artifact) |
| `internal/client/identity` | Hardware/platform identifier collection for registration |

### Auth model

- **Root/admin**: HTTP Basic auth (`root` + configured root password) for direct `/api/v1/*` API access. The web UI obtains a short-lived JWT from `POST /api/v1/ui/login`; root-protected endpoints accept either Basic auth or that Bearer token.
- **Agent**: JWT Bearer token issued at `/api/v1/register`, passed via `?token=` on WebSocket upgrade.

### Playbook model

Playbooks are YAML with ordered `jobs` (list), each containing `steps`. Steps use either `run` (shell) or `uses` (action like `reboot`, `download-artifact`, `upload-artifact`). Secrets are injected via `${{ secrets.NAME }}`. The daemon tracks a `resume_step_index` per deployment so agents resume from the last confirmed step after reboot.

### Configuration

- Daemon: `config.yml` (paths, TLS, JWT) + `secrets.yml` (root password, registration secrets, client secrets)
- Agent: `client.config.yml` (server URL, registration secret, platform hint, state file path)
- Both configs are optional on first run — the daemon auto-generates missing credentials and prints them.

## Module path

`github.com/marko-stanojevic/kompakt`
