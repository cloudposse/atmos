# Fix: Terraform 1.15+ `const`-variable module source interpolation no longer fails to parse

**Date:** 2026-08-10

## Summary

Atmos failed to load any Terraform component whose `module` block used variable interpolation in
`source` (e.g. `source = "./mods/${var.org}"`) when the variable was declared `const = true` — valid
syntax under Terraform 1.15+ (released April 2026), analogous to OpenTofu 1.8+'s existing support for
the same pattern. Commands that resolve the component (`atmos describe component`, `atmos terraform
plan`, etc.) failed before any Terraform/OpenTofu binary was ever invoked, with:

```
failed to load terraform component
The Terraform component '<name>' contains invalid HCL code at <path>:<line>.
Variables not allowed: Variables may not be used here.
```

Fixes [#2913](https://github.com/cloudposse/atmos/issues/2913).

## Context

Atmos pre-parses every Terraform component with `terraform-config-inspect`
(`tfconfig.LoadModule`, called from `internal/exec/utils.go`) before running any Terraform/OpenTofu
command. That library decodes a module's `source` attribute via
`gohcl.DecodeExpression(attr.Expr, nil, &source)` — a **nil `hcl.EvalContext`** — so any variable
reference there always produces the HCL diagnostic "Variables not allowed", regardless of whether
the configured tool/version actually supports evaluating it.

Atmos already tolerated this exact diagnostic for **OpenTofu 1.8+** (added in PR #1756, closing
#1753): `IsOpenTofu()` (`internal/exec/terraform_detection.go`) detects the configured binary, and
`isKnownOpenTofuFeature()` pattern-matched the diagnostic — but the skip was only applied when
`IsOpenTofu()` returned true. At the time, this was correct: Terraform had no equivalent capability.

Terraform 1.15 (April 29, 2026) added the identical capability via a new `const = true` variable
attribute, resolved at `terraform init` time. Because the existing skip was gated on OpenTofu
detection, plain-Terraform users hitting this now-valid syntax still got a hard failure. The
diagnostic text is identical either way — `terraform-config-inspect` can't evaluate the expression,
so it can't distinguish "valid under a modern tool" from "genuinely invalid"; Atmos already accepted
that ambiguity unconditionally for OpenTofu (it never checked the OpenTofu version was actually
≥1.8). The fix extends that same leniency to Terraform by decoupling the skip from tool detection
entirely.

Also investigated per user request: whether this affects Atmos's SBOM generation, since it also
reads Terraform module data. Confirmed it does not — see Changes below.

## Changes

- `internal/exec/terraform_detection.go`: renamed `isKnownOpenTofuFeature` →
  `isKnownModuleSourceInterpolationDiagnostic` (and its local pattern slice), with a doc comment
  explaining the real, tool-agnostic mechanism (a `terraform-config-inspect` static-parser
  limitation, not an OpenTofu-specific feature gate). `IsOpenTofu()` and its detection cache are
  intentionally left in place even though this path no longer calls them — retained as generically
  useful, already well-tested infrastructure; removing ~130 lines of tested detection code was out
  of scope for this fix.
- `internal/exec/utils.go`: removed the `effectiveConfig`/command-override clone that existed only to
  feed `IsOpenTofu()`; the skip condition is now `!isKnownModuleSourceInterpolationDiagnostic(diagErr)`
  with no tool-detection gate; renamed the `component_info` flag `validation_skipped_opentofu` →
  `validation_skipped_module_source_interpolation`.
- `internal/exec/terraform_detection_test.go`: renamed test/benchmark functions to match
  (`TestIsKnownOpenTofuFeature*` → `TestIsKnownModuleSourceInterpolationDiagnostic*`, etc.); all
  existing test-case tables/assertions unchanged.
- `internal/exec/opentofu_module_source_interpolation_test.go`: updated the assertion to read the
  renamed `validation_skipped_module_source_interpolation` flag; the rest of the OpenTofu regression
  test (fixture, `command: "tofu"`, nested-var assertions) is unchanged and still passes.
- Added `tests/fixtures/scenarios/terraform-module-source-interpolation/` and
  `internal/exec/terraform_module_source_interpolation_test.go`, reproducing the exact #2913
  scenario on plain `terraform` (no `command:` override, no OpenTofu). Confirmed this test fails
  against pre-fix code with the exact reported error, and passes post-fix.
- `docs/prd/opentofu-module-source-interpolation.md`: appended an "Update (2026-08-10)" section
  documenting the generalization and annotating the doc's now-superseded "OpenTofu-specific... NOT
  supported in HashiCorp Terraform" claim.
- SBOM investigation (no functional change needed): `pkg/sbom/terraform.go` gathers module evidence
  via `terraform modules -json` against an already-initialized directory, never via
  `terraform-config-inspect`, so it never hits this diagnostic. Empirically verified against real
  Terraform 1.15.6 that `terraform modules -json` (and the underlying
  `.terraform/modules/modules.json` manifest) already reports the **resolved** module source (e.g.
  `./mods/acme`), not the unresolved `${var.org}` template — Atmos's SBOM pipeline was already
  recording the effective source correctly. Added
  `pkg/sbom/terraform_test.go::TestAppendModulesForDirectoryRecordsResolvedDynamicModuleSource` as a
  permanent regression guard for this invariant.

## Validation

- `go test ./internal/exec/... -run 'TestTerraformModuleSourceInterpolation|TestOpenTofuModuleSourceInterpolation|TestIsKnownModuleSourceInterpolationDiagnostic|TestIsOpenTofu'` — all pass.
- Confirmed the new test fails against pre-fix code with the exact issue #2913 error text, then
  passes after the fix (reproduce → fix → verify, per the mandatory bug-fix workflow).
- Full `go test ./internal/exec/...` — pass (518s).
- `go test ./pkg/sbom/...` — pass, including the new SBOM guard test.
- `go test ./internal/exec/... -run TestHCLSyntaxError` — pass; confirms the unrelated issue #1864
  error-formatting path (genuine HCL syntax errors must still fail loudly) is unaffected, i.e. the
  fix didn't over-broaden the leniency.
- `go build ./...` — clean.
- `./custom-gcl run --config=.golangci.yml --new-from-rev=origin/main --allow-serial-runners` (the
  real PR lint gate) — 0 issues, after fixing one `godot` finding (comment capitalization).
- Empirically verified the SBOM claim above by running real `terraform init` (v1.15.6) + `terraform
  modules -json` against a scratch fixture with a `const`-variable dynamic module source, and
  inspecting the actual JSON output.

## Follow-ups

None.
