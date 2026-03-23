# Kompakt Project Structure

Welcome to the Kompakt codebase! This document serves as your guide to the project's layout and core components.

## 🏗️ High-Level Architecture

Kompakt is a two-binary system designed for resilient deployment automation:

- **`kompakt` (Daemon)**: The central command plane. It manages playbooks, secrets, artifacts, and tracks the state of all deployments. It exposes an HTTP API and a Web Dashboard.
- **`kompakt-agent` (Agent)**: The execution unit installed on target nodes. It connects to the daemon via WebSocket, receives instructions, executes playbooks, and streams logs back in real-time.

---

## 📂 Directory Layout

### Root Directories
- [**`cmd/`**](../cmd): Entry points for the two main binaries (`kompakt` and `kompakt-agent`).
- [**`internal/`**](../internal): The heart of the project. Contains all core logic.
- [**`docs/`**](../docs): Technical documentation and design specs.
- [**`examples/`**](../examples): Reference YAML files for configurations and playbooks.

### Core Packages (`internal/`)
- [**`internal/server/`**](../internal/server):
    - `handlers/`: HTTP/WebSocket endpoints (Auth, Playbooks, Agents).
    - `service/`: Business logic for deployment dispatch and management.
    - `store/`: SQLite-backed persistence layer.
- [**`internal/agent/`**](../internal/agent):
    - `executor/`: The engine that runs individual playbook steps (Shell, Action, Reboot).
    - `identity/`: Logic for uniquely identifying and registering nodes.
- [**`internal/common/`**](../internal/common): Shared data models, configuration loaders, and constants.

---

## 🛠️ Development Workflow

### Build System
The project uses a standard Go toolchain and a `Makefile` for convenience.
- `make build`: Compiles binaries into `bin/`.
- `make test`: Runs all unit and integration tests.

### Configuration
- **Daemon**: Configured via `config.yml` (server settings) and `secrets.yml` (credentials).
- **Agent**: Configured via `client.config.yml`.
- On first run, the daemon auto-generates root credentials if they are missing.

---

## 🚀 Getting Started
1. Run `make build` to prepare your binaries.
2. Start the daemon: `./bin/kompakt -config examples/config.yml`.
3. Start an agent: `./bin/kompakt-agent -config examples/client.config.yml`.
4. Navigate to `http://localhost:8080/ui` to see the dashboard.
