# Fix: `aws/cloudformation apply` no longer crashes on a publish-only target, and gains a friendlier "stack not found" error

**Date:** 2026-09-09

## Summary

`atmos aws cfn apply <component> -s <stack> --target <publish-only-target>` crashed with a raw,
unwrapped AWS SDK error (`ValidationError: Stack [...] does not exist`) whenever the selected
provision target never actually deployed a stack directly — a `kind: aws/s3` target selected via
`--target`, or any other non-`aws/cloudformation` target kind (e.g. `kind: git`). `runApply`
unconditionally ran the stack-policy, termination-protection, and outputs follow-up steps after
`deliverApply` returned, regardless of whether a direct stack deploy had actually happened. Fixed
by gating those three follow-up steps behind `deliverApply`'s `*changeSetResult` return value.
Separately, a user explicitly asked for friendlier error messages after hitting the raw AWS error
verbatim; a narrowly-scoped helper now recognizes AWS's "stack does not exist" validation-error
shape and adds an explanation + actionable hint via the error builder, reusing this package's
existing `isStackNotFoundError` pattern-recognition helper.

## Context

`pkg/component/aws/cloudformation/provision.go`'s `deliverApply` resolves the selected provision
target for an `apply` and has three possible outcomes:

1. `kind: aws/cloudformation` (the default/implicit target) — deploys directly via `deployDirect`
  and its changeset flow, returning a non-nil `*changeSetResult`.
2. `kind: aws/s3` selected explicitly (e.g. `--target artifacts`) — publish-only: uploads the
  template to S3 and returns immediately with `result == nil`. No stack is created or touched.
3. Any other kind (e.g. `kind: git`) — packages through S3, then delivers via the generic target
  registry (`target.Deliver`), also never touching a direct stack. Returns `result == nil`.

`pkg/component/aws/cloudformation/executor.go`'s `runApply` called `setStackPolicy`,
`applyTerminationProtection`, and `describeStackOutputs` unconditionally after `deliverApply`
returned without error — regardless of which of the three outcomes had occurred. For outcomes 2
and 3, no stack exists (or the stack, if any, is unrelated to this operation), so these
stack-scoped API calls always failed. The confirmed live repro: a component with
`termination_protection: true` and an `aws/s3` provision target named `artifacts`, never yet
deployed as a direct stack, running `atmos aws cfn apply <component> -s <stack> --target
artifacts`, hit:

```text
Error: aws/cloudformation API call failed: operation error CloudFormation: UpdateTerminationProtection, https response error
StatusCode: 400, ... api error ValidationError: Stack [<name>] does not exist
```

This error is a raw AWS SDK error string with no explanation or actionable next step, following
this package's dominant (but not universal) `fmt.Errorf("%w: %w", errUtils.ErrAwsCloudFormationAPICallFailed, err)`
wrapping pattern — contrasted with the curated errors elsewhere in the same package (e.g.
`delete.go`'s termination-protection/retain-resources gates) that use the full error-builder
pattern with an explanation and hint.

## Changes

**Bug 1 (primary) — `pkg/component/aws/cloudformation/executor.go`, `runApply` (~line 397-448):**

`result != nil` is a reliable signal for "a direct stack deploy just happened, successfully."
Verified by reading `deliverApply`, `deployDirect`, `createChangeSet`, and `waitForChangeSet` in
full:

- `deliverApply` only ever returns a non-nil `result` from the `selected.Kind ==
  cfg.CloudFormationComponentType` branch (`deployDirect`'s outcome); both other branches
  (`aws/s3` publish-only, and the generic external-target delivery) explicitly return `nil` for
  `result`.
- `deployDirect` returns whatever `createChangeSet` returns. `createChangeSet` delegates to
  `waitForChangeSet`, which constructs its `*changeSetResult` before its very first `return` and
  returns that same non-nil pointer on every subsequent return path — including the no-op case,
  the timeout case, and the context-cancellation case. The single path that returns `nil, err`
  (a `DescribeChangeSet` API call failure) always pairs `nil` with a non-nil `err`.
- `runApply` already checks `if err != nil { return summary, err }` *before* looking at `result`,
  so by the time the code reaches the `result` check, any path that could produce `(nil, non-nil
  err)` has already returned. This means `result != nil` and "a direct stack deploy just
  succeeded" are equivalent at that point — no separate boolean needed.

`runApply` now returns immediately after merging `deliverApply`'s summary when `result == nil`,
skipping `setStackPolicy`, `applyTerminationProtection`, and `describeStackOutputs` entirely for
the publish-only and external-target outcomes. The direct-deploy path's behavior (all three
follow-up steps still run, in the same order) is unchanged.

**Bug 2 (secondary, narrowly scoped) — new helper `wrapAPICallError` in
`pkg/component/aws/cloudformation/changeset.go`, applied at 3 call sites:**

Rather than rewrapping every `fmt.Errorf("%w: %w", errUtils.ErrAwsCloudFormationAPICallFailed,
err)` call site in the package (a much larger effort spanning `executor.go`, `validate.go`,
`delete.go`, `stackset.go`, `backend.go`, `drift.go`, `get.go`, `list.go`, `observability.go` —
out of scope here and a real regression risk), this fix adds one small, targeted helper next to
the package's existing `isStackNotFoundError` pattern-recognition function (already used by
`changeset.go`, `changeset_verbs.go`, and `events.go` to special-case this exact AWS error shape):

```go
func wrapAPICallError(stackName string, err error) error {
  if !isStackNotFoundError(err) {
    return fmt.Errorf(wrapFmt, errUtils.ErrAwsCloudFormationAPICallFailed, err)
  }
  return errUtils.Build(errUtils.ErrAwsCloudFormationAPICallFailed).
    WithCause(err).
    WithExplanationf("Stack %q doesn't exist yet.", stackName).
    WithHint("Check `--target`/`-s`/component name, or run `apply` without `--target` first if you need to create the stack directly.").
    Err()
}
```

`WithCause` preserves `errors.Is` matching against both the sentinel and the original AWS error,
per the error builder's documented contract. Unrecognized AWS error shapes fall through to the
existing plain wrap unchanged — this deliberately avoids inventing a hint for error shapes this
fix hasn't specifically verified.

Applied at the 3 call sites most directly implicated by the confirmed bug (all three are
stack-scoped follow-up calls that can legitimately target a non-existent stack):

- `pkg/component/aws/cloudformation/validate.go`, `setStackPolicy`
- `pkg/component/aws/cloudformation/validate.go`, `applyTerminationProtection`
- `pkg/component/aws/cloudformation/output.go`, `describeStackOutputs` (also used by the
  standalone `output` verb, not just `runApply`'s end-of-deploy summary)

`output.go` no longer needs the `fmt` or `errUtils` imports after this change and had both
removed.

## Validation

- `go build ./...` — passes.
- `go vet ./pkg/component/aws/cloudformation/...` — passes.
- `go test ./pkg/component/aws/cloudformation/...` — passes (full package, including new tests).
- New/updated tests in `pkg/component/aws/cloudformation/executor_test.go` and
  `pkg/component/aws/cloudformation/changeset_test.go`:
  - `TestRunApply_PublishOnlyTarget_SkipsPostDeploySteps` — regression test for Bug 1. Uses a
    `MockCloudFormationClient` with **zero** `EXPECT()` calls set, so any
    `UpdateTerminationProtection`/`SetStackPolicy`/`DescribeStacks` call fails the test outright.
    The `stackSpec` sets both `TerminationProtection: true` and a non-empty `StackPolicyBody` to
    prove the gate actively skips those steps rather than merely having nothing to do.
  - `TestRunApply_Success`, `TestRunApply_SetsStackPolicy`, `TestRunApply_TerminationProtectionError`,
    `TestRunApply_DescribeOutputsError` (pre-existing, unmodified) — continue to pass unchanged,
    proving the direct-deploy path still runs all three follow-up steps exactly as before.
  - `TestWrapAPICallError_StackNotFound` — asserts the wrapped error matches both
    `errUtils.ErrAwsCloudFormationAPICallFailed` and the original AWS error via `errors.Is`, and
    carries a hint (via `cockroachdb/errors.GetAllHints`) mentioning `--target` and `apply`.
  - `TestWrapAPICallError_OtherError_PlainWrap` — asserts an unrecognized AWS error still matches
    the sentinel and original error, but carries **no** hint.
- `atmos lint --changed` (via `GOTOOLCHAIN=go1.26.6 ./custom-gcl run --config=.golangci.yml
  --allow-serial-runners --new-from-rev=origin/main`, working around a known Homebrew-Go/go.mod
  toolchain mismatch in this environment): zero findings on any file touched by this fix. Three
  pre-existing `dupl` findings surfaced in `executor_test.go` (among three untouched, already
  near-identical `TestRunApply_*Error` tests) and one unrelated `godot` finding in
  `internal/exec/yaml_func_terraform_output.go` — both pre-date this change (confirmed via `git
  diff origin/main`, which shows the whole `executor_test.go` file as new relative to `origin/main`
  since this feature branch hasn't merged yet) and are out of this fix's scope.

## Follow-ups

None.
