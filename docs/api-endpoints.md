# API endpoints

This document describes the HTTP and WebSocket endpoints exposed by sear-daemon.

## Authentication model

- Public endpoints: no auth required
- Client endpoints: JWT bearer token
- Admin endpoints: HTTP Basic auth (`admin` + root password)
- Artifacts endpoints: accepts either client JWT or admin Basic auth

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

## Admin endpoints

All admin endpoints require HTTP Basic auth.

### Status

- `GET /status`
  - JSON summary of clients and deployments

- `GET /status/ui`
  - Live HTML dashboard

### Playbooks

- `GET /admin/playbooks`
  - List all playbooks

- `POST /admin/playbooks`
  - Create playbook

- `GET /admin/playbooks/{id}`
  - Get a playbook

- `PUT /admin/playbooks/{id}`
  - Update playbook

- `DELETE /admin/playbooks/{id}`
  - Delete playbook

- `POST /admin/playbooks/{id}/assign`
  - Assign playbook to a client
  - If client is connected, deployment is pushed immediately

### Clients

- `GET /admin/clients`
  - List clients

- `GET /admin/clients/{id}`
  - Get client

- `PUT /admin/clients/{id}`
  - Update client fields such as `playbook_id` or `status`

- `DELETE /admin/clients/{id}`
  - Delete client

### Deployments

- `GET /admin/deployments`
  - List deployments

- `GET /admin/deployments/{id}`
  - Get deployment details

- `GET /admin/deployments/{id}/logs`
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

These endpoints accept either client JWT auth or admin Basic auth.

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