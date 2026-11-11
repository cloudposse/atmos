# Fix: telemetry no longer warns "Could not load config" on a missing profile

**Date:** 2026-11-11

## Summary

Running a trivial command such as `atmos version` with `ATMOS_PROFILE` pointing at a profile that
does not exist in the current directory printed a confusing, user-facing warning:

```
👽 Atmos 1.234.1 on darwin/arm64

 WARN  Could not load config error="profile not found"
```

The warning came from the telemetry subsystem, which loads configuration independently and
best-effort. It now degrades quietly: the config-load failure is logged at `DEBUG` instead of
`WARN`, so trivial commands no longer surface telemetry's internal config problems. Nothing a user
can act on is lost - a command that actually needs configuration still surfaces its own enriched
error (with profile hints and `atmos profile list` guidance).

## Context

Command telemetry loads configuration in two places in `pkg/telemetry/utils.go`:

- `getTelemetryFromConfig()` - reads `settings.telemetry.*`.
- `atmosProWorkspaceID()` - reads `settings.pro.workspace_id`.

Both call `cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, false)`. `InitCliConfig` resolves the
active profile from `--profile`, then `ATMOS_PROFILE`, then `profiles.default` in `atmos.yaml`
(`pkg/config/load.go`). When a profile is requested but cannot be found in any configured location,
it returns `errUtils.ErrProfileNotFound` ("profile not found") and aborts the whole load.

A user who exports `ATMOS_PROFILE` in their shell and then runs Atmos from an unrelated directory
(no matching `profiles/` tree) hits exactly this. For a real command the hard error is correct -
the user asked for a profile that does not exist. But telemetry is best-effort and already handles
the failure gracefully (it returns `nil`/`""` and disables telemetry). The only defect was the log
level: it logged at `WARN`, turning every telemetry-touching command - including `atmos version`,
whose own execution never needs config - into noise. The message was also misleading: the base
configuration loaded fine; only the requested profile was missing.

The project already asserts this warning must not surface for config-free help
(`cmd/root_doctor_help_test.go::TestDoctorHelpDoesNotLoadConfiguration`), but the command-telemetry
capture path - which runs for every command via `RootCmd.ExecuteC()` - was not covered, and
`atmos version` is not a config-free command.

## Reproduction

```bash
cd "$(mktemp -d)"            # a directory with no atmos.yaml and no profiles/
ATMOS_PROFILE=nonexistent atmos version
# Before: prints "WARN Could not load config error=\"profile not found\""
# After:  no warning; the version still prints normally
```

## Changes

- `pkg/telemetry/utils.go`: both config-load error branches
  (`getTelemetryFromConfig` and `atmosProWorkspaceID`) now log at `log.Debug` instead of
  `log.Warn`, with a comment explaining the best-effort, degrade-quietly rationale. The diagnostic
  is preserved at `DEBUG` for troubleshooting.
- `pkg/telemetry/config_load_warning_test.go` (new): reproduces the scenario (missing
  `ATMOS_PROFILE` resolved from an empty working directory) and asserts that:
  - neither config-load site surfaces "Could not load config" at `WARN`;
  - both still log it at `DEBUG` (diagnostic preserved);
  - telemetry degrades to `nil` / empty workspace ID;
  - the public `CaptureCmd` entry point runs end to end (both variadic forms) without a
    user-facing warning when telemetry is disabled.

## Why not suppress the warning only for `version`

Telemetry runs for every command and its config load is incidental to all of them, so gating the
log level on a specific command would leave the same noise on `atmos describe`, `atmos list`, and
any other command that touches telemetry. Lowering the level at the single best-effort source fixes
every caller at once, and the real error path (the command's own `InitCliConfig`) is unchanged.

## Verification

- `go test ./pkg/telemetry/ -count=1` - passes; package coverage 89.7% -> 92.1%
  (`getTelemetryFromConfig`, `atmosProWorkspaceID`, `captureCmdString`, and the public `CaptureCmd`
  are all 100%).
- `go test ./cmd/ -run 'TestDoctorHelpDoesNotLoadConfiguration|TestDoctorIsAnnotatedConfigFree'` -
  passes (existing config-free contract unaffected).
- Manual: `ATMOS_PROFILE=nonexistent atmos version` from an empty directory prints no warning.
