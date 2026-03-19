# API endpoints

This document describes the HTTP and WebSocket endpoints exposed by kompakt.

## Authentication model

| Caller | Mechanism |
| --- | --- |
| Root / admin (direct API) | HTTP Basic auth (`root` + root password) |
| Root / admin (web UI) | Short-lived Bearer JWT issued by `POST /api/v1/ui/login` |
| Agent | Bearer JWT issued at `POST /api/v1/register` |
| Artifacts | Accepts either agent JWT or root Basic/Bearer |

Root-protected endpoints accept both HTTP Basic and the UI JWT interchangeably.

> Daemon generates missing credentials on startup and prints them for onboarding.

## Public endpoints

- `GET /healthz`
  - Returns `200 OK` with `ok` body

- `POST /api/v1/register`
  - Registers (or re-registers) a client
  - Validated by `registration_secret` (must match daemon config)
  - Returns client ID and agent JWT

- `POST /api/v1/ui/login`
  - Validates root password, returns a short-lived Bearer JWT for UI sessions
  - Body: `{ "password": "..." }`

- `GET /api/v1/ws`
  - Upgrades to WebSocket for agent control and log streaming
  - Auth via `Authorization: Bearer <token>` or `?token=` query param

## Root endpoints

Require HTTP Basic auth (`root` + root password) or UI JWT Bearer token.

### UI pages

Served under `/ui` — no auth required at the HTTP level; page JS handles auth for API calls.

- `GET /ui` — homepage dashboard
- `GET /ui/clients` — connected clients
- `GET /ui/vault` — secrets management
- `GET /ui/playbooks` — playbook library
- `GET /ui/deployments` — deployment history and logs
- `GET /ui/artifacts` — artifact storage

### Status

- `GET /api/v1/status`
  - JSON snapshot of all clients and active deployments

### Playbooks

- `GET /api/v1/playbooks` — list all playbooks
- `POST /api/v1/playbooks` — create playbook (JSON or YAML body)
- `GET /api/v1/playbooks/{id}` — get playbook (includes YAML for editor round-trips)
- `PUT /api/v1/playbooks/{id}` — update playbook
- `DELETE /api/v1/playbooks/{id}` — delete playbook
- `POST /api/v1/playbooks/{id}/assign` — assign playbook to a client; pushes deployment immediately if client is connected

### Clients

- `GET /api/v1/clients` — list all registered clients
- `DELETE /api/v1/clients/{id}` — remove a client record

### Deployments

- `GET /api/v1/deployments` — list all deployments
- `DELETE /api/v1/deployments/{id}` — remove a deployment and its logs
- `GET /api/v1/deployments/{id}/logs` — get persisted log entries for a deployment

### Artifacts

Artifacts use the `/artifacts` prefix (not `/api/v1`). Access is controlled per artifact by its access policy (public, authenticated, restricted).

- `GET /artifacts` — list all artifacts
- `POST /artifacts?filename=<f>&name=<n>&access_policy=<p>&allowed_clients=<ids>` — upload artifact (raw body)
- `GET /artifacts/{id}` — download artifact
- `PATCH /artifacts/{id}?access_policy=<p>&allowed_clients=<ids>` — update access policy
- `DELETE /artifacts/{id}` — delete artifact

### Secrets

- `GET /api/v1/secrets` — list secret names (values are not returned)
- `GET /api/v1/secrets/{name}` — get a single secret value
- `PUT /api/v1/secrets/{name}` — create or update a secret; body: `{ "value": "..." }`
- `DELETE /api/v1/secrets/{name}` — delete a secret

## Notes

- All state (agents, deployments, playbooks, artifacts, secrets, logs) is persisted in a single SQLite database (`kompakt.db`) in the server's data directory.
- Agents resume deployments automatically after reboot or crash using the stored `resume_step_index`.
- Secrets are injected into playbook steps via `${{ secrets.NAME }}` at execution time.
