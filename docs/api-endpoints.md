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

All root endpoints require HTTP Basic auth.

### Status

- `GET /status`
  - JSON summary of clients and deployments

- `GET /status/ui`
  - Live HTML dashboard

### Playbooks

- `GET /playbooks`
  - List all playbooks

- `POST /playbooks`
  - Create playbook

- `GET /playbooks/{id}`
  - Get a playbook

- `PUT /playbooks/{id}`
  - Update playbook

- `DELETE /playbooks/{id}`
  - Delete playbook

- `POST /playbooks/{id}/assign`
  - Assign playbook to a client
  - If client is connected, deployment is pushed immediately

### Clients

- `GET /clients`
  - List clients

- `GET /clients/{id}`
  - Get client

- `PUT /clients/{id}`
  - Update client fields such as `playbook_id` or `status`

- `DELETE /clients/{id}`
  - Delete client

### Deployments

- `GET /deployments`
  - List deployments

- `GET /deployments/{id}`
  - Get deployment details

- `GET /deployments/{id}/logs`
  - Get deployment log entries

### Secrets

- `GET /secrets`
  - List secret names

- `GET /secrets/{name}`
  - Get secret value

- `PUT /secrets/{name}`
  - Set or update secret value

- `DELETE /secrets/{name}`
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