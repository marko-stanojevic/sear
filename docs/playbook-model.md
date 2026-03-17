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
