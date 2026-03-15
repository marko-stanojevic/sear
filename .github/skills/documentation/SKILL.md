---
name: documentation
description: "Use when code changes require documentation updates. Triggers: feature added, behavior changed, endpoint modified, config changed, flags added/removed, files deleted, refactor with user-visible impact, release notes preparation. Produces accurate doc updates mapped directly to additions/changes/deletions in code."
---

# Documentation Sync Skill

## Goal
Keep documentation accurate and current by translating code additions, modifications, and deletions into concrete doc updates.

## Scope
This skill is for:
- Updating README, API docs, and operation guides after code changes.
- Generating changelog and migration notes from merged deltas.
- Detecting stale docs when code paths, flags, endpoints, or configs changed.
- Capturing behavior, defaults, and compatibility impacts.
- Removing obsolete documentation for deleted code paths.

## Output Contract
Return results in this order:

1. Change summary (adds/changes/deletes).
2. Doc impact matrix (code area -> doc file/section).
3. Proposed edits (exact text snippets or patch plan).
4. Verification checklist.
5. Follow-up docs still missing.

## Workflow

1. Gather code deltas
- Inspect changed, added, and deleted files.
- Classify each change as behavior, API, config, CLI, runtime, or internal-only.

2. Determine documentation impact
- Map each code change to affected docs (README, docs/, examples, changelog).
- Mark severity: required update, optional clarification, or no user-facing impact.

3. Draft doc updates
- Write concise, user-facing descriptions of what changed and why.
- Include examples for new/changed usage and remove obsolete examples.
- Add migration guidance when behavior or defaults changed.

4. Apply consistency checks
- Ensure command names, flags, endpoints, and config keys match code exactly.
- Ensure terminology is consistent across all docs.

5. Validate completeness
- Confirm every high-impact code change has matching doc updates.
- Flag unknowns or missing implementation details as TODO questions.

## Quality Bar
- Documentation statements must be traceable to current code.
- Prefer minimal, targeted edits over broad rewrites.
- Keep wording concrete and testable.
- Clearly distinguish breaking changes from non-breaking improvements.

## Recommended Documentation Targets
- `README.md` for quick start and common workflows.
- `docs/api-endpoints.md` for contract-level API behavior.
- `examples/` files when configuration or usage changed.
- `CHANGELOG.md` (if present) for release-facing change tracking.

## Recommended Model
- **Best fit**: Language model.
- Strong prose generation and editing for clear, developer-facing writing.
- Ability to summarize code diffs and translate technical changes into readable language.
- Consistency checking across doc files for unified terminology and style.

## Non-Goals
- Inventing behavior not implemented in code.
- Rewording entire docs without change-driven need.
- Replacing architecture docs unless requested.
