---
name: test-driven-development
description: "Use when writing unit tests, practicing TDD, or improving test coverage. Triggers: write tests, test-driven development, TDD, unit test, table tests, test coverage, test gaps, missing assertions, test refactor, behavior verification. Produces well-structured tests with clear arrange/act/assert form, coverage for happy paths and edge cases, and actionable guidance for test-first workflows."
---

# Test-Driven Development Skill

## Goal
Help developers write effective unit tests and apply TDD to produce correct, maintainable, and confidence-building test suites.

## Scope
This skill is for:
- Writing unit tests for new and existing code.
- Practicing red/green/refactor TDD cycles.
- Identifying test gaps and missing assertions.
- Structuring table-driven and subtests idiomatically.
- Improving test quality, coverage, and readability.
- Detecting tests that assert the wrong things or provide false confidence.

## Output Contract
Return results in this order:

1. Test strategy summary (what to test and why).
2. Test cases list (happy path, edge cases, failure paths).
3. Implemented test code.
4. Coverage impact notes.
5. Follow-up test gaps remaining.

## Workflow

1. Understand the unit under test
- Identify inputs, outputs, and observable side effects.
- Detect dependencies that require mocking or faking.
- Confirm what behavior is contractual versus implementation detail.

2. Define test cases before writing code (TDD)
- Write the failing test first when practicing TDD.
- Name tests after behavior, not implementation.
- Cover: happy path, boundary values, invalid inputs, and error paths.

3. Write clean, structured tests
- Use arrange/act/assert structure consistently.
- Keep each test focused on one behavior.
- Use table-driven tests for repeated logic with varying inputs.
- Use subtests to group related cases with shared setup.

4. Handle dependencies safely
- Use interfaces and fakes over live dependencies.
- Avoid test coupling to internal implementation details.
- Prefer in-memory fakes over mocks where practical.

5. Validate test quality
- Confirm each assertion fails when the real behavior is wrong.
- Check that test names describe failure context clearly.
- Avoid logical assertions that can never fail.
- Prefer specific assertions over broad catch-all checks.

6. Measure and close coverage gaps
- Identify untested paths with coverage tooling.
- Prioritize covering critical paths and boundary conditions.
- Do not chase 100% coverage at the expense of test value.

## Quality Bar
- Tests must fail before the implementation is correct and pass after.
- Test names read as behavioral sentences.
- No shared mutable state between test cases.
- Tests are deterministic and do not depend on execution order.
- Concurrency tests must be race-safe.

## Go-Specific Defaults
- Use the standard `testing` package.
- Table-driven tests via `[]struct{ name, input, expected }` slices.
- Subtests via `t.Run`.
- Race checks: `go test ./... -race -count=1`.
- Coverage profile: `go test ./... -coverprofile=cover.out; go tool cover -func=cover.out`.
- Use `httptest` for HTTP handler tests.
- Use interface fakes for stores, hubs, and external clients.

## Recommended Model
- **Best fit**: Coding model.
- Code generation for idiomatic, compiling test code with correct arrange/act/assert structure.
- Knowledge of Go testing patterns (table tests, subtests, `httptest`, fakes).
- Coverage analysis reasoning to find meaningful gaps and prioritize test cases.

## Non-Goals
- Writing tests that replicate implementation details instead of behavior.
- Achieving 100% coverage as a primary metric over test quality.
- Using heavyweight mocking frameworks when simple fakes suffice.
