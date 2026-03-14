# API endpoints

This document describes the HTTP and WebSocket endpoints exposed by sear-daemon.

## Authentication model

- Public endpoints: no auth required
- Client endpoints: JWT bearer token
- Root endpoints: HTTP Basic auth (`root` + root password)
- Artifacts endpoints: accepts either client JWT or root Basic auth

## Public endpoints

- `GET /healthz`
  - Returns `200 OK` with `ok` body

- `POST /api/v1/register`
  - Registers (or re-registers) a client
  - Validated by `registration_secret`
  - Returns client ID and JWT token

- `GET /api/v1/ws`
  - Upgrades to WebSocket for client control/log stream
  - Auth via bearer token or `?token=` query parameter

## Root endpoints

Root management APIs require HTTP Basic auth.

### UI pages

UI pages are served under `/ui`:

- `GET /ui`
  - Clients/status dashboard UI

- `GET /ui/secrets`
  - Secrets management UI

- `GET /ui/playbooks`
  - Playbooks management UI

- `GET /ui/deployments`
  - Deployments and logs UI

The UI signs in and then calls the root APIs below using HTTP Basic credentials.

### Status API

- `GET /api/v1/status`
  - JSON summary of clients and deployments

### Playbooks

- `GET /api/v1/playbooks`
  - List all playbooks

- `POST /api/v1/playbooks`
  - Create playbook
  - Accepts JSON payload with either:
    - `playbook` (JSON object), or
    - `playbook_yaml` (string)

- `GET /api/v1/playbooks/{id}`
  - Get a playbook
  - Includes `playbook_yaml` in response for editor-friendly roundtrips

- `PUT /api/v1/playbooks/{id}`
  - Update playbook
  - Accepts `playbook` or `playbook_yaml` (same as create)

- `DELETE /api/v1/playbooks/{id}`
  - Delete playbook

- `POST /api/v1/playbooks/{id}/assign`
  - Assign playbook to a client
  - If client is connected, deployment is pushed immediately

### Clients

- `GET /api/v1/clients`
  - List clients

- `GET /api/v1/clients/{id}`
  - Get client

- `PUT /api/v1/clients/{id}`
  - Update client fields such as `playbook_id` or `status`

- `DELETE /api/v1/clients/{id}`
  - Delete client

### Deployments

- `GET /api/v1/deployments`
  - List deployments

- `GET /api/v1/deployments/{id}`
  - Get deployment details

- `GET /api/v1/deployments/{id}/logs`
  - Get deployment log entries

### Secrets

- `GET /api/v1/secrets`
  - List secret names

- `GET /api/v1/secrets/{name}`
  - Get secret value

- `PUT /api/v1/secrets/{name}`
  - Set or update secret value

- `DELETE /api/v1/secrets/{name}`
  - Delete secret

## Artifacts endpoints

These endpoints accept either client JWT auth or root Basic auth.

- `GET /artifacts`
  - List artifact metadata

- `POST /artifacts?name=<filename>`
  - Upload artifact from raw request body

- `GET /artifacts/{id}`
  - Download artifact by ID

- `GET /artifacts/{name}`
  - Download artifact by name (fallback lookup)

- `GET /artifacts/{id}/meta`
  - Get artifact metadata only

- `DELETE /artifacts/{id}`
  - Delete artifact