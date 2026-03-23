# AGENTS.md

This file provides guidance to AI assistants (like Claude Code, GitHub Copilot, or Antigravity) when working with code in this repository. It defines **rules, architecture, behavior, coding conventions, and core skills**.

## Skills & Capabilities

When generating code or reasoning about this repository, assistants should demonstrate:

- **Distributed systems thinking** – agent ↔ server model, event-driven systems
- **Network resilience** – retries, exponential backoff, reconnection
- **Go concurrency** – goroutines, channels, context propagation
- **Systems programming** – process execution, OS interaction, resource management
- **Secure coding** – input validation, secret handling, TLS
- **API and protocol design** – WebSocket, message-driven architecture
- **Reliability engineering** – structured logging, error handling, fault tolerance
- **Deterministic testing** – table-driven tests, subtests, mocking
- **Minimal environment awareness** – Alpine Linux, BusyBox, static binaries

### Bias

- **Prefer**: simple, explicit, resilient, restart-safe logic
- **Avoid**: complex abstractions, heavy dependencies, assumptions about full Linux environments

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
go test ./internal/server/handlers/... -v -count=1

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

- **`cmd/kompakt`** — the central server/daemon. Exposes an HTTP API and web UI. Manages agents, playbooks, deployments, artifacts, and secrets via a SQLite-backed store (`internal/server/store`).
- **`cmd/kompakt-agent`** — the edge execution agent. Registers with the daemon, receives playbooks over WebSocket, executes them step-by-step, and streams logs back.

### Layered Architecture
- **Entrypoints**: `cmd/`
- **Business Logic**: `internal/server/service/` (server) and `internal/agent/` (agent)
- **Data Access**: `internal/server/store/` (repository pattern)
- **Transport**: `internal/server/handlers/` (HTTP/WebSocket) and `internal/agent/` (WS client)
- **Separation of Concerns**: Do not mix concerns across layers (e.g., transport logic stays in handlers; business logic stays in services).

### Key packages

| Package | Role |
| --- | --- |
| `internal/common` | Shared types, config loading, playbook model, secret resolution |
| `internal/server` | HTTP server wiring, route definitions |
| `internal/server/handlers` | All HTTP/WebSocket handlers; auth middleware (JWT for agents, HTTP Basic for root) |
| `internal/server/service` | Business logic (deployment dispatch, logic for resuming vs starting new) |
| `internal/server/store` | SQLite persistence for all server state (`kompakt.db`) |
| `internal/server/handlers/ui` | Web dashboard assets and HTML templates |
| `internal/agent` | Agent run loop: registration, WebSocket connect/reconnect, playbook execution |
| `internal/agent/executor` | Step execution engine (shell, reboot, artifact actions) |
| `internal/agent/identity` | Hardware/platform identifier collection for registration |

## Core Principles
- **Reliability over performance**: Deployments must be deterministic and resume-friendly.
- **Simplicity over abstraction**: Code should be readable even without deep knowledge of the framework.
- **Explicit behavior over implicit magic**: No hidden side effects or auto-magic configurations.

## Architecture Rules
- **Business logic**: Agent-side logic resides in `internal/agent`, while execution logic is in `internal/agent/executor`.
- **Communication**: All external connectivity is managed via `github.com/coder/websocket` in the agent's main loop.
- **Configuration**: Loaded from `client.config.yml`. The agent is designed to be "dumb" and receive its workload from the server.
- **State**: The agent is mostly stateless but persists its identity (`AgentID` and `Token`) in a `state.json` to survive reboots.

## Agent Loop Contract

The agent main loop must:
1. **Load configuration**: From `client.config.yml`.
2. **Register (if needed)**: Obtain `AgentID` and `Token` from the server.
3. **Establish WebSocket connection**: Persistent connection for real-time communication.
4. **Enter receive → execute → report loop**: Receive playbooks, execute steps, and stream telemetry back.
5. **Reconnect on failure with backoff**: Maintain connectivity across network/server outages.

> [!IMPORTANT]
> **Resilience**: The agent loop must **never exit** on transient errors (network loss, 5xx server errors). It must retry indefinitely with exponential backoff.

## DO / DO NOT

### DO
- Use `context.Context` for all request-scoped operations
- Keep functions small and composable
- Return early on errors
- Use structured logging with `log/slog`

### DO NOT
- Do not introduce global mutable state
- Do not call external services directly from handlers (keep transport separate from logic)
- Do not ignore errors
- Do not use third-party libraries unless explicitly required
- Do not use CGO-based dependencies

## Agent Behavior
- **Automatic Reconnection**: The agent must maintain connectivity and reconnect with exponential backoff if lost.
- **Resilience**: Network failures or server downtimes must not cause the agent to crash.
- **Streaming Telemetry**: Logs and step statuses must be streamed in real-time as they occur.

## Boot Environment (Important)
- **Minimal OS Compatibility**: The agent is designed to run in minimal environments (Alpine Linux, BusyBox, minimal ISOs).
- **No System dependencies**: Avoid dependencies on `systemd` or heavy OS features that aren't available in minimal build environments.

## Security
- **Never log secrets**: Ensure `${{ secrets.NAME }}` expansion is never leaked into logs.
- **Untrusted Network**: Assume the management network may be untrusted; all communication is TLS-secured.
- **Validated Input**: All instructions received over WebSocket must be validated before execution.

## Anti-patterns
- **No CGO**: Maintain cross-compilability without a C toolchain.
- **No hidden side effects**: Every file modification or system change must be a tracked playbook step.
- **No blocking without timeout**: All network or shell operations must have a sensible timeout.

### Coding Conventions

- **Logging**:
  - Use `log/slog` for structured logging.
  - Logs must include context (e.g., `agentID`, `deploymentID`, or request metadata).
  - **Never log secrets**: Ensure `${{ secrets.NAME }}` expansion is never leaked into logs.
- **SQL**: Use pure Go SQLite (`modernc.org/sqlite`); avoid CGO dependencies.
- **WebSocket**: Use `github.com/coder/websocket` for agent communication.
- **State**: The server is the source of truth; agents stream telemetry (step start/complete) to sync state.
- **Testing**:
  - Use table-driven tests.
  - Use subtests with `t.Run`.
  - Mock external dependencies.
  - Tests must be deterministic.
- **Integration Tests**: Use `//go:build integration` tags for tests in `integration_*.go` files. Run them with `go test ./... -tags=integration`.
- **Concurrency**:
  - Prefer channels or well-scoped goroutines for asynchronous logic.
  - Avoid shared mutable state (use mutexes only when absolutely necessary).
  - Ensure goroutines terminate properly using `context.Context` or closing channels.
- **Context Usage**:
  - Always pass `context.Context` as the first argument.
  - Never store context in structs.
- **Interfaces**:
  - Define interfaces at the consumer side, not the producer.
  - Keep interfaces small (1–3 methods).
  - Avoid premature abstraction.
- **Error Handling**:
  - Always handle errors explicitly.
  - Wrap errors using `fmt.Errorf("context: %w", err)` to maintain underlying error type.
  - Do not use `panic` except for unrecoverable startup errors.
- **General**:
  - Use idiomatic Go.
  - Keep code simple, explicit, and maintainable.
  - Avoid unnecessary abstractions.

- **Retry & Backoff**:
  - All network operations must implement retries with exponential backoff.
  - Backoff must include jitter to prevent thundering herd issues.
  - Retries must respect `context.Context` cancellation.
  - Do not retry non-recoverable errors (e.g., 401 Unauthorized, 404 Not Found, unless specific to the logic).
- **Timeouts**:
  - All network or shell operations must have a sensible timeout.
  - Default timeout: 10–30 seconds unless otherwise specified.
  - Long-running operations (like agent execution) must be cancellable via `context.Context`.

### Dependencies
- Prefer standard library.
- Avoid adding dependencies unless necessary.

### Style
- Follow `gofmt` and `go vet`.
- Keep functions small and focused.
- Prefer explicit over clever code.

> [!IMPORTANT]
> **No CGO**: This project must remain cross-compilable without a C toolchain.
> **Schema Management**: Database migrations are handled manually in `internal/server/store/store.go`.

## Module path

`github.com/marko-stanojevic/kompakt`

## Custom Assistant Workflows

This repository includes structured workflows to help AI assistants perform common tasks correctly. Use these as a reference or follow them step-by-step:

- [.agents/workflows/add-api-endpoint.md](.agents/workflows/add-api-endpoint.md): How to add a new REST or WebSocket API endpoint to the server.
- [.agents/workflows/add-step-type.md](.agents/workflows/add-step-type.md): How to add a new playbook step type (action) to the agent.
- [.agents/workflows/add-ui-page.md](.agents/workflows/add-ui-page.md): How to add a new dashboard page (view) to the UI.
