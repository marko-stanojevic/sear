---
name: code-review
description: "Use when reviewing code changes, pull requests, regressions, or test gaps. Triggers: review, code review, PR review, audit changes, find bugs, risks, regressions, missing tests, unsafe behavior, compatibility issues. Produces severity-ranked findings with file/line evidence and concrete fixes."
---

# Code Review Skill

## Goal
Perform a practical engineering review focused on correctness, risk, and regressions.

## Output Contract
Always return results in this order:

1. Findings first, ordered by severity.
2. Open questions and assumptions.
3. Brief change summary (only after findings).
4. Suggested next actions.

If no issues are found, explicitly say so and call out remaining risk or test gaps.

## Severity Model
- Critical: Security, data loss, auth bypass, destructive behavior, severe outage risk.
- High: Behavioral regression, concurrency bugs, broken error handling, invalid state transitions.
- Medium: Edge-case bugs, weak validation, incomplete tests for key branches.
- Low: Maintainability issues, unclear naming, missing comments for complex logic.

## Review Workflow

1. Gather context
- Check changed files and affected packages.
- Identify entry points, state mutation paths, and IO boundaries.

2. Validate behavior
- Compare intended behavior vs implemented behavior.
- Focus on error paths, nil/empty handling, and branching logic.

3. Check reliability
- Concurrency safety.
- Resource lifecycle correctness (files/sockets/goroutines).
- Timeout and retry behavior.

4. Check compatibility and persistence
- Config and schema assumptions.
- Persistent-state readers/writers and migration-sensitive code.

5. Check tests
- Confirm tests cover happy path and negative paths.
- Look for missing race-sensitive and boundary tests.

## Go-Specific Checklist
- Error values are wrapped or compared correctly.
- Context cancellation is honored in blocking paths.
- No goroutine leaks.
- Deferred cleanup order is safe.
- Maps/slices are not mutated unsafely across goroutines.
- HTTP status and auth behavior are consistent.
- Time handling is deterministic where required.

## Evidence Rules
For each finding include:
- Severity.
- Why it matters.
- Exact file and line reference.
- Minimal fix recommendation.
- Test recommendation (if applicable).

## Repo Defaults (sear)
When useful for validation, use:
- go test ./... -v -count=1
- go test ./... -race -count=1
- go test ./internal/daemon/handlers/...
- go test ./internal/daemon/store/...

Prioritize review of:
- internal/daemon/handlers
- internal/daemon/service
- internal/daemon/store
- internal/client

## Recommended Model
- **Best fit**: Coding + reasoning model.
- Code fluency to read idiomatic Go and detect logic errors and anti-patterns.
- Reasoning ability to assess exploitability, regression risk, and missing test cases.
- Structured output discipline for severity-ranked findings with evidence.

## What Not To Do
- Do not lead with a summary before findings.
- Do not report style-only nits as primary issues.
- Do not claim certainty without code evidence.
- Do not skip testing implications for risky changes.
