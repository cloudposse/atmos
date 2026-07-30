# Fix: Schema-driven deprecation warnings and compatibility

**Date:** 2026-07-23

## Summary

Legacy Atmos config and stack fields that were at risk of being rejected or silently
unsupported as the JSON schemas tightened now stay valid and emit non-failing deprecation
warnings with replacement guidance, in `atmos validate config`, `atmos validate stacks`, and
the Atmos LSP.

## Context

As the generated JSON schemas were tightened (the "1.224 rules" referenced by this branch's
name, `osterman/fix-terraform-1-224-rules`), several previously-supported legacy fields were
at risk of failing schema validation or losing runtime behavior with no warning: `stacks.name_pattern`,
Helmfile `helm_aws_profile_pattern`/`cluster_name_pattern`, auth `provider_type`, `version`
provider `type`, `settings.terminal.no_color`, legacy `settings.docs.*`, component-level
`depends_on`, and Terraform S3 backend/remote-state compatibility options. The fix (see
`.context/plans/schema-driven-deprecation-warnings-and-compatibili.md` for the design) keeps
all of this configuration valid, annotates each field with standard JSON Schema `deprecated:
true` plus a custom `x-atmos-replacement` hint, and surfaces the annotations as warnings
(never errors) everywhere schema validation runs, so users get a migration nudge instead of a
broken config.

## Changes

- Added `pkg/validator/deprecation.go`: a reusable schema walker
  (`LoadDeprecatedYAMLSchema`/`FindYAMLFields`, plus a `FindDeprecatedYAMLFields` convenience
  wrapper) that resolves local `$ref`s and `allOf`/`anyOf`/`oneOf` branches, walks authored
  YAML against the parsed schema, and returns `DeprecatedField{Path, Replacement}` findings,
  coalescing nested parent/child warnings down to the most specific path.
  `FormatDeprecatedField` renders the Markdown warning message.
- Tagged the deprecated struct fields in `pkg/schema/schema.go`, `pkg/schema/schema_auth.go`,
  and `pkg/schema/version.go` with `jsonschema_extras:"deprecated=true,x-atmos-replacement=..."`.
  Regenerated `pkg/datafetcher/schema/atmos/config/1.0.json`,
  `pkg/datafetcher/schema/atmos/manifest/1.0.json`, and
  `pkg/datafetcher/schema/stacks/stack-config/1.0.json` carry the matching `deprecated`/
  `x-atmos-replacement` annotations, and restore `depends_on` (pointing at
  `dependencies.components`) plus Terraform `dependencies`/backend compatibility fields in the
  manifest and stack-config schemas.
- Wired the warnings into validation surfaces: `internal/exec/validate_schema.go`
  (`deprecationDiagnostics`, called from both the aggregate report path and
  `printValidation`), `internal/exec/validate_stacks.go` (`warnDeprecatedStackFields`), and
  `cmd/validate_schema.go` (exit code stays `0` when a report has only diagnostics, no errors).
  `pkg/lsp/server/config_schema.go` surfaces the same findings as LSP warning diagnostics.
- Restored component-level `depends_on` compatibility in dependency resolution:
  `internal/exec/dependency_parser.go` (new `hasDependencies` helper, falls back from
  `settings.depends_on` to component-level `depends_on`) and
  `internal/exec/describe_dependents.go` (same fallback in `getComponentDependencies`).
  Precedence is `dependencies.components` → `settings.depends_on` → direct component
  `depends_on`.
- Added a regression fixture at `tests/fixtures/scenarios/deprecated-config-compatibility/`
  (`atmos.yaml` plus `stacks/deploy/dev.yaml`) exercising every supported deprecated field, and
  a CLI test case at `tests/test-cases/deprecated-config-compatibility.yaml` asserting `atmos
  validate config` and `atmos validate stacks` both exit `0` and print the expected deprecation
  warnings on stderr.
- Added unit tests: `pkg/validator/deprecation_test.go`,
  `pkg/config/schema/generate_test.go` (`TestGenerateMarksDeprecatedConfigurationFields`),
  `pkg/lsp/server/config_schema_test.go`, `internal/exec/dependency_parser_test.go`, and
  `internal/exec/describe_dependents_test.go`.

## Validation

- `go build ./...` — passed, no errors.
- `go test ./pkg/validator/... ./pkg/config/schema/... ./pkg/lsp/server/...` — passed (`ok` for
  all three packages).
- `go test ./internal/exec/... -run 'TestDependencyParser_ParseComponentDependencies|TestGetComponentDependencies'`
  — passed, all subtests including the new `depends_on` fallback cases.
- `go test ./tests -run 'TestCLICommands/deprecated_config_compatibility'` — passed both new
  CLI scenarios (`deprecated_config_compatibility_validates_config` and
  `..._validates_stacks`), confirming `atmos validate config`/`atmos validate stacks` exit `0`
  and emit the expected deprecation warnings.

## Follow-ups

None. The design doc's note that "CI workflow modernization patterns remain a separate
semantic inspection effort" is scope framing for this change, not a promised follow-up, so it
does not need a tracked issue.
