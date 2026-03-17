# kompakt

[![CI](https://github.com/marko-stanojevic/kompakt/actions/workflows/ci.yml/badge.svg?branch=main)](https://github.com/marko-stanojevic/kompakt/actions/workflows/ci.yml)
[![Release](https://github.com/marko-stanojevic/kompakt/actions/workflows/release.yml/badge.svg)](https://github.com/marko-stanojevic/kompakt/actions/workflows/release.yml)
[![Coverage](https://codecov.io/gh/marko-stanojevic/kompakt/graph/badge.svg?branch=main)](https://codecov.io/gh/marko-stanojevic/kompakt)
[![Go Version](https://img.shields.io/github/go-mod/go-version/marko-stanojevic/kompakt)](https://github.com/marko-stanojevic/kompakt/blob/main/go.mod)
[![Latest Release](https://img.shields.io/github/v/release/marko-stanojevic/kompakt)](https://github.com/marko-stanojevic/kompakt/releases)
[![Go Report Card](https://goreportcard.com/badge/github.com/marko-stanojevic/kompakt)](https://goreportcard.com/report/github.com/marko-stanojevic/kompakt)
[![License](https://img.shields.io/github/license/marko-stanojevic/kompakt)](LICENSE)

Production-grade edge deployment automation in Go for on-prem, bare-metal, and distributed datacenter environments.

Kompakt helps infrastructure teams roll out and manage repeatable deployments across remote nodes with reboot-safe execution, real-time visibility, and centralized control.

## Platform overview

| Component | Purpose | Core behavior |
| --- | --- | --- |
| kompakt | Central command plane | API, dashboard, artifact storage, and durable deployment state |
| kompakt-agent | Execution agent on each node | Registers with daemon, executes playbooks, streams logs, resumes after reboot |
| Workflow engine | Standardized rollout model | Ordered jobs/steps, action-style operations, and secret injection |
| Persistence layer | Operational continuity | Resume index and deployment status survive restarts/reboots |

## Start here

| For operators | For integration | For contributors |
| --- | --- | --- |
| [Download releases](https://github.com/marko-stanojevic/kompakt/releases) | [API endpoints](docs/api-endpoints.md) | [Contributing guide](CONTRIBUTING.md) |
| [Quick start](#quick-start) | [Playbook model](docs/playbook-model.md) | [Project docs](docs/documentation.md) |

## Why teams choose kompakt

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

- <https://github.com/marko-stanojevic/kompakt/releases>

Choose the archive for your OS/architecture and extract it.

### 2) Configure daemon and agent

Create your config files using the examples in this repository:

- examples/config.yml (daemon config)
- examples/secrets.yml (daemon secrets)
- examples/client.config.yml (agent config)
- examples/playbook.yml (sample workflow)

If `root_password` or `registration_secrets` are missing, the daemon generates them on startup and prints them.

### 3) Run daemon

```bash
./kompakt -config config.yml -secrets secrets.yml
```

### 4) Run agent

```bash
./kompakt-agent -config client.config.yml
```

### 5) Open status dashboard

- URL: `http://localhost:8080/ui`
- UI pages:
	- `/ui` (homepage/dashboard)
	- `/ui/clients` (clients/status)
	- `/ui/secrets` (secrets management)
	- `/ui/playbooks` (playbook library)
	- `/ui/deployments` (deployment history)

## Configuration tips

- Daemon and agent configs are YAML files; see `examples/` for templates.
- Secrets are injected into playbooks using `${{ secrets.NAME }}` syntax.
- Artifacts are distributed automatically during workflow execution.
- All deployment logs are persisted in the daemon's logs directory.
- The agent resumes from the last confirmed step after reboot or crash.

## Troubleshooting

- If the agent cannot register, check `registration_secret` values in both agent and daemon configs.
- If JWT tokens expire, adjust `token_expiry_hours` in daemon config.
- For artifact download errors, verify artifact paths and permissions.

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) for guidelines and workflow.

## License

Kompakt is licensed under the MIT License. See [LICENSE](LICENSE) for details.
