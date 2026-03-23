# Kompakt: Detailed Architecture Deep-Dive

This document provides a technical deep-dive into the core components of Kompakt, explaining how they interact to provide resilient, reboot-safe deployment automation.

## 🖥️ Server Internal Architecture (`internal/server`)

The server is built as a modular system with clear separation between state, logic, and external interfaces.

### 1. Persistence Layer (`store/store.go`)
- **Engine**: Pure Go SQLite (`modernc.org/sqlite`).
- **Schema**: Manages durable state for Agents (metadata, status), Deployments (resume index, error details), Playbooks (YAML content), Secrets (encrypted at rest if configured), and Logs.
- **Key Feature**: The `deployments` table tracks `resume_step_index`, which is the source of truth for where an agent should resume after a reboot.

### 2. Service Layer (`service/manager.go`)
- **Orchestration**: The `Manager` is the brain of the server. It makes high-level decisions, such as:
    - Should we start a new deployment or resume an existing one?
    - Which secrets should be injected into this specific agent's playbook?
- **Push Mechanism**: When a playbook is assigned or an agent connects, the `Manager` uses the `Hub` to push the workload over the active WebSocket.

### 3. Communication Hub (`handlers/ws.go`)
- **WebSocket Protocol**: Built on `github.com/coder/websocket`.
- **Bidirectional Streaming**:
    - **Downstream**: Server pushes Playbooks and remote commands to Agents.
    - **Upstream**: Agents stream Logs, Step Start/Complete/Failed events, and Reboot notifications.
- **State Synchronization**: Every step-complete event from the agent triggers an immediate update to the `resume_step_index` in the database.

---

## 🤖 Agent Internal Architecture (`internal/agent`)

The agent is designed to be a "dumb" executor with "smart" lifecycle management.

### 1. Run Loop & Lifecycle (`agent.go`)
- **Registration**: On startup, the agent collects hardware identity (Hostname, OS, Model, Vendor) and registers with the server using a pre-shared `registration_secret`.
- **State Persistence**: The agent saves its `AgentID` and `Token` to a local `state.json`. This allows it to survive reboots and reconnect as the same identity.
- **Connection Management**: A robust loop handles automatic reconnection with exponential backoff if the server is unreachable.

### 2. Execution Engine (`executor/executor.go`)
- **Step Dispatcher**: Routes steps to specific logic based on the `uses` or `run` fields.
- **Supported Action Types**:
    - `reboot`: Signals the agent to notify the server and trigger a system reboot.
    - `download-artifact`: Fetches versioned binaries from the server's artifact store.
    - `upload-artifact`: Pushes local files (e.g., build results) back to the server.
- **Shell Runner**: Supports `bash`, `sh`, `pwsh`, `cmd`, and `python`. It captures both stdout and stderr, sanitizes ANSI escape codes, and streams lines back to the server in real-time.

---

## 🔄 The Deployment Lifecycle (Step-by-Step)

1. **Assignment**: Operator assigns a playbook to an agent via the UI.
2. **Dispatch**: `Manager` creates a `DeploymentState` (index 0) and pushes the `Playbook` + `Secrets` over WS.
3. **Execution**: Agent enters a loop, running each step.
4. **Telemetry**: Agent sends `WSMsgStepStart`, then streams log lines, then sends `WSMsgStepComplete`.
5. **Reboot (Optional)**: If a step requests a reboot:
    - Agent sends `WSMsgReboot` (with the next step index).
    - Server marks deployment as `Rebooting`.
    - Agent reboots the host OS.
6. **Resumption**: After reboot, Agent reconnects. `Manager` sees the `Rebooting` status and pushes the playbook again, starting from the recorded `resume_step_index`.
7. **Completion**: Once all steps finish, Agent sends `WSMsgDeployDone`.
