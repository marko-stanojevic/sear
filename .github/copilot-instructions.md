# Copilot Instructions for the Kompakt Project

## Project Overview

**Kompakt** is a portable client-server framework written in Go, designed for bootstrapping edge datacenters and on-prem hardware. It executes GitHub Actions-style YAML workflows that persist across system reboots, with extensible client registration and a central dashboard for real-time deployment monitoring.

- **Module path**: `github.com/marko-stanojevic/kompakt`
- **Language**: Go (no CGo dependencies)
- **Key dependencies**: `gopkg.in/yaml.v3`, `github.com/golang-jwt/jwt/v5`, `github.com/google/uuid`, `github.com/gorilla/websocket`

## Repository Layout

```
kompakt/
├── cmd/
│   ├── kompakt/              # Server/daemon entry point (main.go)
│   └── kompakt-agent/        # Client CLI entry point (main.go)
├── internal/
│   ├── common/               # Shared types used by both daemon and client
│   ├── daemon/
│   │   ├── handlers/         # HTTP and WebSocket handlers; auth middleware
│   │   ├── service/          # Business logic (deployment dispatch, hub management)
│   │   └── store/            # JSON-file persistence layer
│   └── client/               # Agent run loop: registration, WebSocket, playbook execution
│       ├── executor/         # Step execution engine (shell, reboot, artifact ops)
│       └── identity/         # Hardware/platform identifier collection
├── examples/
│   ├── config.yml            # Example daemon config
│   ├── secrets.yml           # Example daemon secrets
│   ├── client.config.yml     # Example client config
│   └── playbook.yml          # Example playbook
├── docs/                     # API reference and playbook model docs
├── .goreleaser.yml           # Release packaging config
├── go.mod
├── go.sum
└── .github/
    ├── workflows/            # CI and release workflows
    └── copilot-instructions.md
```

## Build Instructions

Always ensure dependencies are downloaded before building:

```bash
go mod download
```

Build both binaries:

```bash
go build -o bin/kompakt ./cmd/kompakt
go build -o bin/kompakt-agent ./cmd/kompakt-agent
```

Or build everything at once:

```bash
go build ./...
```

Run local examples:

```bash
go run ./cmd/kompakt -config examples/config.yml -secrets examples/secrets.yml
go run ./cmd/kompakt-agent -config examples/client.config.yml
```

## Testing

Run all tests:

```bash
go test ./... -v -count=1
```

Run tests with verbose output and race detector:

```bash
go test ./... -race -count=1
```

Run tests for a specific package:

```bash
go test ./internal/daemon/handlers/...
```

## Linting

Use `go vet` for static analysis (always available with Go toolchain):

```bash
go vet ./...
```

If `golangci-lint` is installed, run:

```bash
golangci-lint run ./...
```

Tidy modules:

```bash
go mod tidy
```

## Release

Local snapshot check:

```bash
go run github.com/goreleaser/goreleaser/v2@v2.14.3 release --snapshot --clean --skip=validate --skip=publish
```

Release artifacts and matrix are configured in `.goreleaser.yml`.

## Code Conventions

- Follow standard Go idioms and conventions (effective Go, standard library style).
- Keep `internal/common` free of external dependencies — it holds only shared types.
- The daemon (`cmd/kompakt`) is the long-running server process; the agent (`cmd/kompakt-agent`) is the short-lived CLI tool.
- Workflow definitions use YAML (via `gopkg.in/yaml.v3`); model them after GitHub Actions syntax.
- Authentication uses JWT (`github.com/golang-jwt/jwt/v5`).
- Use `github.com/google/uuid` for generating unique IDs.
- No CGo: keep the codebase portable and cross-compilable.

## Key Architectural Notes

- **Workflows persist across reboots**: the storage layer (`internal/daemon/store`) is responsible for durable state.
- **Client registration**: clients register with the daemon; the daemon assigns them identities (UUIDs) and tracks their state.
- **Dashboard**: served at `/ui`; individual pages at `/ui/clients`, `/ui/secrets`, `/ui/playbooks`, `/ui/deployments`, `/ui/artifacts`. Keep handler logic in `internal/daemon/handlers`.
- **Auth split**: root API endpoints use HTTP Basic auth or a short-lived UI JWT (issued by `POST /api/v1/ui/login`); client endpoints use JWT Bearer tokens.
- **Logs storage**: deployment logs are persisted per deployment in `logsDir`, not inside `state.json`.

## Available Skills

The repository provides reusable Copilot skills in `.github/skills`:

- `code-interpretation`: Read AI-generated code and understand unfamiliar codebase structure/flow.
- `code-review`: Find bugs, regressions, and missing tests with severity-ranked findings.
- `refactor`: Perform behavior-preserving restructuring and cleanup.
- `security-audit`: Audit for security risks, insecure defaults, and hardening gaps.
- `developer-experience`: Improve local workflow, CI, tasks, and onboarding setup.
- `documentation`: Update documentation from code additions, changes, and deletions.
- `design-compliance`: Verify design patterns, enforce SOLID/clean architecture, detect bad practices, and guide developers.
- `test-driven-development`: Write unit tests, practice TDD red/green/refactor cycles, and close coverage gaps.
- `architecture-awareness`: Understand project architecture, runtime usage, and how to add or modify features safely.

## Trust These Instructions

Trust the information in this file. Only search the codebase if the information here is incomplete or appears incorrect.
