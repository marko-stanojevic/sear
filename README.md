# Kompakt

[![CI](https://github.com/marko-stanojevic/kompakt/actions/workflows/ci.yml/badge.svg?branch=main)](https://github.com/marko-stanojevic/kompakt/actions/workflows/ci.yml)
[![Release](https://github.com/marko-stanojevic/kompakt/actions/workflows/release.yml/badge.svg)](https://github.com/marko-stanojevic/kompakt/actions/workflows/release.yml)
[![Coverage](https://codecov.io/gh/marko-stanojevic/kompakt/graph/badge.svg?branch=main)](https://codecov.io/gh/marko-stanojevic/kompakt)
[![Go Version](https://img.shields.io/github/go-mod/go-version/marko-stanojevic/kompakt)](https://github.com/marko-stanojevic/kompakt/blob/main/go.mod)
[![Latest Release](https://img.shields.io/github/v/release/marko-stanojevic/kompakt)](https://github.com/marko-stanojevic/kompakt/releases)
[![Go Report Card](https://goreportcard.com/badge/github.com/marko-stanojevic/kompakt)](https://goreportcard.com/report/github.com/marko-stanojevic/kompakt)
[![License](https://img.shields.io/github/license/marko-stanojevic/kompakt)](LICENSE)

**Deploy to any node. Survive any reboot.**

Kompakt is an agent-based deployment automation tool built for on-premises, bare-metal, and edge environments. A central daemon manages playbooks, secrets, and artifacts. Lightweight agents on each node execute those playbooks step-by-step, stream logs in real time, and resume exactly where they left off after a reboot or crash — no cloud dependency, no orchestrator required.

## Overview

| Component | Role |
| --- | --- |
| `kompakt` | Central command plane — HTTP API, web dashboard, artifact store, durable deployment state |
| `kompakt-agent` | Execution agent — registers with the daemon, runs playbooks, streams logs, survives reboots |

## Use cases

**Bootstrapping bare-metal nodes**
Provision fresh hardware from zero: install OS packages, write config files, place binaries, and reboot mid-playbook without losing your place. Kompakt resumes from the last confirmed step.

**Deploying applications to on-prem fleets**
Push a new version of your service to dozens of nodes in a controlled sequence. Track progress per node in the dashboard, inspect per-step logs, and re-run failed deployments from a single UI.

**Managing edge datacenter infrastructure**
Nodes at remote sites phone home over WebSocket. The daemon queues work; agents pull and execute when connected. No VPN tunnels or inbound firewall rules required on the node side.

**Distributing versioned artifacts**
Upload binaries or configuration archives once to the daemon. Reference them in a playbook step — agents download them at execution time with access-policy enforcement (public, authenticated, or restricted to specific clients).

**Secrets-aware automation**
Store credentials and tokens centrally. Inject them into any playbook step with `${{ secrets.NAME }}` — values never touch disk on the agent and are not logged.

## Features

- **Reboot-safe execution** — a persistent resume index means a node reboot mid-playbook is handled automatically, not an error condition.
- **Real-time telemetry** — step start/complete events and stdout/stderr stream to the daemon over WebSocket and appear live in the dashboard.
- **Centralized artifact distribution** — upload once, reference in any playbook; access policies control which clients can download.
- **Secret injection** — secrets resolved server-side at dispatch time; agents receive values in environment, never in plaintext config.
- **Structured playbooks** — ordered jobs and steps with shell execution, artifact operations, and reboot actions in a single YAML definition.
- **Audit trail** — all deployment logs persisted per deployment; queryable via API and viewable in the UI.
- **Zero external dependencies** — single static Go binary per component, no container runtime, no orchestrator, no database.
- **TLS and JWT out of the box** — agent communication is JWT-authenticated; UI sessions use short-lived tokens; TLS termination is built-in.

## Quick start

### 1. Download binaries

Download prebuilt archives from [GitHub Releases](https://github.com/marko-stanojevic/kompakt/releases) and extract the `kompakt` and `kompakt-agent` binaries for your OS and architecture.

### 2. Start the daemon

```bash
./kompakt -config config.yml -secrets secrets.yml
```

On first run the daemon generates a root password and registration secret, prints them, and writes them to the secrets file. See [`examples/config.yml`](examples/config.yml) and [`examples/secrets.yml`](examples/secrets.yml) for full configuration reference.

### 3. Register a node

Copy `kompakt-agent` to the target node and run:

```bash
./kompakt-agent -config client.config.yml
```

The agent registers with the daemon, establishes a persistent WebSocket connection, and waits for work. See [`examples/client.config.yml`](examples/client.config.yml).

### 4. Deploy a playbook

Open the dashboard at `http://<daemon-host>:8080/ui`, navigate to **Playbooks**, upload a playbook YAML, then assign it to a connected client. Deployment starts immediately and logs stream in real time under **Deployments**.

A minimal playbook looks like:

```yaml
name: hello
jobs:
  - name: verify
    steps:
      - name: print hostname
        shell: bash
        run: hostname && uname -a
```

See [`examples/playbook.yml`](examples/playbook.yml) for a full multi-phase example with reboots and artifact distribution.

## Dashboard

| Page | Path | Purpose |
| --- | --- | --- |
| Home | `/ui` | Fleet summary — clients, deployments, playbooks, artifacts at a glance |
| Clients | `/ui/clients` | Live connection status, platform info, and last activity per node |
| Playbooks | `/ui/playbooks` | Upload, edit, and assign playbooks to clients |
| Deployments | `/ui/deployments` | Execution history, per-step logs, and status for every deployment |
| Artifacts | `/ui/artifacts` | Upload binaries and files; manage access policies |
| Secrets | `/ui/secrets` | Store and rotate credentials used in playbook steps |

## Documentation

- [API endpoints](docs/api-endpoints.md) — full HTTP and WebSocket API reference
- [Playbook model](docs/playbook-model.md) — step types, secret injection, reboot handling, and best practices
- [Project docs](docs/documentation.md) — FAQ and architecture overview

## Troubleshooting

| Symptom | Check |
| --- | --- |
| Agent fails to register | `registration_secret` in agent config must match a value in the daemon secrets file |
| JWT token expired errors | Increase `token_expiry_hours` in daemon config |
| Artifact download fails | Verify the artifact access policy; confirm the client ID is in the allowed list for restricted artifacts |
| Deployment stuck after reboot | Agent resumes automatically on reconnect — check agent logs for connection errors |

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) for development setup, testing, and contribution guidelines.

## License

Kompakt is licensed under the [MIT License](LICENSE).
