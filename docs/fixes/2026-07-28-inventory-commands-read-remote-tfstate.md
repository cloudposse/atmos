# Cross-Account Terraform State Reads Abort `atmos list` / `atmos describe stacks`

**Date:** 2026-07-28 **Severity:** High — in a multi-account repository, commands that walk
every stack abort with an S3 `AccessDenied` reading another stage's state bucket, making
inventory and describe unusable
**Issue:** https://github.com/cloudposse/atmos/issues/2566 (reported as a 1.217
regression against 1.216) **Reproducer:**
`internal/exec/yaml_func_terraform_state_degradation_test.go`

______________________________________________________________________

## Why this is a fix doc (and not a blog post / changelog entry)

This is a `patch` bug fix. There is no new command or flag — an existing error mode
(`--error-mode=warn`, already the default) is corrected to cover a class of failure it was
always meant to handle. Per the repo's label decision tree that makes it a `patch`, which
does not require a `website/blog/` post or a roadmap milestone.

## Symptom

A multi-stage repository where each stage lives in a distinct AWS account with its own
state bucket, and a `customer` component reads a var out of the stage's `global`
component:

```yaml
data_bucket_name: !terraform.state global "{{ .vars.environment }}-{{ .settings.context.instance }}-global" data_bucket_name
```

Listing the inventory with credentials for one stage tries to read every *other* stage's
bucket, and the first unreadable one kills the command:

```console
$ atmos list stacks --identity prd-bastille-access
 Error: failed to read Terraform state for component `global` in stack `dev-pen`
   in YAML function: `!terraform.state global dev-pen data_bucket_name`
   operation error S3: GetObject, ... StatusCode: 403, api error AccessDenied
```

Reversing the identity just reverses which bucket is unreachable. There is no identity that
works, because the scan spans every account while any one identity holds credentials for
exactly one.

## Root Cause

Atmos already has the right mechanism for this. `--error-mode=warn` (the default)
substitutes `degradation.AtmosComputedValue{}` — rendered as `(computed)` — for a value it
could not resolve, records a warning, and lets the walk finish. The classifier deciding
what qualifies is `isRecoverableInWarnMode` (`internal/exec/yaml_func_terraform_state.go`),
and it recognized only two errors:

```go
func isRecoverableInWarnMode(err error) bool {
	return isRecoverableTerraformError(err) // ErrTerraformStateNotProvisioned / ErrTerraformOutputNotFound
}
```

A backend read that *failed* — the cross-account `AccessDenied` — was deliberately excluded:

> Authentication, credential-refresh, network, and backend API failures must remain visible
> rather than silently using a fallback value.

That is a reasonable default for a single-account repository, where a credential failure
really is a defect. It is the wrong call for the multi-account topology Atmos is built to
orchestrate, where a command that walks every stack will *always* meet backends the current
identity cannot read. There, an unreadable backend is the topology working as designed, and
`ProcessCustomYamlTagsLenient` never got the chance to substitute `(computed)` for it.

## Fix

Widen `isRecoverableInWarnMode` to include `ErrReadTerraformState`, the wrapper
`GetTerraformState` puts around every backend read failure — but subtract the failures that
wrapper also carries which are defects in the repository's own manifests rather than
conditions of the environment:

```go
func isRecoverableInWarnMode(err error) bool {
	if isTerraformStateManifestDefect(err) {
		return false
	}

	return isRecoverableTerraformError(err) ||
		errors.Is(err, errUtils.ErrReadTerraformState)
}
```

`atmos list stacks` and an unfiltered `atmos describe stacks` now render the reachable
values and `(computed)` for the rest, with an end-of-command summary of what was degraded
and the real cause available at `--logs-level=Debug`.

### Why the subtraction is necessary

`ErrReadTerraformState` is not a synonym for "environmental failure". `GetTerraformState`
wraps it around *everything* `GetTerraformBackend` and the static-backend path can return,
which includes four failures that are unambiguously mistakes in the stack manifests:

| Failure | Sentinel | Why it must stay fatal |
| --- | --- | --- |
| `backend_type` names a backend Atmos does not implement | `ErrUnsupportedBackendType` | A typo; no error mode makes it work. |
| Retrieved state is not parseable Terraform state | `ErrProcessTerraformStateFile` | Degrading hides state corruption. |
| A `static` backend does not declare the requested output | `ErrStaticRemoteStateOutputMissing` | The outputs are written in the manifest; a missing key is a typo. |
| The YQ expression failed against state Atmos *did* retrieve | `ErrEvaluateTerraformBackendVariable` | The read succeeded; the expression is wrong. |

Degrading any of these substitutes `(computed)` for what is really a typo, reports success,
and exits 0 — the user gets a plausible-looking value with no signal at all that something
is wrong. That is strictly worse than the abort this fix set out to remove, because the
abort at least said what happened.

Telling them apart required one supporting change: `GetTerraformState` formatted its cause
with `%v`, which flattened every backend failure into a single unmatchable string. It now
uses `%w`, so `errors.Is` can reach the real sentinel. The static-backend "output does not
exist" case had no sentinel of its own at all and now carries
`ErrStaticRemoteStateOutputMissing`.

The change is deliberately narrow in three ways:

- **Warn/silent mode only.** `--error-mode=strict` still surfaces every one of these, which
  is the entire reason to select it.
- **`isRecoverableTerraformError` is untouched.** That is the narrower classifier gating the
  YQ `//` default operator, so `!terraform.state … // "fallback"` still refuses to paper
  over a credential failure with its literal default. The two classifiers were already split
  for exactly this kind of divergence.
- **Environmental failures only.** See the table above.

`describe stacks` additionally keeps the actionable hints added earlier in this PR, now
leading with `--error-mode`. They are gated on `isRecoverableInWarnMode` rather than merely
on strict mode, because every hint is a way to stop Atmos failing on an unreadable backend
and the lead one promises that dropping `--error-mode=strict` lets the command continue. For
a failure warn mode also aborts on — a circular `!terraform.state` chain, a corrupt state
file, a typo'd `backend_type` — that promise is false, and the five suggestions bury the
error's own remediation. The hints now appear on exactly the failures they can resolve.

## What this replaces

An earlier revision of this PR tried to fix the symptom in `cmd/list` by skipping the
credential-backed YAML functions *unconditionally* for inventory commands. Review
(osterman, #2820) rejected that:

> I don't believe this is the correct fix because there are legitimate use cases for having
> columns based on customizable views supported by Atmos to refer to this state. What we've
> been doing throughout is, instead, gracefully degrading and emitting placeholder computed
> values instead. This should already be supported everywhere.

That is right, and it locates the defect one layer down: the problem was never that these
commands resolve state when a view asks for it, it is that a failed resolution had no graceful
path. Unconditionally skipping the reads would also have diverged `list` from the rest of
Atmos, which degrades rather than blanks values out. So the graceful-degradation fix lives in
the classifier, where every caller of the describe pipeline benefits rather than only the four
commands that were patched by hand.

What survives in `cmd/list` is the narrower, demand-driven guard
(`skipCredentialBackedYAMLFunctionsForInventory`): built-in identity-only inventory output —
no `--identity`, no custom column, query, filter, or upload — stays credential-free, because
nothing that output renders can carry a credential-backed value, so there is nothing to read.
The moment a view actually asks for one (an explicit identity, or a customized
column/query/filter/upload), the read runs and now degrades instead of aborting.

## Verification

- `internal/exec/yaml_func_terraform_state_degradation_test.go` drives
  `processNodesWithContext` with the package-level `stateGetter` swapped for a generated
  `MockTerraformStateGetter` that fails with a wrapped `AccessDenied` — deterministic, no
  network, no credentials.
  - `TestProcessNodesWithContext_DegradesCrossAccountAccessDenied` asserts the walk
    completes, the value becomes `AtmosComputedValue{}` rendering as `(computed)`, unrelated
    values are untouched, and exactly one warning carries the real cause. The mock's
    `MinTimes(1)` enforces that the read was actually **attempted** — the property that
    distinguishes this fix from the skip-based approach it replaces.
  - `TestProcessNodesWithContext_StrictModeStillFailsOnAccessDenied` is the negative path:
    strict mode must still fail, with the cause matchable by `errors.Is`.
  - `TestProcessNodesWithContext_FatalErrorsStillAbortInWarnMode` proves the widening is
    scoped — a bad YQ expression still aborts even with a warn callback installed, and
    reports no degradation.
- `TestIsRecoverableInWarnMode` (`yaml_func_terraform_state_yq_defaults_test.go`) was updated
  rather than shadowed, since it encoded the previous contract. Every case now also asserts
  that a backend failure does **not** satisfy `isRecoverableTerraformError`, so widening warn
  mode can never silently widen the YQ default operator.
- `go test ./internal/exec/...` — the only failure is the pre-existing `TestCopyFile_FailCreate`,
  which fails identically on a clean tree because the sandbox runs as root and read-only file
  modes do not block writes.

## Recommendations

- **Cost, not correctness, is now the open question.** Default identity-only inventory output
  stays credential-free and reads nothing. But once a view asks for a credential-backed value —
  an explicit `--identity`, or a custom column, query, filter, or upload — a multi-account
  `atmos list stacks` still *attempts* every cross-account read before degrading, and each
  unreachable backend costs its full retry budget. Correct, but potentially slow on a large
  repository. Resolving those values lazily — only when something is about to render them — is
  the real answer, and is worth tracking separately.
- **`!terraform.output` is not covered and should be.** It is the closest sibling to the
  function this fixes and fails the same way in the same topology, but it shells out to
  `terraform output` and wraps every subprocess failure in `ErrTerraformOutputFailed` —
  missing binary, HCL error, timeout, and access denial alike. Degrading that wholesale would
  hide real defects, so it is deliberately left strict here. Closing the gap means classifying
  the subprocess failure (or having the executor surface an access-denied sentinel) before
  degrading it.
- **Consider whether `!store` and `!aws.*` need the same treatment.** The other
  credential-backed YAML functions can fail the same way in the same topology.
