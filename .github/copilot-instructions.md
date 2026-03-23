# kompakt-agent AI Guidance

This file provides guidance for AI assistants (like GitHub Copilot, Claude, or internal agents) when working with code in this repository. It defines **rules, architecture, behavior, coding conventions, and core skills**.

---

## 1. Purpose

The kompakt-agent repository consists of:

- **cmd/kompakt** — central server/daemon (HTTP API, Web UI, manages agents, playbooks, artifacts, secrets via SQLite).
- **cmd/kompakt-agent** — edge execution agent (registers with server, receives playbooks via WebSocket, executes them step-by-step, streams logs back).

Agents must be **reliable, deterministic, and minimal-environment compatible**.

---

## 2. Skills & Capabilities

When generating code or reasoning about this repository, prefer solutions that demonstrate:

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

- Prefer: simple, explicit, resilient, restart-safe logic  
- Avoid: complex abstractions, heavy dependencies, assumptions about full Linux environments

---

## 3. DO / DO NOT

### DO
- Use `context.Context` for all request-scoped operations
- Keep functions small, composable, readable
- Return early on errors
- Use structured logging with `log/slog`
- Use retry utility with exponential backoff

### DO NOT
- Introduce global mutable state
- Call external services directly from handlers
- Ignore errors
- Use CGO-based dependencies
- Assume full Linux environment

---

## 4. Architecture & Layering

- **Entrypoints:** `cmd/`
- **Business Logic:** `internal/server/service` (server), `internal/agent` (agent)
- **Data Access:** `internal/server/store`
- **Transport:** `internal/server/handlers` (HTTP/WebSocket), `internal/agent` (WS client)
- **Separation of Concerns:** Keep transport, business, and storage layers separate

### Key Packages

| Package | Role |
| --- | --- |
| `internal/common` | Shared types, config, playbook model, secret resolution |
| `internal/server` | HTTP server wiring, routes |
| `internal/server/handlers` | HTTP/WebSocket handlers, auth middleware |
| `internal/server/service` | Business logic |
| `internal/server/store` | SQLite persistence (`kompakt.db`) |
| `internal/server/handlers/ui` | Web dashboard assets/templates |
| `internal/agent` | Agent run loop: registration, WS connect/reconnect, playbook execution |
| `internal/agent/executor` | Step execution engine (shell, reboot, artifact actions) |
| `internal/agent/identity` | Hardware/platform identifier collection |

---

## 5. Agent Behavior

- **Automatic Reconnection:** maintain connectivity with exponential backoff
- **Resilience:** network failures or server downtime must not crash the agent
- **Streaming Telemetry:** log step start/complete events in real-time
- **State Management:** mostly stateless; persists identity (`AgentID`, `Token`) in `state.json` (atomic writes)

---

## 6. Boot Environment

- Minimal OS: Alpine Linux, BusyBox, minimal ISOs
- No systemd or heavy OS features
- All dependencies must work in static-binary or lightweight environments

---

## 7. Security

- Never log secrets (`${{ secrets.NAME }}`)
- Assume untrusted network; all communication is TLS-secured
- Validate all instructions received over WebSocket

---

## 8. Coding Conventions

- **Logging:** `log/slog`, include context (`agentID`, `deploymentID`, request metadata)
- **SQL:** use pure Go SQLite (`modernc.org/sqlite`), avoid CGO
- **WebSocket:** `github.com/coder/websocket` for agent communication
- **Testing:** table-driven tests, subtests, mock dependencies, deterministic
- **Integration tests:** `//go:build integration`, run with `-tags=integration`
- **Concurrency:** goroutines/channels, avoid shared state, terminate properly with `context.Context`
- **Error Handling:** explicit errors, wrap with `fmt.Errorf("context: %w", err)`, only panic for unrecoverable startup
- **Context Usage:** always pass, never store in structs
- **General:** idiomatic Go, simple, explicit, maintainable, avoid unnecessary abstractions

---

## 9. Dependencies & Style

- Prefer standard library, minimal external dependencies
- `gofmt` + `go vet`
- Keep functions small and focused
- Prefer explicit over clever code

---

## 10. Module Path

`github.com/marko-stanojevic/kompakt`

---

## 11. Custom Assistant Workflows

Structured workflows to guide AI or humans:

- [.agents/workflows/add-api-endpoint.md](../.agents/workflows/add-api-endpoint.md) – add REST/WebSocket endpoint
- [.agents/workflows/add-step-type.md](../.agents/workflows/add-step-type.md) – add new playbook step
- [.agents/workflows/add-ui-page.md](../.agents/workflows/add-ui-page.md) – add new dashboard page
