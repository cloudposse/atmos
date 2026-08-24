# Raw Instance Status Upload to Atmos Pro

## Overview

Previously, the Atmos CLI interpreted terraform exit codes locally — mapping them to status strings like `"in_sync"` or `"drifted"` — before sending the result to Atmos Pro. This meant Atmos Pro never saw the raw data, status interpretation was coupled to CLI releases, and error exit codes were silently dropped.

This change makes the CLI a dumb pipe: it sends the raw command (`plan` or `apply`) and exit code to Atmos Pro, which interprets them server-side. This lets Atmos Pro evolve status logic without requiring CLI updates. The upload is also extended from plan-only to both plan and apply, eliminating the "Unknown" status problem on the dashboard.

## What Changed in the CLI

1. **DTO** (`pkg/pro/dtos/instances.go`): Replaced `HasDrift bool` with `Command string` + `ExitCode int`.
2. **API client** (`pkg/pro/api_client_instance_status.go`): Sends `{ command, exit_code }` instead of `{ status }`.
3. **Upload logic** (`internal/exec/pro.go`): Removed exit code filtering (previously skipped non-0/non-2). Removed `HasDrift` interpretation. Passes raw `SubCommand` and `exitCode`.
4. **Upload scope** (`internal/exec/pro.go`): `shouldUploadStatus()` now accepts `apply` in addition to `plan`.
5. **Upload trigger**: Both plan and apply require the explicit `--upload-status` flag.
6. **Flag reading**: The `--upload-status` flag is read from Cobra/Viper via `info.UploadStatus`, with a fallback to parsing `AdditionalArgsAndFlags` for backward compatibility with legacy code paths.

## CI Exit Code Mapping

Atmos always preserves the real exit code from terraform subcommands. When `--upload-status` is used with `terraform plan`, Atmos adds `--detailed-exitcode`, which makes terraform return exit code 2 for "changes detected". In CI environments with `set -e`, this causes the workflow step to fail.

To handle this, Atmos supports a configurable exit code mapping under `components.terraform.ci`:

```yaml
ci:
  enabled: true          # global CI gate — must be true for mapping to apply

components:
  terraform:
    ci:
      exit_codes:
        0: true          # exit 0 → success (exit 0)
        1: false         # exit 1 → failure (preserve exit 1)
        2: true          # exit 2 → success (exit 0)
```

**Behavior:**
- Only active when the global `ci.enabled` is true.
- After all side-effects (status upload) complete, the mapping is applied.
- If an exit code maps to `true`, Atmos exits 0 (success for the CI runner).
- If an exit code maps to `false` or is unmapped, the original exit code is preserved.
- The upload always sends the *original* exit code to Atmos Pro, never the remapped one.

**Design rationale:** The exit code mapping and the status upload are independent concerns. Upload captures what happened (raw data for Atmos Pro). The CI mapping controls what the *caller* (CI runner) sees. This separation means you can use CI exit code mapping without upload, and upload without CI exit code mapping.

## API Contract

The CLI sends a PATCH to `/api/v1/repos/{owner}/{repo}/instances?stack={stack}&component={component}` with:

```json
{
  "command": "plan" | "apply",
  "exit_code": <integer>,
  "last_run": "<ISO 8601 datetime>"
}
```

Atmos Pro interprets exit codes server-side:

| Command | Exit Code | Instance Status |
|---------|-----------|-----------------|
| plan    | 0         | `in_sync`       |
| plan    | 2         | `drifted`       |
| plan    | other     | `error`         |
| apply   | 0         | `in_sync`       |
| apply   | != 0      | `error`         |

The legacy `{ status }` format is still accepted for backward compatibility.
