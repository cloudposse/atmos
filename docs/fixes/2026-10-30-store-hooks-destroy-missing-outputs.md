# Fix: Store hooks no longer fail `terraform destroy` on missing outputs

**Date:** 2026-10-30

## Summary

A store hook now skips a Terraform output that no longer exists during a destroy, logs a warning, and lets the destroy finish. The hook still fails when the output can't be read, and on every other event.

## Context

`terraform destroy` now fires lifecycle hooks, and a hook with no `events:` matches every supported event. An existing store hook therefore ran after the destroy, found its outputs gone, and made Atmos exit 1 with `terraform output not found` after the resources had been destroyed. A destroy retried after a partial failure could hit the same error in the before hook and never run. The regression test for the upcoming release found this: v1.233.2 exited 0 in the same project.

This fix removes the false failure only. Atmos reads outputs when a hook runs and has no state of its own, so it can't recover values after they're gone, and stored values stay in the store after a destroy. The hooks documentation describes this rough edge.

## Changes

- `pkg/hooks/store_cmd.go`: on Terraform destroy events, including aggregates, a missing output logs a warning and is skipped. Other outputs in the hook are still written.
- `pkg/hooks/store_cmd_destroy_test.go`: covers before, after, and aggregate destroy events, plus the negative paths: missing outputs on other events, and read errors on destroy.
- `website/docs/stacks/hooks.mdx`: documents the behavior, stale stored values, and how to keep a store hook out of destroy runs.

## Validation

- Added tests first and confirmed the destroy cases failed before the change. The negative cases already passed and still pass.
- `go test -count=1 -v ./pkg/hooks/`: 226 top-level tests passed, 0 failed. The other `pkg/hooks/...` packages also pass.
- Ran an event-less store hook through `apply`, `destroy`, `destroy` with real Terraform on local state and a localhost fake Redis. v1.233.2 exits 0 on both destroys. The unfixed candidate exits 1 on both: the first in the after hook, the second in the before hook. This fix exits 0 on both and logs the warning, and the stored value stays in the store.
- Patch-scoped `custom-gcl` lint on `pkg/hooks/...` reports 0 issues; `gofumpt -l` reports no files.
- `npm --prefix website run build` exits 0.
- Not run: real Redis or cloud stores, `--all` runs, or the aggregate event end to end (unit test only).

## Follow-ups

None.
