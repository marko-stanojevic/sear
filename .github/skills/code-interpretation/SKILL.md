---
name: code-interpretation
description: "Use when reading AI-generated code or understanding an unfamiliar codebase. Triggers: explain generated code, map architecture, trace execution flow, identify module responsibilities, understand dependencies, summarize file purpose. Produces a clear mental model, flow maps, and actionable next reading steps."
---

# Code Interpretation Skill

## Goal
Help users quickly understand generated code and the broader codebase with accurate, evidence-backed explanations.

## Scope
This skill is for:
- Interpreting AI-generated code for correctness, intent, and maintainability.
- Explaining existing modules, interfaces, and file responsibilities.
- Mapping execution flow across handlers, services, stores, and clients.
- Summarizing dependency relationships and data movement.
- Turning large code areas into concise learning paths.

## Output Contract
Return results in this order:

1. High-level summary.
2. Component map.
3. Execution flow (entrypoints to outcomes).
4. Key assumptions and risks.
5. Suggested next files/functions to read.

## Workflow

1. Establish context
- Identify user goal: bug fix, feature work, review, onboarding, or refactor.
- Detect relevant entrypoints and primary packages.

2. Build a component map
- List major modules and each module's responsibility.
- Note boundaries: API layer, domain/service layer, persistence, shared types.

3. Trace important flows
- Follow one or two critical paths end-to-end.
- Capture where data is validated, transformed, persisted, and emitted.

4. Interpret generated code safely
- Explain what the code does line-by-line only where needed.
- Call out unclear naming, hidden coupling, and unhandled error paths.

5. Surface risks and confidence
- Distinguish verified behavior from assumptions.
- Highlight test coverage gaps and likely regression areas.

6. Provide learning path
- Recommend a short sequence of files/functions to read next.
- Include concrete questions the user should answer while reading.

## Quality Bar
- Explanations must be anchored to actual code locations.
- Keep summaries concise, then drill down only where useful.
- Prefer behavior and data-flow explanations over restating syntax.
- Separate facts from interpretation.

## Recommended Model
- **Best fit**: Language model.
- Strong language comprehension and summarization for translating code structure into clear explanations.
- Broad context window to hold multiple files and trace cross-module flows.
- Reasoning ability to separate facts from assumptions.

## Non-Goals
- Performing broad refactors unless requested.
- Guessing behavior not supported by code evidence.
- Rewriting architecture during explanation tasks.
