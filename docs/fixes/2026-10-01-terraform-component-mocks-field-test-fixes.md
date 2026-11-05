# Fix: `--use-mocks` fallback field-test findings

**Date:** 2026-10-01

## Summary

A hands-on field test of the `--use-mocks` fallback mode (PR #3244) found silent wrong values, a flag
regression, and several confusing errors. This fix makes fallback mocks fill keys missing from real
map outputs, restores the boolean flag forms, reports `--use-mocks always` (space-separated) with a
hint, names `ATMOS_USE_MOCKS` in its errors, hints at `--use-mocks=always` when a fallback lookup
fails hard, validates `mocks.mode` from atmos.yaml at config load, and corrects the docs.

## Context

The field test ran the real binary against a local-backend fixture and a Floci S3 emulator, and
compared it with the released `atmos 1.229.0`:

- Real `config = { a = 1 }` with mock `config: { b: 2 }` resolved `.config.b` to `null`, because the
  fallback merge only overlaid top-level outputs.
- `--use-mocks=1`, `=0`, `=t`, and `ATMOS_USE_MOCKS=0` worked in 1.229.0 (the flag was a plain
  boolean) and now failed with an invalid-value error.
- `atmos terraform plan app --use-mocks always` passed `always` to Terraform ("Too many command line
  arguments"), and `atmos describe component app --use-mocks always` printed a usage dump.
- An exported `ATMOS_USE_MOCKS=true` broke `atmos terraform apply` with an error that named only the
  flag.
- A bare `--use-mocks` on a machine without Terraform, credentials, or backend access worked in
  1.229.0 and now failed with no hint that `--use-mocks=always` restores mocks-only resolution.
- An invalid `components.terraform.mocks.mode` in atmos.yaml was accepted (and shown by
  `describe config`) until a `--use-mocks` lookup ran, while an invalid env var failed every command.
- The progress message read `Fetching . output from vpc in dev` for the whole-map fetch.
- The configuration reference claimed `!terraform.output` returns the not-provisioned error for a
  never-applied component; it returns `null`.

Intentionally not changed:

- Terraform does not record `null`-valued outputs in state, so a real `null` output is
  indistinguishable from a missing one and resolves to the mock. This is now documented as a
  limitation instead.
- Mock-resolved values are still reported only at Debug level (one line per resolved path), to keep
  `describe component` and `plan` output unchanged at the default level.
- `.flag // "x"` returning the default for a real `false` is YQ's `//` semantics and behaves the same
  without mocks.
- The field test also saw Atmos's own S3 state reader use virtual-hosted addressing despite
  `use_path_style: true`; that code is outside this PR.

## Changes

- `internal/exec/terraform_mocks.go`: fallback deep-merges mocks under the real outputs
  (`mergeRealOverMocks`; real values win, maps merge recursively, lists and scalars replace). The
  Debug log now covers nested paths resolved from mocks (`resolvedFromMocks`).
  `withFallbackModeHint` adds a `--use-mocks=always` hint to non-recoverable fallback errors in
  `yaml_func_terraform_state.go` and `yaml_func_terraform_output.go`.
- `pkg/config/use_mocks.go`: `ParseUseMocksFlag` accepts the `strconv.ParseBool` forms again;
  `ParseUseMocksValue` names the value's source (`--use-mocks (or ATMOS_USE_MOCKS)` for
  `atmos terraform`); `CheckUseMocksSeparatedMode` rejects a mode word passed as a separate argument,
  with a hint.
- `cmd/terraform/utils.go` and `cmd/describe_component.go`: wire the separated-mode check (before
  hooks and in RunE for `plan`; in RunE for `describe component`, whose arity check now defers to
  it). The "supported only by plan" error names `ATMOS_USE_MOCKS` and hints to unset it.
- `pkg/config/utils.go`: `normalizeConfiguredMocksMode` lower-cases and validates the atmos.yaml
  `mocks.mode` at config load; both invalid-mode errors name their source.
- `pkg/terraform/output`: `fetchingOutputMessage` shows "Fetching all outputs" for the `.` identity.
- Tests: deep-merge, nested, list-replacement, hint, boolean-form, separated-mode, source-naming,
  and load-time validation cases. Loose `require.Error` plus text assertions in
  `terraform_mocks_test.go` now use `ErrorIs` against the sentinels. New CLI cases in
  `tests/test-cases/terraform-component-mocks.yaml` run `describe component --use-mocks` end to end
  (no CLI case existed before).
- Fixture `tests/fixtures/scenarios/terraform-component-mocks-fallback/` (built for the field test)
  now backs two real-state tests. The `dev` stack (local backend) is used by
  `internal/exec/terraform_mocks_fallback_test.go`, which applies the producer with the installed
  `tofu`/`terraform` and checks fallback, `always`, and mocks-off lookups before and after the apply.
  The `emu` stack (S3 backend, toolchain-installed OpenTofu) is used by
  `tests/terraform_mocks_fallback_floci_test.go`, which runs the CLI against the Floci emulator and
  covers a missing state object, applied state, `always`, and an unreachable backend. The Floci test
  is added to the `floci-go` job's `-run` allowlist in `.github/workflows/test.yml`.
- Docs: mocks, configuration, `terraform plan`, `describe component`, environment variables, YAML
  function pages, agent skill, and PRD updated for the behavior above, the `null` limitation, the
  `=` requirement, and the `ATMOS_USE_MOCKS` scope.

## Validation

- Rebuilt `./build/atmos` and reran the field-test repros:
  - `.config.b` now resolves to `2`.
  - `--use-mocks=1`, `=0`, and `ATMOS_USE_MOCKS=0` work again.
  - `--use-mocks always` reports the missing `=` with a hint for both `plan` and `describe component`.
  - `ATMOS_USE_MOCKS=true atmos terraform apply` names the env var and hints to unset it.
  - Missing Terraform and an unreachable S3 emulator both fail with the `--use-mocks=always` hint.
  - An invalid atmos.yaml mode now fails `describe config`, and `Always` is accepted.
- `go test` on `./pkg/config/...`, `./pkg/terraform/output/...`, `./cmd/terraform/...`, `./cmd/`, and
  `./internal/exec/`: pass.
- `go test ./internal/exec -run TestTerraformComponentMocksFallbackRealState`: pass, applying the producer with
  the local Terraform binary.
- `ATMOS_TEST_FLOCI=true go test ./tests -run TestTerraformComponentMocksFallbackFlociS3`: pass, with
  Floci auto-started through testcontainers.
- `cd website && npm run build`: pass.
- `go test ./tests -run 'TestCLICommands/(describe_component_--use-mocks|invalid_ATMOS_COMPONENTS_TERRAFORM_MOCKS_MODE)'`:
  5/5 pass.
- `./custom-gcl run --new-from-rev=origin/main` on the changed packages: 0 issues.

## Follow-ups

None.
