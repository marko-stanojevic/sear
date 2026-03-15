---
name: developer-experience
description: "Use when setting up or improving development workflow and team DX. Triggers: developer workflow setup, tasks setup, CI/CD pipeline setup, devcontainer setup, VS Code settings setup, onboarding automation, local development environment, workspace tooling standardization. Produces practical setup changes with validation and run instructions."
---

# Developer Experience Skill

## Goal
Create or improve a reliable developer workflow across local development and CI.

## Scope
This skill is for:
- VS Code tasks and workspace automation.
- CI/CD pipelines and validation steps.
- Devcontainer configuration.
- Workspace settings for consistent behavior.
- Onboarding quality-of-life improvements.

## Output Contract
Return results in this order:

1. Setup summary.
2. Files created/updated.
3. Validation steps and commands.
4. Follow-up recommendations.

## Workflow

1. Assess current workflow
- Detect existing tasks, CI files, devcontainer files, and workspace settings.
- Identify gaps in build, test, lint, and run flows.

2. Propose minimal standardization
- Add or update only what is needed.
- Preserve current conventions and existing commands where possible.

3. Implement setup
- Tasks: create clear build/test/lint/run tasks.
- Pipelines: add deterministic CI checks with explicit steps.
- Devcontainer: provide reproducible toolchain and extensions.
- VS Code settings: include editor and formatting defaults relevant to the repo.

4. Validate
- Run the key tasks or commands locally where possible.
- Confirm CI config syntax if tooling is available.

5. Document usage
- Provide quick start commands and task names.

## Quality Bar
- Commands must be executable as written.
- Setup should be deterministic and portable.
- Avoid introducing unrelated dependencies.
- Keep changes explicit and reviewable.

## Recommended Defaults for Go Repos
- Build: go build ./...
- Test: go test ./... -v -count=1
- Race test (optional where available): go test ./... -race -count=1
- Vet: go vet ./...
- Tidy: go mod tidy

## CI Considerations
- Cache Go modules and build cache where supported.
- Fail fast on lint/test/build errors.
- Keep pipeline readable with short, named steps.

## Devcontainer Considerations
- Pin a stable Go toolchain.
- Install required extensions for Go and YAML.
- Ensure working directory and mount behavior are explicit.

## Recommended Model
- **Best fit**: Coding model.
- Config and script generation for tasks, pipelines, and devcontainer files.
- Toolchain knowledge (Go, Docker, CI systems) to produce valid, runnable setups.
- Attention to platform-specific syntax (PowerShell, bash, YAML) depending on target environment.

## Non-Goals
- Rewriting project architecture.
- Adding heavy platform-specific tooling without request.
- Replacing existing workflows that already satisfy team requirements.
