# Kompakt Project Documentation Index

Welcome to the Kompakt documentation hub. This index provides easy access to all guides, references, and support resources for operators, integrators, and contributors.

---

## 📚 Guides & References

- [API Endpoints](api-endpoints.md): HTTP and WebSocket API reference for kompakt.
- [Playbook Model](playbook-model.md): Workflow structure, environment injection, secrets, and artifact usage.
- [Example Playbook](../examples/playbook.yml): Sample deployment workflow.
- [Daemon Configuration](../examples/config.yml): Daemon config example.
- [Client Configuration](../examples/client.config.yml): Client config example.
- [Secrets File](../examples/secrets.yml): Daemon secrets example.
- [Contributing Guide](../CONTRIBUTING.md): How to contribute to Kompakt.

---

## 🚀 Onboarding & Quick Start

- [Quick Start in README](../README.md#quick-start): Step-by-step setup for new users.
- [Operator Guide](../README.md#platform-overview): Overview of Kompakt components and their roles.
- [Dashboard UI](../README.md#quick-start): Access real-time status and deployment logs.

---

## ❓ Frequently Asked Questions (FAQ)

**Q: How do I register a new client?**
A: Set a matching `registration_secret` in both daemon and client configs, then run the client binary with its config.

**Q: How are secrets managed?**
A: Secrets are defined in `secrets.yml` and injected into playbooks using `${{ secrets.NAME }}` syntax.

**Q: What happens if a client reboots during deployment?**
A: Kompakt resumes from the last confirmed step after reboot, ensuring deterministic rollout.

**Q: How are artifacts distributed?**
A: Artifacts are uploaded to the daemon and referenced in playbooks; clients download them automatically during workflow execution.

**Q: Where are deployment logs stored?**
A: All logs are persisted in the daemon's logs directory for audit and troubleshooting.

---

## ⚙️ Technical Choices & Quality Practices

- **Language:** Go (no CGo dependencies; portable and cross-compilable)
- **Workflow Engine:** YAML-based, inspired by GitHub Actions for familiar syntax
- **Authentication:** JWT for clients, HTTP Basic for root endpoints
- **Persistence:** Durable state and logs for reboot-safe operation
- **Testing:** Extensive unit and integration tests, race detection enabled
- **Linting:** Static analysis via `go vet` and `golangci-lint`
- **CI/CD:** Automated builds, tests, and releases with GitHub Actions
- **Documentation:** Maintained as first-class artifacts, updated with every feature/refactor

---

## 🏆 Project Values

- **Usability:** Clear onboarding, example configs, and dashboard UI for easy adoption
- **Reliability:** Reboot-safe deployments, artifact distribution, and log persistence
- **Security:** Secret injection, registration secrets, and audit logs
- **Extensibility:** Modular design for custom workflows and integrations
- **Support:** Responsive issue tracking and community engagement

---

For additional help, see the README or open an issue on GitHub. If you have suggestions for improvement, please contribute or contact the maintainers.
