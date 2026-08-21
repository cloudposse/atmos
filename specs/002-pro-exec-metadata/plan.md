# Implementation Plan: Atmos Pro Command-Execution Metadata Upload

**Branch**: `1199-pro-exec-metadata` | **Date**: 2026-08-21 | **Spec**: [spec.md](./spec.md)

**Input**: Feature specification from `specs/002-pro-exec-metadata/spec.md`

**Note**: This is an eleventh re-plan. It narrows the tenth re-plan's already-narrow
scope further, based on two 2026-08-21 clarifications made directly against FR-006e:

1. **`-detailed-exitcode` is `plan`-only**, not also added to `apply`/`deploy`'s
   internal `apply` invocation — `-detailed-exitcode` support on `apply` only landed
   in Terraform 1.5+/OpenTofu, and Atmos supports arbitrary pinned older terraform/tofu
   binaries via `atmos.yaml` that could hard-fail on the unrecognized flag on `apply`.
   `apply`/`deploy` keep plain 0 (success)/1 (error) semantics; their `exit_code` is
   still captured pre-CI-remap for consistency, but its value space stays 0/1, never 2.
2. **Atmos's own process exit code for `plan` must be provably unaffected** by adding
   `-detailed-exitcode`. Investigating this (Decision 35 below) found the two gates
   involved are **not the same mechanism and do not coincide today**:
   - FR-001's CI-detection gate (`pkg/proexec/gate.go`'s `gateOpen`) uses
     `telemetry.IsCI()` — broad, automatic environment-variable-based CI-provider
     detection (`GITHUB_ACTIONS`, `GITLAB_CI`, `CI=true`, etc.), always-on with no
     opt-in required.
   - The CI-mode exit-code remap (`internal/exec/ci_exit_codes.go`'s `mapCIExitCode`)
     is gated by `atmosConfig.CI.Enabled`, which is bound **only** from an explicit
     `ci.enabled: true` in `atmos.yaml` (mapstructure-bound config, not a flag/env
     binding) — a deliberate master switch also gating CI annotations, SARIF result
     uploads, container run summaries, and hook CI-mode behavior
     (`pkg/ci/mode.go`'s `Enabled`/`AnnotationsEnabled`/`ResultsEnabled`,
     `pkg/runner/step/container_summary.go`, `pkg/hooks/hooks.go`). The codebase
     already treats "detected in CI" and "`ci.enabled` set" as a KNOWN, intentional
     divergence — `cmd/ci/status.go` explicitly errors when `cipkg.IsCI()` is true but
     `atmosConfig.CI.Enabled` is false, instructing the user to opt in.

   Because most real CI environments satisfying FR-001 do **not** also have
   `ci.enabled: true` set in `atmos.yaml`, adding `-detailed-exitcode` to `plan`
   unconditionally under FR-001's gate would, in the common case, leave `mapCIExitCode`
   inactive and let terraform's real exit 2 propagate straight through to Atmos's own
   process exit code — a real, not hypothetical, behavior change from today's always-0.
   **Flipping the global `ci.enabled` master switch to close this gap is rejected** —
   it would silently turn on annotations/SARIF/summaries/hooks CI-mode as a side effect
   of an unrelated bug fix. The correct fix is narrower: a **local** exit-code
   neutralization specific to the `-detailed-exitcode` value added for exec-metadata
   purposes, applied only at the call site that added the flag, independent of the
   global `ci.enabled` switch.

## Summary

Two deltas, both confined to `cmd/terraform/utils.go` / `internal/exec/terraform*.go`,
fixing two confirmed production bugs in the already-shipped `TerraformExecData` (FR-006)
exec-metadata payload:

1. **`exit_code` reliability (FR-006e)**: `buildPlanSubcommandArgs`
   (`internal/exec/terraform_execute_helpers_args.go`) currently only adds
   `-detailed-exitcode` when the legacy `--upload-status` flag is set. It must instead
   add `-detailed-exitcode` to `plan` whenever exec-metadata capture is active
   (`pkg/proexec.IsSyncCommand`), independent of `--upload-status`, and NOT add it to
   `apply`/`deploy`'s internal `apply` invocation at all. `captureExecMetadataSync`
   (`internal/exec/terraform.go`) must capture the terraform/tofu subprocess's real,
   pre-remap exit code for `TerraformExecData.exit_code`, before
   `executeMainTerraformCommand`'s `mapCIExitCode` call
   (`internal/exec/terraform_execute_helpers_exec.go`) remaps it for Atmos's own
   returned error/exit status. Additionally, whenever `-detailed-exitcode` was added to
   a `plan` invocation specifically because exec-metadata capture required it (not
   because the user's own `ci.enabled` config already covers it), that same code path
   must apply its own local exit-2-to-0 neutralization to what Atmos itself returns —
   independent of the global `atmosConfig.CI.Enabled` switch, since that switch must
   not be force-enabled as a side effect of this fix (see Decision 35).
2. **`resource_counts`/`outputs`/`changes` reliability (FR-006f)**:
   `terraformCaptureShellOpts` (`cmd/terraform/utils.go`) currently shares one
   stdout/stderr buffer across `terraform init`, `terraform workspace select`, and the
   main `plan`/`apply` invocation (`executeCommandPipeline`,
   `internal/exec/terraform_execute_helpers_exec.go`). The buffer fed to
   `terraformExecMetadataParserFunc` (`cmd/terraform/utils.go`) must instead be
   reset/isolated immediately before the final `plan`/`apply` subprocess invocation, so
   the existing regex-based `citerraform.ParseOutput`/`ParsePlanOutput`/`ParseApplyOutput`
   parser (`pkg/ci/plugins/terraform` — unchanged, same mechanism Native CI already
   uses) only ever sees that invocation's own output.

No `-json`-stream parser rewrite, no shared-parser change touching Native CI's own
code path — that direction was proposed and retracted earlier in this spec's
Clarifications (FR-006g/FR-006h, marked Retracted).

## Technical Context

**Language/Version**: Go 1.26 (per `go.mod`; CI pins via `go-version-file: go.mod`)

**Primary Dependencies**:
- `internal/exec/terraform_execute_helpers_args.go` — `buildPlanSubcommandArgs`
  (currently gates `-detailed-exitcode` behind `uploadStatusFlag`) needs an additional
  trigger: exec-metadata capture being active for this invocation
  (`pkg/proexec.IsSyncCommand(subCommand)` for `"plan"`).
- `internal/exec/terraform_execute_helpers_exec.go` — `executeMainTerraformCommand`
  (owns `mapCIExitCode` application) needs to (a) surface the pre-remap exit code
  separately for `captureExecMetadataSync` to read, and (b) apply a local
  exit-2-neutralization for Atmos's own returned status when `-detailed-exitcode` was
  added specifically for exec-metadata purposes and the global `ci.enabled` switch is
  not already covering it.
- `internal/exec/ci_exit_codes.go` — `mapCIExitCode`/`defaultCIExitCodes` — read, not
  modified; this is the existing global-`ci.enabled`-gated remap, left as-is per
  Decision 35's finding that widening its gate is the wrong fix.
- `internal/exec/terraform.go` — `captureExecMetadataSync`/`ExecuteTerraform` — exit
  code plumbing into `TerraformExecData.exit_code`.
- `cmd/terraform/utils.go` — `buildTerraformExecData`, `parseTerraformOutputMirror`,
  `terraformCaptureShellOpts`, `terraformExecMetadataParserFunc` — buffer scoping fix
  (FR-006f) and `exitCode` parameter plumbing (FR-006e).
- `pkg/ci/plugins/terraform` (`ParsePlanOutput`/`ParseApplyOutput`/`ParseOutput`) —
  consumed, unchanged.
- `pkg/proexec` (`IsSyncCommand`, `gateOpen`) — consumed to determine when
  exec-metadata capture (and therefore this fix's behavior) is active.

**Storage**: N/A (no new persistence; existing Atmos Pro upload endpoints unchanged)

**Testing**: `atmos test` (short mode), `atmos test --coverage`; existing suites in
`internal/exec/terraform_execute_helpers_args_test.go`,
`internal/exec/terraform_exitcode_test.go`, `cmd/terraform/utils_test.go` (or
equivalent) to extend per this repo's mandatory bug-fixing workflow (write a failing
test reproducing each bug first, then fix).

**Target Platform**: Linux/macOS/Windows (cross-platform CLI); no platform-specific
behavior introduced by either fix.

**Project Type**: CLI (single Go module, `cmd/`/`internal/`/`pkg/` layout)

**Performance Goals**: N/A — no measurable perf target; both fixes are correctness
fixes with no new I/O beyond what the existing `plan`/`apply` invocation already does.

**Constraints**: Must not change Atmos's own process exit code for `plan`/`apply`/
`deploy` in any CI environment satisfying FR-001 (the exit-code-neutralization
guarantee, Decision 35). Must not force-enable the global `ci.enabled` config switch as
a side effect. Must not add `-detailed-exitcode` to `apply`/`deploy` (version-support
risk on older pinned terraform/tofu binaries).

**Scale/Scope**: Two functions/call sites for FR-006e, one buffer-lifecycle change for
FR-006f; no new files expected, no schema/wire-shape changes (payload fields defined by
FR-006 are unchanged, only how they get populated is fixed).

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

- **Interface-Driven Design / DI**: N/A — no new external dependency being introduced;
  existing `ShellCommandOption`/functional-options pattern for
  `terraformCaptureShellOpts` is reused as-is for the buffer-scoping fix.
- **Error Handling (static errors, `errors.Is`)**: No new error paths introduced by
  either fix; existing exit-code plumbing (`errUtils.GetExitCode`) is reused.
- **Performance Tracking**: Any new/modified public function gets
  `defer perf.Track(atmosConfig, "pkg.FuncName")()` per existing convention (both
  touched files already follow this).
- **Test Isolation / Test Quality**: New tests reproduce each bug first (mandatory bug-
  fixing workflow), then verify the fix; existing `pkg/ci/plugins/terraform` tests are
  unaffected since that package's code is not modified by either fix.
- **Package Organization**: No new package needed; both fixes live in existing
  `cmd/terraform` and `internal/exec` files.
- **No violations requiring Complexity Tracking.**

## Project Structure

### Documentation (this feature)

```text
specs/002-pro-exec-metadata/
├── plan.md              # This file (eleventh re-plan)
├── research.md          # Decisions 1-35 (33/34 retracted, see Clarifications)
├── data-model.md        # TerraformExecData shape (fields unchanged by this re-plan)
├── quickstart.md        # Unaffected by this re-plan (wire shape unchanged)
├── contracts/           # Unaffected by this re-plan (wire shape unchanged)
└── tasks.md             # Regenerated to match this re-plan's two-fix scope
```

### Source Code (repository root)

```text
cmd/terraform/
├── utils.go              # buildTerraformExecData, parseTerraformOutputMirror,
│                          # terraformCaptureShellOpts, terraformExecMetadataParserFunc
└── plan.go                # plan subcommand wiring (ciMode local var, unrelated to
                            # atmosConfig.CI.Enabled — see Decision 35)

internal/exec/
├── terraform_execute_helpers_args.go   # buildPlanSubcommandArgs (-detailed-exitcode)
├── terraform_execute_helpers_exec.go   # executeMainTerraformCommand, mapCIExitCode
│                                        # call site, executeCommandPipeline (buffer
│                                        # threading through init/workspace/main)
├── ci_exit_codes.go                    # mapCIExitCode, defaultCIExitCodes (read-only)
└── terraform.go                        # captureExecMetadataSync, ExecuteTerraform

pkg/proexec/
└── gate.go               # gateOpen (FR-001 CI-detection gate, telemetry.IsCI())

pkg/ci/
└── mode.go                # Enabled/AnnotationsEnabled/ResultsEnabled (all keyed off
                            # atmosConfig.CI.Enabled — the switch this fix must NOT
                            # force-enable)
```

**Structure Decision**: No new packages or files. Both fixes are confined to existing
files in `cmd/terraform/` and `internal/exec/`, following this repo's existing
convention of small, focused changes within already-established file boundaries rather
than new abstractions.

## Complexity Tracking

*No Constitution Check violations — this section intentionally left empty.*
