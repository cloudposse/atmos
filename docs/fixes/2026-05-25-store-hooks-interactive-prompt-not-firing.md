# Fix: Store hooks don't fire when stack is selected via interactive prompt

**Date:** 2026-05-25

**Issue:** [#2432](https://github.com/cloudposse/atmos/issues/2432)
**Related:** [#2428](https://github.com/cloudposse/atmos/issues/2428) (closed, superseded by #2432 and #2433)

## Problem

`after-terraform-apply` store hooks only fire when the stack is passed
explicitly with `-s`. When the stack is selected via the interactive
choice prompt, the hooks silently don't fire — no output, no errors.

### Reproducer

```yaml
# Hook config (resolves correctly in `atmos describe component`)
hooks:
  store-outputs:
    command: store
    events: ["after-terraform-apply"]
    name: ssm
    outputs:
      vpc_flow_logs_bucket_arn: .vpc_flow_logs_bucket_arn
```

Works:
```bash
atmos terraform apply vpc-flow-logs-bucket -s my-stack
# ✓ Fetching vpc_flow_logs_bucket_arn output from vpc-flow-logs-bucket
```

Does NOT work:
```bash
atmos terraform apply vpc-flow-logs-bucket
# (select my-stack from interactive prompt)
# Hook never fires. No output, no errors.
```

### Expected behavior

Store hooks should fire regardless of whether the stack was passed via
`-s` or selected interactively.

### Environment

- Atmos 1.218.0
- macOS darwin/arm64

## Root cause

When the stack is selected via the interactive prompt, the resolved
stack name is not propagated to the hooks execution context. The hook
runner receives an empty stack value and silently skips execution.

The interactive prompt path sets the stack after the initial command
parsing, but the hooks infrastructure reads the stack from the
originally-parsed command arguments (which have an empty `-s` value).

### Code path to investigate

1. Interactive stack selection happens in the terraform command handler
   after initial flag parsing.
2. The selected stack must flow through to
   `internal/exec/hooks.go` (or equivalent hook runner) so the
   hook's `terraform output` subprocess can resolve the correct
   state backend.
3. The CI hooks check (`Running CI hooks`) may also be intercepting
   the event before the store hook has a chance to run — trace shows
   `CI provider not detected, skipping CI hooks` which suggests
   the event dispatch path routes through CI hooks first and may
   short-circuit for non-CI environments.

## Fix

TBD — requires investigation into:

1. How the interactively-selected stack propagates to hook execution.
2. Whether the CI hooks check is gating non-CI hook events.
3. Whether the hook runner should read the stack from the resolved
   execution context rather than the original CLI arguments.

## Workaround

Pass the stack explicitly with `-s`:

```bash
atmos terraform apply vpc-flow-logs-bucket -s my-stack
```

## Tests

- Unit test: verify that hooks fire when the stack is set
  programmatically (simulating interactive selection) rather than
  via CLI flag.
- Integration test: verify `after-terraform-apply` store hook fires
  for both `-s` and interactive prompt paths.

---

## Related

- [#2433](https://github.com/cloudposse/atmos/issues/2433) — Store
  hook doesn't assume backend role when reading terraform outputs
  (companion issue from the same investigation).
- [#2428](https://github.com/cloudposse/atmos/issues/2428) — Original
  consolidated issue (closed in favor of #2432 and #2433).
- [#2357](https://github.com/cloudposse/atmos/issues/2357) — Related
  auth resolver injection issue for hooks.
