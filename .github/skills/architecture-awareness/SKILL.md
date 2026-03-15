---
name: architecture-awareness
description: "Use when understanding project architecture, runtime behavior, usage patterns, or planning new features and modifications. Triggers: understand architecture, how does this project work, where do I add a feature, how do I modify this behavior, explain system design, trace data flow end-to-end, map service boundaries, onboarding to codebase. Produces an architectural overview, usage map, extension points, and a concrete guide for adding or modifying features safely."
---

# Architecture Awareness Skill

## Goal
Give developers a clear, accurate picture of how the project is structured, how it runs in practice, and how to confidently add or modify features without breaking existing behavior.

## Scope
This skill is for:
- Understanding the overall architecture and component responsibilities.
- Mapping how the system is deployed and used end-to-end.
- Identifying the right place to add a new feature or change an existing one.
- Locating integration points, seams, and extension mechanisms.
- Surfacing constraints (performance, security, compatibility) that affect design decisions.
- Onboarding new developers to the codebase quickly.

## Output Contract
Return results in this order:

1. Architecture overview (layers, major components, and boundaries).
2. Runtime usage map (how the system starts, processes a request, and terminates).
3. Component responsibility table.
4. Extension guide (where and how to add or change features).
5. Constraints and risks for the planned change.
6. Recommended reading path.

## Workflow

1. Establish architecture style
- Identify the architectural pattern (layered, clean, ports/adapters, event-driven).
- Map top-level layers: entrypoints, coordination, domain, persistence, shared types.
- Note cross-cutting concerns: auth, logging, error handling, config.

2. Map runtime usage
- Trace the lifecycle: startup, request handling, state transitions, teardown.
- Follow at least one critical flow end-to-end from entrypoint to persistence.
- Identify where clients, services, stores, and external systems interact.

3. Build a component responsibility table
- List each major package/module.
- State its single responsibility and its allowed dependencies.
- Flag packages with unclear or mixed responsibilities.

4. Identify extension points
- Locate interfaces, registries, and plugin points explicitly designed for extension.
- Identify implicit seams where new behavior can be inserted safely.
- Note conventions used by existing features that new ones should follow.

5. Plan the feature addition or modification
- Identify the exact layers and files that need to change.
- Confirm dependency direction stays correct for the change.
- Note what tests need to be written or updated.
- Surface any cross-cutting impact (config, auth, docs, examples).

6. Surface constraints
- List invariants that must be preserved.
- Call out security, concurrency, and persistence considerations.
- Flag backward-compatibility risks for changes to shared contracts.

## Quality Bar
- All claims about architecture must be traceable to actual code.
- Distinguish design intent from current implementation reality.
- Provide enough specificity that a developer can start coding immediately.
- Separate stable architectural facts from areas of known complexity or debt.

## Recommended Artifacts to Inspect
- Entrypoints (`cmd/`) for startup and config wiring.
- Handler layer for API contracts and auth enforcement.
- Service/manager layer for orchestration and business rules.
- Store/persistence layer for state shape and durability guarantees.
- Shared types (`internal/common`) for cross-boundary contracts.
- Examples and config files for intended usage patterns.

## Recommended Model
- **Best fit**: Reasoning model.
- System-level thinking to map layers, boundaries, and dependency flows.
- Planning capability to identify the right extension points and impact of changes.
- Large context window to hold multiple files and reason about cross-module interactions.

## Non-Goals
- Redesigning architecture without explicit request.
- Proposing large-scale refactors as a prerequisite to feature work.
- Speculating about requirements not present in the codebase or stated by the user.
