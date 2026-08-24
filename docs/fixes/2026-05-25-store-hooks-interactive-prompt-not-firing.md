# Fix: Store hooks don't fire when stack is selected via interactive prompt

**Date:** 2026-05-25

**Issue:** [#2432](https://github.com/cloudposse/atmos/issues/2432)
**PR:** [#2520](https://github.com/cloudposse/atmos/pull/2520)
**Related:** [#2428](https://github.com/cloudposse/atmos/issues/2428) (closed, superseded by #2432 and #2433)

## Problem

`after-terraform-apply` store hooks only fire when the stack is passed
explicitly with `-s`. When the stack is selected via the interactive
choice prompt, the hooks silently don't fire — no output, no errors.

### Reproducer

```yaml
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

## Root cause

The interactive prompt fills `info.Stack` inside
`handleInteractiveComponentStackSelection` (`cmd/terraform/utils.go`),
but never persists it to the Cobra flag set. PostRunE hooks re-parse
args via `ProcessCommandLineArgs`, which reads
`cmd.Flags().GetString("stack")` — still empty from the original
parse. The hook runner sees an empty stack and silently skips.

## Fix

1. **Persist interactive selection to Cobra flag set.** After the
   interactive prompt fills `info.Stack`, the value is written back
   to the Cobra `--stack` flag via `f.Value.Set(stack)`. Errors from
   the flag set are wrapped with `ErrSetFlag` and returned.

2. **PostRunE hooks see the selected stack.** `runHooksWithOutput`
   re-parses args via `ProcessCommandLineArgs`, which reads
   `cmd.Flags().GetString("stack")` — now populated. Store hooks
   execute as expected.

3. **Consistent behavior.** Hook execution is now identical whether
   the stack is provided via `-s` or selected interactively.

### Files changed

- `cmd/terraform/utils.go` — flag persistence in
  `handleInteractiveComponentStackSelection`; `promptForStack` and
  `promptForComponent` changed from functions to `var` for test
  stubbing.

## Tests

- `TestInteractiveStackSelection_PersistsToCobraFlag` — stubs the
  interactive prompt, calls `handleInteractiveComponentStackSelection`,
  verifies both `info.Stack` and the Cobra flag are set.
- `TestHandleInteractiveComponentStackSelection_BothProvided` —
  short-circuit path when both component and stack are already set.
- `TestHandleInteractiveComponentStackSelection_SkipsMultiComponent` —
  skips prompting when multi-component flags are set.

## Workaround

Pass the stack explicitly with `-s`:

```bash
atmos terraform apply vpc-flow-logs-bucket -s my-stack
```

---

## Related

- [#2433](https://github.com/cloudposse/atmos/issues/2433) — Store
  hook doesn't assume backend role when reading terraform outputs.
- [#2428](https://github.com/cloudposse/atmos/issues/2428) — Original
  consolidated issue (closed in favor of #2432 and #2433).
- [#2357](https://github.com/cloudposse/atmos/issues/2357) — Related
  auth resolver injection issue for hooks.
