# Copilot Instructions for the Sear Project

## Project Overview

**Sear** is a portable client-server framework written in Go, designed for bootstrapping edge datacenters and on-prem hardware. It executes GitHub Actions-style YAML workflows that persist across system reboots, with extensible client registration and a central dashboard for real-time deployment monitoring.

- **Module path**: `github.com/marko-stanojevic/sear`
- **Language**: Go (no CGo dependencies)
- **Key dependencies**: `gopkg.in/yaml.v3`, `github.com/golang-jwt/jwt/v5`, `github.com/google/uuid`

## Repository Layout

```
sear/
├── cmd/
│   ├── sear-daemon/        # Server/daemon entry point (main.go)
│   └── sear-client/        # Client CLI entry point (main.go)
├── internal/
│   ├── common/             # Shared types used by both daemon and client
│   ├── daemon/
│   │   ├── handlers/       # HTTP/gRPC request handlers for the daemon
│   │   └── store/          # Persistent storage layer
│   └── client/             # Client-side logic and communication
├── go.mod
├── go.sum
├── .github/
│   └── copilot-instructions.md
└── README.md
```

## Build Instructions

Always ensure dependencies are downloaded before building:

```bash
go mod download
```

Build both binaries:

```bash
go build ./cmd/sear-daemon
go build ./cmd/sear-client
```

Or build everything at once:

```bash
go build ./...
```

## Testing

Run all tests:

```bash
go test ./...
```

Run tests with verbose output and race detector:

```bash
go test -race -v ./...
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

## Code Conventions

- Follow standard Go idioms and conventions (effective Go, standard library style).
- Keep `internal/common` free of external dependencies — it holds only shared types.
- The daemon (`cmd/sear-daemon`) is the long-running server process; the client (`cmd/sear-client`) is the short-lived CLI tool.
- Workflow definitions use YAML (via `gopkg.in/yaml.v3`); model them after GitHub Actions syntax.
- Authentication uses JWT (`github.com/golang-jwt/jwt/v5 v5.2.2` — use this minimum version or newer).
- Use `github.com/google/uuid` for generating unique IDs.
- No CGo: keep the codebase portable and cross-compilable.

## Key Architectural Notes

- **Workflows persist across reboots**: the storage layer (`internal/daemon/store`) is responsible for durable state.
- **Client registration**: clients register with the daemon; the daemon assigns them identities (UUIDs) and tracks their state.
- **Dashboard**: real-time monitoring is served by the daemon; keep handler logic in `internal/daemon/handlers`.

## Trust These Instructions

Trust the information in this file. Only search the codebase if the information here is incomplete or appears incorrect.
