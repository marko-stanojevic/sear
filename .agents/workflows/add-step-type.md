---
description: how to add a new playbook step type (action) to the agent
---

1. Define the action name (e.g., `my-action`).
2. Open [internal/agent/executor/executor.go](../../internal/agent/executor/executor.go).
3. Locate the `RunStep` function's `switch` statement.
4. Add a new `case step.Uses == "my-action":`.
5. Implement the logic either directly in the case or as a helper function (e.g., `runMyAction`).
6. Ensure the result returns `Result{Err: err}` for failures or `Result{NeedsReboot: true}` for reboots.
7. If the action needs new fields in the `with` YAML block, access them via `step.With["key"]`.
8. Add a test case in [internal/agent/executor/executor_test.go](../../internal/agent/executor/executor_test.go).
