# API endpoints

This document describes the HTTP and WebSocket endpoints exposed by kompakt.

## Authentication model

- Public endpoints: no auth required
- Client endpoints: JWT bearer token (issued at registration)
- Root endpoints: HTTP Basic auth (`root` + root password)
- Artifacts endpoints: accepts either client JWT or root Basic auth

> Note: Daemon generates missing secrets and passwords on startup and prints them for onboarding.

## Public endpoints

- `GET /healthz`
  - Returns `200 OK` with `ok` body

- `POST /api/v1/register`
  - Registers (or re-registers) a client
  - Validated by `registration_secret` (must match daemon config)
  - Returns client ID and JWT token

- `GET /api/v1/ws`
  - Upgrades to WebSocket for client control/log stream
  - Auth via bearer token or `?token=` query parameter
  - Streams real-time deployment events and logs

## Root endpoints

Root management APIs require HTTP Basic auth.

### UI pages

UI pages are served under `/ui`:

- `GET /ui` (clients/status dashboard)
- `GET /ui/secrets` (secrets management)
- `GET /ui/playbooks` (playbooks management)
- `GET /ui/deployments` (deployments and logs)

The UI signs in and then calls the root APIs below using HTTP Basic credentials.

### Status API

- `GET /api/v1/status`
  - JSON summary of clients and deployments

### Playbooks

- `GET /api/v1/playbooks` (list all playbooks)
- `POST /api/v1/playbooks` (create playbook; accepts JSON or YAML)
- `GET /api/v1/playbooks/{id}` (get playbook; includes YAML for editor roundtrips)
- `PUT /api/v1/playbooks/{id}` (update playbook)
- `DELETE /api/v1/playbooks/{id}` (delete playbook)
- `POST /api/v1/playbooks/{id}/assign` (assign playbook to client; pushes deployment if connected)

### Clients

- `GET /api/v1/clients` (list all registered clients)
- `GET /api/v1/clients/{id}` (get client details)
- `POST /api/v1/clients/{id}/reboot` (trigger client reboot)

### Artifacts

- `POST /api/v1/artifacts` (upload artifact)
- `GET /api/v1/artifacts/{name}` (download artifact)

### Secrets

- `GET /api/v1/secrets` (list secrets)
- `POST /api/v1/secrets` (add secret)
- `DELETE /api/v1/secrets/{name}` (remove secret)

## Notes

- All deployment logs are persisted in the daemon's logs directory.
- Clients resume deployments after reboot or crash.
- Artifacts are distributed automatically during workflow execution.
