# Fix: module-source-interpolation skip no longer swallows unrelated HCL errors

**Date:** 2026-08-10

## Summary

A field-test pass on the module-source-interpolation generalization (see
[2026-08-10-terraform-module-source-interpolation.md](2026-08-10-terraform-module-source-interpolation.md))
found that `atmos describe component` could silently accept a component with a genuine, unrelated
HCL error — no error returned, `terraform_config` set to `nil` — as long as the error co-occurred,
in the same file, with the known-safe "Variables not allowed" module-source diagnostic and that
diagnostic happened to sort first. Confirmed with a real repro: `atmos describe component` exited
`0` on a component with an invalid `output.sensitive` value (a list, not a bool) declared after a
valid `module.source` interpolation; the same file with the two blocks swapped correctly failed.

## Context

`internal/exec/utils.go`'s `processStacks()` decides whether to skip `terraform-config-inspect`
validation by pattern-matching `diags.Err().Error()` against `"Variables not allowed"`. But
`tfconfig.Diagnostics.Error()` (the vendored `terraform-config-inspect` library) only renders the
**first** diagnostic's Summary/Detail — any others collapse to `"(and N other messages)"` with no
text at all (`$(go list -m -f '{{.Dir}}' github.com/hashicorp/terraform-config-inspect)/tfconfig/diagnostic.go:45-54`).
So when the module-source diagnostic happened to be `diags[0]`, the pattern match succeeded and the
code unconditionally skipped validation for the *entire* diagnostics set — discarding any other,
completely unrelated diagnostic in the same file, not just hiding its detail.

This isn't a full silent-forever bug: `atmos terraform init`/`plan` with a real binary does
eventually hit the same underlying error later. But any workflow that only calls
`describe`/`validate`-adjacent Atmos commands without running `init` (CI describe/affected/drift
paths) would never see it, and even when it does surface, it's via a later, less-integrated
`init`/`plan` failure instead of Atmos's own localized, hinted `ErrFailedToLoadTerraformComponent`
error.

## Changes

- `internal/exec/terraform_detection.go`: added `allDiagnosticsAreModuleSourceInterpolation(diags
  tfconfig.Diagnostics) bool` and its helper `diagnosticPositionKey()`. These group error-severity
  diagnostics by source position (`file:line`) and require every position group to contain at least
  one diagnostic matching the known pattern. This tolerates `terraform-config-inspect`'s own
  companion diagnostic at the *same* position as the module-source one (e.g. `"Unsuitable value:
  value must be known"`, a side effect of decoding `module.source` with a nil `hcl.EvalContext`),
  while still treating a genuine error at a *different* position as unrelated and unsafe to skip.
  `isKnownModuleSourceInterpolationDiagnostic()` (the original text-pattern check) is unchanged and
  reused per-diagnostic inside the new function.
- `internal/exec/utils.go`: the skip decision in `processStacks()` now calls
  `allDiagnosticsAreModuleSourceInterpolation(diags)` (the full diagnostics slice) instead of
  `isKnownModuleSourceInterpolationDiagnostic(diagErr)` (the collapsed, first-diagnostic-only
  string).
- Added `tests/fixtures/scenarios/terraform-module-source-interpolation-mixed-diagnostics/`: a
  `module.source` interpolation declared before an `output` block with a genuinely invalid
  `sensitive` value.
- Added `internal/exec/terraform_module_source_interpolation_test.go::TestTerraformModuleSourceInterpolationDoesNotSwallowUnrelatedErrors`
  — written first and confirmed failing against pre-fix code (per the mandatory bug-fix workflow),
  passes post-fix.
- Added `internal/exec/terraform_detection_test.go::TestAllDiagnosticsAreModuleSourceInterpolation`
  — unit-level table test covering: single known diagnostic, known diagnostic + same-position
  companion, known diagnostic + different-position genuine error, only a genuine error, known
  diagnostic + unrelated warning, warnings-only (no errors), and a diagnostic with no position.

## Validation

- New tests written first and confirmed failing pre-fix, then passing post-fix (reproduce → fix →
  verify).
- `go test ./internal/exec/... -run 'ModuleSourceInterpolation|IsKnownModuleSourceInterpolationDiagnostic|IsOpenTofu|AllDiagnosticsAreModuleSourceInterpolation'` — all pass, including the
  original happy-path (`TestTerraformModuleSourceInterpolation`) and OpenTofu
  (`TestOpenTofuModuleSourceInterpolation`) regression tests, confirming this change didn't
  reintroduce the original #2913 failure or break the OpenTofu path.
- Full `go test ./internal/exec/...` (593s) and `go test ./pkg/sbom/...` — pass.
- `atmos lint --changed` (patch-scoped `custom-gcl` gate) — 0 issues.
- `go build ./...` — clean.
- Manually re-verified against a freshly built `./build/atmos`: the mixed-diagnostics fixture now
  correctly fails with `ErrFailedToLoadTerraformComponent` and a file:line hint; the original
  single-diagnostic fixture still succeeds with `vars.org == "acme"`.

## Follow-ups

The underlying pattern match is still text-based (`"Variables not allowed"`), not scoped to
confirming the diagnostic's position is actually a `module` block's `source`/`version` attribute.
`terraform-config-inspect` uses the same `gohcl.DecodeExpression(attr.Expr, nil, ...)` call (and
thus produces the identical diagnostic) for several other attributes that were never a real
supported variable-interpolation feature — e.g. `output.description`, `variable.sensitive`,
`provider.alias`. Confirmed with real Terraform 1.15.6 that `output.description = var.x` is
genuinely invalid HCL (Terraform itself rejects it), yet `atmos describe component` accepts it
silently today, only failing later and less clearly at `terraform init`/`plan`. A real fix would
re-parse the failing file with `hclsyntax` to find the actual `module.source`/`version` attribute
ranges and require diagnostics to fall within one of them, rather than trusting diagnostic text
alone. No GitHub issue has been opened for this at the user's explicit request — noted here only as
a known, untracked gap.
