# Contract: Component Identity in Atmos Pro Uploads

**Feature**: `specs/003-fix-upload-component-name` | **Date**: 2026-09-10

## Invariant

> Every payload Atmos uploads to Atmos Pro that names a component MUST carry the component's **full logical name** — the exact key under `components.terraform:` in the stack manifest (e.g. `foo/bar/baz`) — never a truncated segment, a Terraform directory name, or a filesystem path.

This is the correlation key Atmos Pro uses for exact-match lookups (approvals "previous plan", instance identity, locks). Any channel deviating from it breaks cross-channel joins.

## Per-channel contract

| # | Channel | Payload / field | Invocation shapes | Status before | Status after |
|---|---------|----------------|-------------------|---------------|--------------|
| 1 | Instance status (`--upload-status`) | `InstanceStatusUploadRequest.Component` | single + per node in bulk | ❌ leaf (`baz`) | ✅ full (`foo/bar/baz`) — **fixed here** |
| 2 | Execution record, bulk aggregate | `Data.components[].component` (per graph node) | `--affected`/`--all`/`--components`/`--query` | ❌ leaf | ✅ full — **fixed here** |
| 3 | Execution record, single-component | `Data.components[].component` | single | ✅ full (CLI arg) | ✅ full — regression-guarded |
| 4 | Execution record envelope | `Args[0]` | single | ✅ full | ✅ unchanged |
| 5 | Stack lock/unlock | lock key `<owner>/<repo>/<stack>/<component>` | explicit command | ✅ full | ✅ unchanged |
| 6 | Affected/inventory uploads | `Component` per affected entry | describe-affected flows | ✅ full | ✅ unchanged |

Known exception (deferred, spec FR-008): channel 3 reports the raw filesystem path when the CLI argument is path-style (`./components/terraform/vpc`) — tracked in a follow-up issue, not fixed here.

## Contract tests

- Unit: nested-name (`foo/bar/baz`) and flat-name (`vpc`) cases for channels 1–3 (new for 1–2, regression guard for 3).
- Pact (`pkg/pro/consumer_pact_test.go`): shape unchanged; existing flat-name fixtures remain valid. No pact changes expected (research.md Decision 5).
- Cross-channel: for one nested-component invocation, channels 1 and 3/4 report the identical string (spec SC-002).
