# Native CI Integration - Clarifications

All decisions from this document have been distributed into the relevant PRD files:

- Q1 (Plugin-Executor architecture) → [hooks-integration.md](./framework/hooks-integration.md), [interfaces.md](./framework/interfaces.md)
- Q2 (Error severity) → [hooks-integration.md](./framework/hooks-integration.md), [overview.md](./overview.md)
- Q3–5 (PR comments deferred) → [pr-comments.md](./providers/github/pr-comments.md)
- Q6 (Output last-writer-wins) → [ci-outputs.md](./providers/github/ci-outputs.md)
- Q7 (Plan verification workflow) → [plan-verification.md](./terraform-plugin/plan-verification.md)
- Q8 (Matrix fixed schema) → [describe-affected-matrix.md](./terraform-plugin/describe-affected-matrix.md)
- Q9 (ci.enabled kill switch) → [ci-detection.md](./framework/ci-detection.md), [configuration.md](./framework/configuration.md)
- Q10 (Per-plugin storage) → [artifact-storage.md](./framework/artifact-storage.md)
- Q11 (context_prefix wiring) → [status-checks.md](./providers/github/status-checks.md)
- Q12 (GitHub Artifacts lookup) → [planfile-storage.md](./terraform-plugin/planfile-storage.md)
- Q13 (Store type validation) → [planfile-storage.md](./terraform-plugin/planfile-storage.md)
- Q14 (Testing strategy) → [implementation-status.md](./framework/implementation-status.md)
- Q15 (Rewrite hooks-integration.md) → Done

This file can be deleted.
