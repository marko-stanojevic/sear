# Kompakt

[![CI](https://github.com/marko-stanojevic/kompakt/actions/workflows/ci.yml/badge.svg?branch=main)](https://github.com/marko-stanojevic/kompakt/actions/workflows/ci.yml)
[![Release](https://github.com/marko-stanojevic/kompakt/actions/workflows/release.yml/badge.svg)](https://github.com/marko-stanojevic/kompakt/actions/workflows/release.yml)
[![Latest Release](https://img.shields.io/github/v/release/marko-stanojevic/kompakt)](https://github.com/marko-stanojevic/kompakt/releases/latest)
[![Coverage](https://codecov.io/gh/marko-stanojevic/kompakt/graph/badge.svg?branch=main)](https://codecov.io/gh/marko-stanojevic/kompakt)
[![Go Version](https://img.shields.io/github/go-mod/go-version/marko-stanojevic/kompakt)](https://github.com/marko-stanojevic/kompakt/blob/main/go.mod)
[![Go Report Card](https://goreportcard.com/badge/github.com/marko-stanojevic/kompakt)](https://goreportcard.com/report/github.com/marko-stanojevic/kompakt)
[![Status](https://img.shields.io/badge/status-alpha-orange)](https://github.com/marko-stanojevic/kompakt)
[![License](https://img.shields.io/github/license/marko-stanojevic/kompakt)](LICENSE)

**Deploy to any node. Survive any reboot.**

Kompakt is a resilient, agent-based deployment automation tool designed for on-premises, bare-metal, and edge environments. It ensures that complex multi-step deployments complete reliably, even across hardware reboots and network outages.

---

## Why Kompakt?

Traditional orchestration struggles at the “last mile.” Kompakt changes that:

- **No more KVM headaches**: Skip slow consoles and manual passwords.
- **Edge-ready**: Deploy anywhere, even without PXE, vaults, or artifact stores.
- **Multi-Stage Provisioning**: Orchestrate complex workflows across reboots, preserving state at every step.
- **Secrets handled securely**: Inject passwords and keys at runtime, never in the OS image.
- **Configurable playbooks & ISOs**: The right recipe for every hardware and OS.

Kompakt brings the full stack to the edge, turning tedious tasks into fast, repeatable processes.

---

## Architecture

```mermaid
sequenceDiagram
    participant Agent as kompakt Agent
    participant Server as kompakt Server

    Note over Agent, Server: Phase 1: Registration
    Agent->>Server: Register (Hostname, HW ID, Secret)
    Server->>Agent: Issue Opaque Token & Identity

    Note over Agent, Server: Phase 2: Live Connection
    Agent->>Server: Establish Outbound WebSocket (Secure)
    Server-->>Agent: Connection Idle (Heartbeat)

    Note over Agent, Server: Phase 3: Deployment Execution
    Server->>Agent: Push Workload (Playbook + Encrypted Secrets)
    Agent->>Server: Step Start Notification
    Agent->>Server: Stream real-time logs (STDOUT/STDERR)
    Agent->>Server: Step Complete / Progress Sync

    Note over Agent, Server: Phase 4: Resilience
    Agent->>Server: Reboot Signal (Current State Persisted)
    Note over Agent: Hardware Reboot Cycle
    Agent->>Server: Reconnect (Opaque Auth)
    Server->>Agent: Resume Playbook from Last Step
```

---

## Core Features

| Feature | Description |
| :--- | :--- |
| **Tiered Multi-Stage Deployments** | Orchestrate complex workflows from Live CD installation to final OS configuration on first boot or multiple reboots. |
| **Integrated Credential Vault** | Securely manage and inject secrets via `${{ secrets.NAME }}` without ever touching the agent's disk. |
| **Centralized Artifact Storage** | Efficiently distribute binaries and blobs with granular, agent-specific access policies. |
| **Live Remote Command Execution** | Execute ad-hoc commands with real-time feedback for rapid troubleshooting and development. |
| **Centralized Audit Logs** | Stream and store all agent execution logs centrally for a complete, immutable audit trail. |
| **Automated ISO Builder** | Create custom bootable Linux or WinPE media with the Kompakt Agent pre-integrated. |
| **Resilient Reboot Lifecycle** | Survive power cycles and intentional restarts with synchronized state on both server and agent. |
| **Zero External Dependencies** | Statically linked Go binaries with no requirement for Docker, Python, or external runtimes. |

---

## Project Resources

- [Architecture Deep Dive](docs/architecture-deep-dive.md)
- [API Documentation](docs/api-endpoints.md)
- [Playbook Model & Step Types](docs/playbook-model.md)
- [Agent Authentication & Security](docs/agent-authentication.md)
- [Onboarding Guide](docs/onboarding.md)
- [General Documentation](docs/documentation.md)

---

### License
Kompakt is licensed under the [MIT License](LICENSE).
