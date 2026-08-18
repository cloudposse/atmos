# Fix: `--type=auto` infers instead of nagging ("auto nag" fix)

**Date:** 2026-08-10

## Summary

`atmos config set` and `atmos stack set --type=auto` used to fall back to a plain string for
any brand-new key and merely warn that the value "looked like" a bool/int/float, without ever
acting on that judgment. `--type=auto` now actually infers the type in that case: from the
target Terraform component's declared variable type (`stack set` only, `vars.*` paths), then
from the new value's own shape, only falling back to a warned string when the value doesn't
look like anything but a string.

## Context

Erik flagged the existing behavior as "auto nag": the code already computed
`atmosyaml.LooksNonString(value)` to decide whether to warn, then discarded that judgment and
wrote a literal string anyway — e.g. `atmos stack set vars.replicas 5` on a brand-new key wrote
the string `"5"` and printed a warning, instead of just writing the integer `5`.

Two follow-up design questions were resolved with Erik before implementing:

- **Should `stack set` tap into the target Terraform component's declared variable type?**
  Atmos already parses each component's `variables.tf` via `terraform-config-inspect` as a
  side effect of resolving the component for `stack set` (stashed at
  `component_info.terraform_config`), so this signal was available for free. Yes — added as the
  most authoritative tier, ranked above even an already-stored value.
- **Should a Terraform-declared type retype an already-stored value that disagrees with it**
  (e.g. `variable "replicas" { type = number }` but the manifest has `replicas: "5"` quoted)?
  Yes, confirmed explicitly — Terraform's declared type wins. Because that changes state beyond
  "fill in a blank," a real retype now prints an informational notice (`ui.Infof`, not a
  warning) rather than happening silently.

While confirming the new regression tests failed for the right reasons pre-implementation, an
apparent additional bug surfaced: `yqlib`'s encoder strips quotes it judges "unnecessary" from
values like `nan`/`inf`/`yes`/`no`/`on`/`off` even when written via `--type=string`. Investigated
further and this turned out to be a false alarm — `gopkg.in/yaml.v3` (Atmos's actual downstream
config/stack loader) and `pkg/yaml`'s own `GetType`/`Get` both resolve those unquoted words back
to plain strings (verified directly against `yaml.v3`'s `resolveMap`, which only special-cases
`true/false/null` and the dotted `.nan`/`.inf` forms, not the bare words). An initial fix
(forcing double-quote style on every string value) was implemented, found to break existing
tests expecting unquoted plain strings (e.g. `region: us-east-1`), and was reverted once the
false alarm was confirmed empirically.

## Changes

- `pkg/yaml/typed.go`: added `GuessNumericType` (tries `buildValidatedRHS` as int then float,
  reusing the already-validated write path so a guess is guaranteed to write successfully; fails
  closed to `("", false)` on NaN/Infinity per the existing `buildValidatedRHS` rejection) and
  `GuessScalarType` (bool via literal `true`/`false` match — not `strconv.ParseBool`, which would
  also grab `"1"`/`"0"` away from the int branch — then delegates to `GuessNumericType`).
- `pkg/stack/var_type.go` (new): `VarNameFromRelPath` (recognizes a bare top-level `vars.<name>`
  path) and `InferVarType` (maps a Terraform-declared HCL type string — `string`/`bool`/`number`/
  `list(...)`/`map(...)`/`object(...)`/`tuple(...)`/`set(...)` — plus the raw CLI value to an
  `atmosyaml.TypeXXX`).
- `internal/exec/terraform_declared_var_type.go` (new): `TerraformDeclaredVarType` reads the
  same `component_info.terraform_config` map shape `terraformSensitiveVarKeys` already reads,
  returning the declared HCL type text for a given variable name.
- `cmd/stack/operations.go`: `editTarget` gained `terraformVarType`; `resolveEditTarget` populates
  it. `effectiveStackValueType`'s tier order is now: explicit `--type` → Terraform-declared type →
  existing file value → merged/inherited value → shape-guess → string fallback. Returns a new
  `stackValueTypeResult` struct (bundling `valType`/`resolved`/`retypedFrom`, to stay within this
  repo's 3-return-value lint limit) instead of multiple bare returns. `runStackSet` prints an
  `ui.Infof` retype notice when `retypedFrom` is non-empty.
- `cmd/config/operations.go`: `effectiveValueType` gained a shape-guess tier before the final
  string fallback (schema and existing-value tiers unchanged — `cfg.InferValueType` already does
  full reflection-based schema inference and needed no change).
- `warnIfSilentlyStoredAsString` (both copies): logic unchanged, but now reachable only for the
  narrow case where `GuessScalarType` itself fails closed (the bare `"nan"` literal) — doc
  comments updated to describe the new, much narrower scope.
- `tests/fixtures/scenarios/config-field-test/components/terraform/mock/main.tf`: added
  `quota`/`instance_count` (`number`), `feature_flag` (`bool`), `allowed_cidrs` (`list(string)`)
  variable declarations.
- `tests/fixtures/scenarios/config-field-test/stacks/catalog/mock.yaml`: added `quota: "5"`
  (deliberately quoted) to exercise the Terraform-type-wins-over-existing-value retype case.
- Added regression tests across `pkg/yaml`, `pkg/stack`, `internal/exec`, `cmd/stack`, and
  `cmd/config`, written before the implementation per this repo's test-first bug-fixing workflow
  (confirmed failing against stub implementations first). Two pre-existing tests
  (`TestRunStackSet_ExplicitFile_AutoFallsBackToStringForNewKey`,
  `TestConfigSetCommand_AutoFallsBackToStringForNewKey`) were rewritten since their old assertion
  (a numeric-looking new value stays a quoted string) was exactly the nag behavior being fixed.
- `website/docs/cli/commands/config/config-set.mdx`,
  `website/docs/cli/commands/stack/config/set.mdx`, `website/docs/cli/commands/stack/stack-set.mdx`:
  updated the `--type=auto` explanation and examples for the new tier chain. `cmd/config/operations.go`
  / `cmd/stack/operations.go`: updated the `Long` command descriptions and `--type` flag help text
  to match.

## Validation

- `go build ./...` — clean.
- `go test ./pkg/yaml/... ./pkg/stack/... ./internal/exec/... ./cmd/config/... ./cmd/stack/...` —
  all pass, including every new/rewritten test individually verified via `-v`.
- Live-drove every scenario against a real `./build/atmos` binary run against a scratch copy of
  `tests/fixtures/scenarios/config-field-test`: brand-new `vars.instance_count 7` (Terraform
  `number`) writes unquoted with no retype notice; `vars.quota 10` (existing quoted `"5"`, same
  declared type) retypes to unquoted `10` and prints "Retyped vars.quota from string to int, per
  variables.tf"; `vars.allowed_cidrs '["a","b"]'` (declared `list(string)`) refuses with the
  existing non-scalar hint; a brand-new undeclared `vars.retry_budget 5` still infers `TypeInt` via
  the shape-guess tier; `config set settings.debug_enabled true` (unmodeled path) infers `TypeBool`;
  `nan` on both `config set` and `stack set` still warns and writes a (safely resolvable, even if
  unquoted) string.
- `atmos fix lint` (patch-scoped, `--new-from-rev=origin/main`) — 0 issues, after fixing 4 findings
  (2 `godot` comment-sentence issues, 1 `nestif` nesting-depth issue, 1 `function-result-limit`
  violation resolved by introducing `stackValueTypeResult`, 1 `cyclomatic` complexity violation in
  `InferVarType` resolved by extracting `isNonScalarHCLType`).

## Follow-ups

None.
