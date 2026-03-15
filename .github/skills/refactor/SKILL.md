---
name: refactor
description: "Use when restructuring code without changing behavior. Triggers: refactor, cleanup, simplify, extract method, rename symbol, reduce duplication, improve readability, improve maintainability, technical debt cleanup. Produces behavior-preserving code changes with tests and risk notes."
---

# Refactor Skill

## Goal
Improve structure, readability, and maintainability while preserving runtime behavior.

## Refactor Contract
Always preserve:
- Public API behavior unless explicitly requested.
- Input/output semantics.
- Error and status-code behavior.
- Persistence and protocol compatibility unless explicitly changed.

## Output Contract
Return results in this order:

1. Refactor summary.
2. Behavior-preservation checks performed.
3. Risk notes.
4. Follow-up opportunities.

## Refactor Workflow

1. Baseline
- Identify affected files and call paths.
- Run or inspect existing tests for the target area.

2. Refactor plan
- Prefer small, isolated changes.
- Choose safe transforms first:
  - Extract helper functions.
  - Remove duplication.
  - Improve naming.
  - Tighten function boundaries.
  - Simplify conditionals.

3. Apply changes
- Keep edits minimal and local.
- Avoid unrelated formatting churn.
- Add concise comments only where complexity warrants.

4. Verify behavior
- Re-run impacted tests first, then broader tests as needed.
- Validate key negative paths and boundary conditions.

5. Report
- Explain what changed structurally.
- Confirm what behavior remained unchanged.
- Note any residual risk.

## Go Refactor Checklist
- Keep error wrapping/comparisons intact.
- Preserve context cancellation behavior.
- Preserve goroutine lifecycle and synchronization.
- Preserve HTTP handlers and status codes.
- Preserve serialization tags and field names.
- Preserve storage schema and migration assumptions unless requested.

## Safety Rules
- Do not silently alter auth, retry, timeout, or persistence behavior.
- Do not rename externally consumed API fields without explicit request.
- Do not remove tests that protect behavior.
- Prefer adding tests when refactoring complex branches.

## Repo Defaults (sear)
For validation, prefer:
- go test ./... -v -count=1
- go test ./internal/daemon/handlers/...
- go test ./internal/daemon/store/...
- go test ./internal/client/...

For race-sensitive refactors where available:
- go test ./... -race -count=1

## Recommended Model
- **Best fit**: Coding + reasoning model.
- Code generation capability to produce idiomatic, compiling refactored code.
- Reasoning ability to verify behavior is preserved and no invariants are broken.
- Test awareness to validate refactor safety before and after.

## Non-Goals
- Feature additions.
- Behavioral redesign.
- Large architecture rewrites without explicit direction.
