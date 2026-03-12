# sear

[![CI](https://github.com/marko-stanojevic/sear/actions/workflows/ci.yml/badge.svg?branch=main)](https://github.com/marko-stanojevic/sear/actions/workflows/ci.yml)
[![Release](https://github.com/marko-stanojevic/sear/actions/workflows/release.yml/badge.svg)](https://github.com/marko-stanojevic/sear/actions/workflows/release.yml)
[![Coverage](https://codecov.io/gh/marko-stanojevic/sear/graph/badge.svg?branch=main)](https://codecov.io/gh/marko-stanojevic/sear)
[![Go Version](https://img.shields.io/github/go-mod/go-version/marko-stanojevic/sear)](https://github.com/marko-stanojevic/sear/blob/main/go.mod)
[![Latest Release](https://img.shields.io/github/v/release/marko-stanojevic/sear)](https://github.com/marko-stanojevic/sear/releases)
[![Go Report Card](https://goreportcard.com/badge/github.com/marko-stanojevic/sear)](https://goreportcard.com/report/github.com/marko-stanojevic/sear)
[![License](https://img.shields.io/github/license/marko-stanojevic/sear)](LICENSE)

Sear is a portable client-server framework for bootstrapping edge and on-prem infrastructure with YAML workflows.

The project has two binaries:

- sear-daemon: central control plane (API, dashboard, artifact store, persistent state)
- sear-client: node agent that registers, receives playbooks, executes steps, and resumes after reboot

## Key capabilities

- Deterministic workflow execution using ordered jobs and steps.
- Reboot-aware deployments: clients resume from the saved global step index.
- Real-time deployment telemetry over WebSocket (step events and logs).
- Artifact upload/download during workflow execution.
- Secret injection into steps via environment variables and `${{ secrets.NAME }}` syntax.
- Built-in status API and live status UI.
- Portable Go implementation (no CGo required).

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

- URL: `http://localhost:8080/status/ui`
- Auth: HTTP Basic
- Username: `admin`
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
- Contributor and development guide: [CONTRIBUTING.MD](CONTRIBUTING.MD)
