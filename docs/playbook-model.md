## kompakt-agent Configuration (client.config.yml)

The kompakt-agent is configured via a YAML file. Below are the supported options:

| Key                      | Required | Type    | Description                                                                                 |
|--------------------------|----------|---------|---------------------------------------------------------------------------------------------|
| server_url               | Yes      | string  | Base URL of the kompakt server (e.g. "https://localhost:8443"). Use https:// if TLS is on. |
| disable_tls_verification | No       | bool    | Set to true to skip TLS certificate verification (for self-signed/dev only).                |
| registration_secret      | Yes      | string  | Must match one of the daemon's registration_secrets values.                                 |
| state_file               | No       | string  | Path to persist agent token and resume position.                                            |
| work_dir                 | No       | string  | Directory for shell steps and temp files.                                                   |
| reconnect_interval_seconds| No      | int     | Seconds to wait before retrying a failed WebSocket connection (default: 10).                |
| log_batch_size           | No       | int     | Max log lines buffered before forced flush (default: 100).                                  |

Example:

```yaml
server_url: "https://localhost:8443"
disable_tls_verification: true
registration_secret: "replace-with-a-strong-random-value"
# state_file: "/var/lib/kompakt/state.json"
# work_dir: "/tmp/kompakt-agent-work"
# reconnect_interval_seconds: 10
# log_batch_size: 100
```
# Playbook Model

This document describes the structure and features of Kompakt playbooks for deployment automation.

## Overview

A playbook defines a repeatable deployment workflow as a sequence of jobs and steps, with support for environment variables, secrets, artifact distribution, and reboot-safe execution.

## Structure

- **name**: Human-readable playbook name.
- **env**: Playbook-level environment variables (available in all steps).
- **jobs**: Ordered list of jobs, each with a name and steps.

### Example

```yaml
name: "Production App Deployment"
env:
  APP_VERSION: "3.1.0"
  DEPLOY_ENV:  "production"
jobs:
  - name: "os-prep"
    steps:
      - name: "Install dependencies"
        shell: bash
        run: |
          apt-get update -qq
          apt-get install -y nginx curl jq
      - name: "Configure sysctl"
        shell: bash
        run: |
          echo "vm.swappiness=10" >> /etc/sysctl.d/99-kompakt.conf
      - name: "Reboot to apply kernel settings"
        uses: reboot
        with:
          reason: "Apply sysctl and kernel parameters"
  - name: "app-install"
    steps:
      - name: "Download application binary"
        uses: download-artifact
        with:
          artifact: "myapp-v3.1.0"
          path: /opt/myapp
```

## Features

- **Ordered jobs and steps**: Jobs execute in the order listed; steps within each job are run sequentially.
- **Environment injection**: Use `env` for variables available in all steps; step-level `env` can override.
- **Secrets**: Inject secrets into steps using `${{ secrets.NAME }}` syntax.
- **Artifact distribution**: Steps can use `download-artifact` to fetch files from the daemon.
- **Reboot-safe execution**: If a reboot step is used, the client resumes from the next job after reboot.
- **Timeouts**: Steps can specify `timeout-minutes` to limit execution time.

## Step Types

- **Shell steps**: Run shell commands (`shell: bash` or `shell: powershell`).
- **Built-in actions**: Use `uses:` for built-in actions like `reboot` or `download-artifact`.

## Best Practices

- Use descriptive job and step names for clarity.
- Group related steps into jobs for logical phases.
- Use environment variables and secrets to avoid hardcoding sensitive values.
- Use artifact distribution for binaries and config files.
- Use reboot steps for kernel or OS changes that require restart.

## References

- [Example playbook](../examples/playbook.yml)
- [API endpoints](api-endpoints.md)
