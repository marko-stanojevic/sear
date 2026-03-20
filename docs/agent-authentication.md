# Agent Authentication

Kompakt uses **opaque bearer tokens** for agent-to-server authentication. This document explains the design, lifecycle, and security properties of these tokens.

## Token format

Each token is a 32-byte cryptographically random value encoded as:

```
kpkt_<64 lowercase hex characters>
```

Example: `kpkt_3f2a1b8e...` (64 hex chars after the prefix)

The `kpkt_` prefix makes tokens easy to identify in logs and secret scanners.

## How tokens are stored

The raw token is **never persisted**. Only its SHA-256 hash is written to the `agent_tokens` table in the SQLite database:

```sql
CREATE TABLE agent_tokens (
    id         TEXT PRIMARY KEY,
    agent_id   TEXT NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
    token_hash TEXT NOT NULL UNIQUE,
    created_at TEXT NOT NULL,
    expires_at TEXT,   -- NULL means no expiry
    revoked_at TEXT    -- NULL means active
);
```

This means a database compromise does not expose any usable tokens.

## Lifecycle

### Issuance

Tokens are issued during agent registration (`POST /api/v1/register`). The server returns the raw token **once** — it is never shown again. The agent is responsible for persisting it in its state file.

On re-registration (same agent ID), all previous tokens for that agent are revoked before a new one is issued. This ensures a re-imaged or reconfigured node cannot have stale tokens still accepted.

### Validation

On every authenticated request the server:

1. Extracts the raw token from the `Authorization: Bearer <token>` header or the `?token=` query parameter (used on WebSocket upgrade).
2. Computes `SHA-256(raw token)`.
3. Looks up the hash in `agent_tokens`.
4. Rejects if the token is revoked (`revoked_at IS NOT NULL`) or expired (`expires_at` is in the past).
5. Returns the associated `agent_id` for downstream handler use.

### Rotation

Agents can rotate their token without re-registering via:

```
POST /api/v1/token/refresh
Authorization: Bearer <current-token>
```

The server issues a new token first, then revokes the old one. This guarantees zero-downtime rotation — there is no window where neither token is valid.

### Revocation

Tokens are revoked in three situations:

| Trigger | Scope |
|---|---|
| Agent re-registers | All tokens for that agent |
| Agent is deleted | All tokens (via `ON DELETE CASCADE`) |
| Token rotation | Previous token only |

## Why not JWT?

JWTs were considered and rejected for agent tokens for the following reasons:

**No instant revocation.** A JWT is valid until its `exp` claim expires. Revoking a compromised 30-day agent token with pure JWTs would require either a revocation list (which is effectively a database lookup on every request) or cutting the expiry very short. Opaque tokens get true revocation for free.

**Signing key blast radius.** A leaked JWT signing key compromises every token ever issued. A leaked opaque token compromises only that one token.

**Payload exposure.** JWT payloads are base64-decodable by anyone holding the token. Opaque tokens reveal nothing about the agent or server internals.

**Per-agent metadata.** Opaque tokens support per-token expiry, revocation timestamps, and audit records without cramming mutable state into immutable JWT claims.

Opaque tokens are the approach used by GitHub PATs, AWS access keys, Stripe API keys, and Kubernetes service account tokens — all environments with large numbers of long-lived machine credentials requiring independent revocation.

## UI sessions

The web UI uses a separate, short-lived **JWT** (`exp: 8 hours`) issued at `POST /api/v1/ui/login`. This is intentional: UI sessions are interactive, single-user, and have no lifecycle management requirements. The JWT is validated without a database lookup, which is appropriate at the dashboard's interactive scale.

Agent tokens and UI tokens are completely separate systems.

## Security summary

| Property | Agent token | UI JWT |
|---|---|---|
| Format | Opaque (`kpkt_<hex>`) | JWT (HS256) |
| Storage | SHA-256 hash in SQLite | Not stored |
| Revocation | Instant, per-token | Expiry only (8h max) |
| Expiry | Configurable, per-token | 8 hours fixed |
| DB lookup on auth | Yes | No |
| Compromise blast radius | Single token | All active UI sessions |
