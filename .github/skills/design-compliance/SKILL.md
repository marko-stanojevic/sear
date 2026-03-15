---
name: design-compliance
description: "Use when validating architecture and code quality against design patterns, SOLID and clean architecture principles, core programming fundamentals, and maintainability standards. Triggers: design review, architecture conformance, SOLID check, clean architecture audit, bad practice detection, code quality guidance. Produces compliance findings and actionable developer guidance."
---

# Design Compliance Skill

## Goal
Verify that implementation choices align with sound design practices and guide developers toward maintainable, extensible, and testable code.

## Scope
This skill is for:
- Verifying use of appropriate design patterns and avoiding unnecessary pattern complexity.
- Enforcing design principles (SOLID and clean architecture boundaries).
- Checking core programming fundamentals (cohesion, coupling, naming, error handling, and testability).
- Detecting bad practices (god objects, hidden side effects, duplicated logic, leaky abstractions).
- Providing practical guidance developers can apply immediately.

## Output Contract
Return results in this order:

1. Compliance summary.
2. Findings by severity (critical, major, minor) with file evidence.
3. Principles/patterns violated or satisfied.
4. Recommended remediations with concrete refactor steps.
5. Coaching notes for developers (what to learn/apply next).

## Workflow

1. Define review boundary
- Identify target modules, services, handlers, and interfaces under review.
- Establish expected architectural style (layered, clean architecture, ports/adapters).

2. Verify design patterns
- Confirm pattern intent matches implementation (for example strategy, factory, adapter).
- Flag anti-pattern usage and pattern misuse.

3. Enforce principles
- SOLID checks:
  - Single responsibility and separation of concerns.
  - Open/closed via extension points instead of repeated branching edits.
  - Liskov substitution behavior safety.
  - Interface segregation and minimal contracts.
  - Dependency inversion at module boundaries.
- Clean architecture checks:
  - Dependency direction toward core domain.
  - Framework and transport concerns isolated from business logic.
  - Ports/interfaces used where boundaries are required.

4. Check fundamentals
- Cohesion and coupling balance.
- Readability, naming clarity, and duplication control.
- Error handling consistency and boundary validation.
- Testability of units and seams for mocks/fakes.

5. Detect bad practices
- Large, multi-purpose modules with unclear ownership.
- Shared mutable state without safe control.
- Premature abstraction and speculative generality.
- Hidden I/O or side effects in low-level helpers.

6. Guide developers
- Recommend small, safe refactor sequence with low regression risk.
- Suggest tests to protect behavior before changes.
- Provide clear before/after outcomes.

## Quality Bar
- Every finding maps to a specific principle or pattern expectation.
- Recommendations are incremental and behavior-preserving unless otherwise stated.
- Advice is practical for current codebase constraints.
- Distinguish style preferences from true design risks.

## Recommended Model
- **Best fit**: Reasoning model.
- Principle recall and application for SOLID, clean architecture, and design patterns.
- Structured analysis to rank findings by severity and map to specific violations.
- Coaching capability to produce actionable, incremental guidance for developers.

## Non-Goals
- Rewriting architecture without stakeholder request.
- Introducing patterns where simple code is sufficient.
- Blocking delivery for non-critical stylistic disagreements.
