# sear

[![CI](https://github.com/marko-stanojevic/sear/actions/workflows/ci.yml/badge.svg?branch=main)](https://github.com/marko-stanojevic/sear/actions/workflows/ci.yml)
[![Release](https://github.com/marko-stanojevic/sear/actions/workflows/release.yml/badge.svg)](https://github.com/marko-stanojevic/sear/actions/workflows/release.yml)
[![Coverage](https://codecov.io/gh/marko-stanojevic/sear/graph/badge.svg?branch=main)](https://codecov.io/gh/marko-stanojevic/sear)
[![Go Version](https://img.shields.io/github/go-mod/go-version/marko-stanojevic/sear)](https://github.com/marko-stanojevic/sear/blob/main/go.mod)
[![Latest Release](https://img.shields.io/github/v/release/marko-stanojevic/sear)](https://github.com/marko-stanojevic/sear/releases)
[![Go Report Card](https://goreportcard.com/badge/github.com/marko-stanojevic/sear)](https://goreportcard.com/report/github.com/marko-stanojevic/sear)
[![License](https://img.shields.io/github/license/marko-stanojevic/sear)](LICENSE)

Production-grade edge deployment automation in Go for on-prem, bare-metal, and distributed datacenter environments.

Sear helps infrastructure teams roll out and manage repeatable deployments across remote nodes with reboot-safe execution, real-time visibility, and centralized control.

## Platform overview

| Component | Purpose | Core behavior |
| --- | --- | --- |
| sear-daemon | Central command plane | API, dashboard, artifact storage, and durable deployment state |
| sear-client | Execution agent on each node | Registers with daemon, executes playbooks, streams logs, resumes after reboot |
| Workflow engine | Standardized rollout model | Ordered jobs/steps, action-style operations, and secret injection |
| Persistence layer | Operational continuity | Resume index and deployment status survive restarts/reboots |

## Start here

| For operators | For integration | For contributors |
| --- | --- | --- |
| [Download releases](https://github.com/marko-stanojevic/sear/releases) | [API endpoints](docs/api-endpoints.md) | [Contributing guide](CONTRIBUTING.md) |
| [Quick start](#quick-start) | [Playbook model](#playbook-model) | [Project docs](#documentation) |

## Why teams choose sear

- Deterministic rollout behavior with ordered jobs and steps.
- Reboot-safe execution that resumes automatically from the last confirmed step.
- Real-time deployment telemetry over WebSocket (step events and logs).
- Integrated artifact distribution during workflow execution.
- Secret-aware automation with environment injection and `${{ secrets.NAME }}` support.
- Built-in operational visibility through status API and live UI dashboard.
- Portable Go implementation with no CGo dependency.

## Quick start

### 1) Download binaries

Download prebuilt binaries from GitHub Releases:

- https://github.com/marko-stanojevic/sear/releases

Choose the archive for your OS/architecture and extract it.

### 2) Configure daemon and client

Create your config files using the examples in this repository:

- examples/config.yml
- examples/secrets.yml
- examples/client.config.yml

If `root_password` or `registration_secrets` are missing, the daemon generates them on startup and prints them.

### 3) Run daemon

```bash
./sear-daemon -config config.yml -secrets secrets.yml
```

### 4) Run client

```bash
./sear-client -config client.config.yml
```

### 5) Open status dashboard

- URL: `http://localhost:8080/ui`
- UI pages:
	- `/ui` (clients/status)
	- `/ui/secrets`
	- `/ui/playbooks`
	- `/ui/deployments`
- Auth: sign in from the UI (HTTP Basic credentials used for API calls)
- Username: `root`
- Password: value from `root_password` in secrets file (or generated password printed at startup)

## Configuration

Examples are available in `examples/config.yml`, `examples/secrets.yml`, and `examples/client.config.yml`.

### Daemon config fields

- listen_addr
- data_dir
- artifacts_dir
- logs_dir
- tls_cert_file
- tls_key_file
- jwt_secret
- token_expiry_hours

Defaults are applied for unset fields (for example `:8080`, `sear-data`, and 30-day token expiry).

### Daemon secrets fields

- root_password
- registration_secrets (map of named pre-shared registration secrets)
- client_secrets (map injected into playbook steps)

### Client config fields

- server_url (required)
- registration_secret (required)
- platform (`auto` by default)
- state_file (OS-specific default if unset)
- work_dir (OS-specific default if unset)
- reconnect_interval_seconds (default 10)
- log_batch_size (default 100)

## Playbook model

Playbooks are YAML documents with ordered jobs and ordered steps.

Supported step forms:

- `run`: execute shell script (`bash` default, plus `sh`, `pwsh`, `cmd`, `python`)
- `uses: reboot`
- `uses: download-artifact`
- `uses: upload-artifact`
- `uses: upload-logs` (no-op, logs already stream in real time)

Step options:

- `continue-on-error`
- `timeout-minutes`
- step-level `env`

Example playbook: `examples/playbook.yml`

## Documentation

- API endpoints: [docs/api-endpoints.md](docs/api-endpoints.md)
- Contributor and development guide: [CONTRIBUTING.md](CONTRIBUTING.md)

## API route conventions

- Public/client control endpoints are under `/api/v1` (for example `/api/v1/register`, `/api/v1/ws`).
- Root management APIs are also under `/api/v1` (for example `/api/v1/status`, `/api/v1/secrets`, `/api/v1/playbooks`, `/api/v1/clients`, `/api/v1/deployments`).
- Browser pages are under `/ui`.
