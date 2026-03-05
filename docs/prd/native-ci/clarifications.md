# Native CI Integration - Clarifications (Round 2)

All decisions from this round have been distributed into the relevant PRD files:

- Q1 (Callback-based HookAction) → [interfaces.md](./framework/interfaces.md), [hooks-integration.md](./framework/hooks-integration.md)
- Q2 (Use ExecuteOptions, no new struct) → [hooks-integration.md](./framework/hooks-integration.md)
- Q3 (Use lower-level Provider API) → [hooks-integration.md](./framework/hooks-integration.md)
- Q4 (Executor API is impl detail) → [hooks-integration.md](./framework/hooks-integration.md)
- Q5 (RunCIHooks signature correct) → [hooks-integration.md](./framework/hooks-integration.md)
- Q6 (Configurable auto upload/download) → [configuration.md](./framework/configuration.md), [planfile-storage.md](./terraform-plugin/planfile-storage.md)
- Q7 (Independent flags) → [planfile-storage.md](./terraform-plugin/planfile-storage.md)
- Q8 (before.terraform.apply for download) → [ci-detection.md](./framework/ci-detection.md)
- Q9 (Whitelist, omitted = all) → [configuration.md](./framework/configuration.md)
- Q10 (Mocks + golden files) → [provider.md](./providers/github/provider.md)
- Q11 (Fix generic.md path) → [generic.md](./providers/generic.md)

Minor fixes also applied:
- `implementation-status.md` paths corrected from `pkg/ci/github/` to `pkg/ci/providers/github/`
- Phase 4 output.go clarified as concrete OutputWriter implementation

This file can be deleted.
