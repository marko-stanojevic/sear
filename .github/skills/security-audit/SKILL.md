---
name: security-audit
description: "Use when reviewing code for security issues or running security scans. Triggers: security audit, security review, security scan, vuln scan, vulnerability audit, auth bypass check, secrets exposure, injection risk, insecure defaults, hardening review. Produces severity-ranked findings with exploitability notes and concrete remediation steps."
---

# Security Review/Scan Skill

## Goal
Find and prioritize security risks in code and configuration, then propose concrete, low-risk fixes.

## Output Contract
Always return results in this order:

1. Findings first, ordered by severity.
2. Exploitability and impact notes.
3. Remediation steps (minimal and practical).
4. Verification/tests to confirm fixes.

If no issues are found, state that explicitly and include residual risk.

## Severity Model
- Critical: Remote code execution, auth bypass, privilege escalation, plaintext secret leakage with immediate abuse path.
- High: Injection risks, broken access control, weak token handling, insecure transport or credential storage.
- Medium: Unsafe defaults, missing validation/sanitization, weak error handling that exposes internals.
- Low: Hardening gaps, defense-in-depth improvements, missing security-focused tests.

## Review Workflow

1. Scope and surfaces
- Identify trust boundaries, inputs, auth entry points, and state-changing endpoints.
- Map data flow: input -> validation -> storage -> output/logging.

2. Static security review
- Check authn/authz logic.
- Check secrets handling and logging.
- Check input validation and parser usage.
- Check file/network/process boundaries.

3. Security scanning
- Dependency vulnerabilities.
- Common Go security patterns (unsafe command execution, path traversal, insecure randomness/crypto usage mistakes).

4. Prioritize and fix
- Prefer minimal, behavior-preserving remediations.
- Add security-focused tests for risky paths.

## Go + Sear Checklist
- JWT validation paths enforce algorithm/method checks.
- Root/basic auth and client JWT access boundaries are not mixed incorrectly.
- Request bodies and path params are validated before use.
- Shell/command execution paths avoid injection vectors.
- Artifact upload/download paths prevent traversal and unauthorized access.
- Secrets are never exposed in list endpoints or logs.
- Persistent files use restrictive permissions.

## Suggested Commands
Use what is available in environment:

- go test ./... -v -count=1
- go test ./... -race -count=1
- go vet ./...
- govulncheck ./...
- go list -m -u all

If `govulncheck` is unavailable, report that and continue with code-level review plus `go vet`.

## Evidence Rules
For each finding include:
- Severity.
- Affected file and line reference.
- Why it is risky and realistic abuse scenario.
- Concrete fix recommendation.
- Test/verification recommendation.

## Recommended Model
- **Best fit**: Reasoning model.
- Adversarial reasoning for threat modeling, abuse scenario construction, and attack surface analysis.
- Policy and principle recall for OWASP, CWE, and secure design patterns.
- Structured risk ranking with exploitability and remediation rationale.

## Non-Goals
- Broad stylistic linting.
- Re-architecting the system unless required to mitigate a critical issue.
