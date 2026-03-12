# Contributing to sear

Thanks for contributing.

This document is for contributors and maintainers. For user-facing setup and usage, see [README.md](README.md).

## Development setup

Prerequisites:

- Go 1.21+
- Optional: golangci-lint

Bootstrap dependencies:

```bash
go mod download
```

Build both binaries:

```bash
go build -o bin/sear-daemon ./cmd/sear-daemon
go build -o bin/sear-client ./cmd/sear-client
```

Or build all packages:

```bash
go build ./...
```

## Run locally

Daemon:

```bash
go run ./cmd/sear-daemon -config examples/config.yml -secrets examples/secrets.yml
```

Client:

```bash
go run ./cmd/sear-client -config examples/client.config.yml
```

## Testing

Run all tests:

```bash
go test ./... -v -count=1
```

Race detector:

```bash
go test ./... -race -count=1
```

Package-scoped run example:

```bash
go test ./internal/daemon/handlers/...
```

## Linting and static checks

Go vet:

```bash
go vet ./...
```

golangci-lint (if installed):

```bash
golangci-lint run ./...
```

Module tidying:

```bash
go mod tidy
```

## VS Code tasks

The workspace includes ready-to-run tasks:

- build
- run daemon (examples)
- run client (examples)
- test
- test-race
- lint
- tidy
- clean

## Repository structure

```text
sear/
  cmd/
    sear-daemon/
    sear-client/
  internal/
    common/
    daemon/
      handlers/
      store/
    client/
      executor/
      identity/
  examples/
  .goreleaser.yml
  .github/workflows/
```

## Code conventions

- Follow standard Go idioms and style.
- Keep internal/common free of external dependencies when possible.
- Daemon endpoints for admins use HTTP Basic auth; client communication uses JWT.
- Logs are persisted per deployment under logsDir and not in state.json.
- Keep job and step order deterministic in playbook processing.

## CI and release

CI workflow:

- [.github/workflows/ci.yml](.github/workflows/ci.yml)

Release workflow:

- [.github/workflows/release.yml](.github/workflows/release.yml)

Local GoReleaser snapshot validation:

```bash
go run github.com/goreleaser/goreleaser/v2@latest release --snapshot --clean --skip=validate --skip=publish
```

Release packaging matrix is defined in [.goreleaser.yml](.goreleaser.yml).

## Documentation expectations

- Keep README focused on operators and users of the tool.
- Put contributor and maintenance guidance in CONTRIBUTING.MD.
- Update examples when changing config schema or workflow behavior.