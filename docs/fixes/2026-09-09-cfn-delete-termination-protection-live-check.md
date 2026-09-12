# Fix: `aws/cloudformation delete` checks live termination protection, not just local config

**Date:** 2026-09-09

## Summary

`deleteStack` (`pkg/component/aws/cloudformation/delete.go`) gated its termination-protection
safety check on `spec.TerminationProtection` — the local, resolved `termination_protection:`
stack config value — instead of the stack's actual live AWS state. When local config had
legitimately drifted to `false` while the deployed stack was still protected, the gate was
skipped entirely, `DeleteStack` was called directly, and AWS rejected it with its own raw,
unhelpful `ValidationError`, with none of Atmos's `--disable-termination-protection` hint. The
fix checks the stack's live `EnableTerminationProtection` (via `DescribeStacks`) whenever local
config alone can't be trusted to say "not protected," while still avoiding an extra AWS call in
the two cases where it isn't needed.

## Context

Commit `8e60963802` (already shipped) made `apply` never disable termination protection on AWS
even when local config says `termination_protection: false` — `applyTerminationProtection` only
ever turns protection ON during apply, never OFF. That fix was deliberate: it stops `apply` from
silently weakening a safety setting a user may have turned on directly against the stack (e.g.
via the AWS console) or in a previous deploy.

The side effect is that local config and live AWS state can now legitimately and permanently
diverge: a component's `termination_protection:` field can say `false` while the real deployed
stack still has protection `ON`, with no code path that ever reconciles the two.

`deleteStack`'s termination-protection gate was written before that divergence was possible, and
still read `spec.TerminationProtection` (the local value) as if it were authoritative:

```go
if spec.TerminationProtection {
    if !opts.DisableTerminationProtection {
        return errUtils.Build(...).Err() // actionable hint
    }
    ...
}
```

Confirmed against real AWS: with local config `termination_protection: true`, the gate correctly
fires before any `DeleteStack` call. But with local config drifted to `false` (the exact scenario
the apply-side fix intentionally allows) while the live stack is still protected, the local check
is skipped, `DeleteStack` is called, and AWS's raw `ValidationError: Stack ... cannot be deleted
while TerminationProtection is enabled` reaches the user instead of Atmos's hint pointing at
`--disable-termination-protection` — defeating the safety net at exactly the moment a user (who
correctly assumed local config changes don't affect the deployed stack, per the apply-side fix)
needed it most.

## Changes

- `pkg/component/aws/cloudformation/delete.go`:
  - `deleteStack` now delegates the termination-protection decision to a new
    `checkTerminationProtectionGate` helper (extracted to keep `deleteStack` under the
    `funlen`/`revive` line limit) and the `--retain-resources` status check to a new
    `checkRetainResourcesGate` helper.
  - `checkTerminationProtectionGate` behavior, chosen to minimize extra AWS calls:
    - `--disable-termination-protection` set: skips any live lookup and unconditionally calls
      `disableTerminationProtection` (`UpdateTerminationProtection(false)`), then proceeds —
      regardless of whether local config, live AWS state, both, or neither reported protection
      on. `UpdateTerminationProtection(false)` is a no-op if protection was already off, so this
      is safe and avoids a redundant `DescribeStacks` call in the common "I'm disabling it
      anyway" path. This also fixes a related latent gap: previously, if local config said
      `false` and `--disable-termination-protection` was still passed defensively, the old code
      never called `UpdateTerminationProtection` at all (the local-only `if` was skipped) and
      would have hit the same raw AWS error if live state was actually protected.
    - `--disable-termination-protection` not set, local config `true`: fails immediately with
      the existing actionable hint, no API call — preserves the original zero-extra-call fast
      path since local config is already sufficient to answer the question.
    - `--disable-termination-protection` not set, local config `false`: local config cannot be
      trusted alone (per the apply-side fix above), so it now calls the new `describeStack`
      helper (`DescribeStacks`) and checks the live `EnableTerminationProtection` value. If live
      state reports protection on, the same actionable hint fires and `DeleteStack` is never
      called. This is the new behavior that closes the gap.
  - Added `describeStack(ctx, client, stackName) (*cfntypes.Stack, error)`, a shared helper
    wrapping `DescribeStacks` and returning the full `Stack` (status, termination protection,
    etc.), reused by both gates. `checkTerminationProtectionGate` returns the stack it fetched
    (if any) so `checkRetainResourcesGate` can reuse it instead of issuing a second
    `DescribeStacks` call — a single delete invocation that needs both a live termination-
    protection check and the `--retain-resources` status check now makes at most one
    `DescribeStacks` call total, not two.
  - Removed `currentStackStatus`: after the refactor it was no longer called from production
    code (only from its own tests), which `unparam` correctly flagged as dead weight. Its two
    tests were repointed at `describeStack`, the helper that now actually implements the same
    "wrap the API error, error on stack-not-found" contract.
- `pkg/component/aws/cloudformation/delete_test.go`:
  - Updated `TestDeleteStack_NoTerminationProtection` to mock the new live `DescribeStacks` call
    (returning `EnableTerminationProtection: false`) that now precedes the `DeleteStack` call
    when local config is `false`.
  - Added `TestDeleteStack_BlocksOnLiveTerminationProtection`: local config `false`, live
    `DescribeStacks` reports `EnableTerminationProtection: true` — the hint must still fire and
    `DeleteStack` must never be called. This is the regression test for the field-tested bug.
  - Added `TestDeleteStack_DisableTerminationProtectionFlag_SkipsLiveLookup`: local config
    `false`, `--disable-termination-protection` set — no `DescribeStacks` call is made,
    `UpdateTerminationProtection` and `DeleteStack` both succeed.
  - Renamed `TestCurrentStackStatus_*` to `TestDescribeStack_*`, now exercising `describeStack`
    directly.
- `pkg/component/aws/cloudformation/executor_test.go`:
  - `TestRunDelete`, `TestRunDelete_FailedStatus`, `TestRunDelete_DeleteStackError`,
    `TestRunDelete_StreamEventsError`, and the `"delete"` case in
    `TestOperationHandlers_Dispatch` all exercise `deleteStack` with local config `false` and no
    `--disable-termination-protection`, so each now mocks the additional live `DescribeStacks`
    call (returning an unprotected stack) that `checkTerminationProtectionGate` issues before
    `DeleteStack`.

## Validation

- `go build ./...` — passes.
- `go test -count=1 ./pkg/component/aws/cloudformation/...` — passes, including the new/updated
  tests above.
- `GOTOOLCHAIN=go1.26.6 ./custom-gcl run --config=.golangci.yml --allow-serial-runners
  --new-from-rev=origin/main` — zero findings against `delete.go`, `delete_test.go`, or the
  hunks touched in `executor_test.go`. (Plain `atmos lint --changed` failed with an unrelated
  `custom-gcl`/Homebrew-Go-toolchain typecheck panic in a third-party dependency, a known local
  environment issue; running `custom-gcl` directly with `GOTOOLCHAIN` pinned to `go.mod`'s
  version works around it — see `docs/fixes/` for the pre-existing record of that issue.)
- Remaining findings from the `custom-gcl` run (a `dupl` finding across three pre-existing
  `TestRunApply_*` tests in `executor_test.go`, a `cyclop` finding in
  `pkg/utils/component_path_utils.go`, and several `paralleltest`/`godot` findings in
  `internal/exec/`) are outside this fix's diff (confirmed via `git diff HEAD` — none touch the
  lines those findings point at) and outside this bug's scope; `pkg/utils/component_path_utils.go`
  and the `internal/exec` files belong to other concurrently-running fixes in this session and
  were left untouched per instructions.

## Follow-ups

None.
