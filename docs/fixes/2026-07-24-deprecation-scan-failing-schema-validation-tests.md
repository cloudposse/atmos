# Fix: Deprecation scan no longer fails schema validation on read/fetch errors

**Date:** 2026-07-24

## Summary

`atmos validate schema`/`config` (and the underlying `ValidateAtmosSchemaReport` used by
`validate stacks`' aggregate report) no longer hard-fail when the new deprecation scanner can't
re-read a file or resolve a schema source. This was breaking CI (`Acceptance Tests (linux)` and
`Acceptance Tests (macos)`) because several `internal/exec` unit tests exercise the validator
through mocks and never write the fake files/schemas they reference to disk.

## Context

`docs/fixes/2026-07-23-schema-deprecation-warnings-and-compatibility.md` added
`deprecationDiagnostics` in `internal/exec/validate_schema.go`, wired into both
`validateAtmosSchemaReport` and `printValidation`. Unlike its sibling,
`warnDeprecatedStackFields` in `internal/exec/validate_stacks.go` (which already logs and
continues on a read/scan failure), `deprecationDiagnostics` returned a hard `error` that
propagated all the way up and aborted validation — even though the primary schema check
(`av.validator.ValidateYAMLSchema`, itself mockable) had already succeeded.

CI surfaced this as 6 failing subtests in `internal/exec`, all in
`internal/exec/validate_schema_test.go`:
`TestExecuteAtmosValidateSchemaCmd/successful_validation`,
`.../validation_errors`, `.../built-in_config_entry_validates_atmos.yaml_by_default`,
`.../source_key_config_targets_only_the_built-in_entry`,
`.../user-configured_config_entry_overrides_the_built-in_defaults`, and
`TestValidateAtmosSchemaReport/collects_diagnostics_with_source_positions`. Each uses a mocked
`validator.Validator`/`filematch.FileMatcher` returning file paths (e.g. `atmos.yaml`) that
don't exist on disk, or a placeholder schema source (`schema.json`) the data fetcher can't
resolve — both now-legitimate reasons for the deprecation scan to no-op, not hard-fail the whole
command.

## Changes

- `internal/exec/validate_schema.go`: `deprecationDiagnostics` now returns
  `[]validation.Diagnostic` only (no `error`). A failed `os.ReadFile` or a failed
  `validator.FindDeprecatedYAMLFields` call logs at debug level and returns `nil` diagnostics
  instead of propagating an error, matching `warnDeprecatedStackFields`'s established
  best-effort pattern. Updated both call sites (`validateAtmosSchemaReport`, `printValidation`)
  to drop the now-removed error handling.

## Validation

- `go build ./...` — passed.
- `go test ./internal/exec/... -run 'TestExecuteAtmosValidateSchemaCmd|TestValidateAtmosSchemaReport' -v`
  — all 6 previously-failing subtests now pass.
- `go test ./internal/exec/...` (full package) — passed, no regressions.
- `go test ./pkg/validator/... ./pkg/lsp/server/... ./cmd/...` — passed.
- `go test ./tests -run 'TestCLICommands/deprecated_config_compatibility'` — passed; confirms
  the graceful-degrade change didn't silently disable the feature — deprecation warnings still
  render correctly for real files against real schemas.
- `gofumpt -l internal/exec/validate_schema.go` and `go vet ./internal/exec/...` — clean.

## Follow-ups

None.
