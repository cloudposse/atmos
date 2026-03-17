# Raw Instance Status Upload to Atmos Pro

## Overview

Previously, the Atmos CLI interpreted terraform exit codes locally — mapping them to status strings like `"in_sync"` or `"drifted"` — before sending the result to Atmos Pro. This meant Atmos Pro never saw the raw data, status interpretation was coupled to CLI releases, and error exit codes were silently dropped.

This change makes the CLI a dumb pipe: it sends the raw command (`plan` or `apply`) and exit code to Atmos Pro, which interprets them server-side. This lets Atmos Pro evolve status logic without requiring CLI updates. The upload is also extended from plan-only to both plan and apply, eliminating the "Unknown" status problem on the dashboard.

## What Changed in the CLI

1. **DTO** (`pkg/pro/dtos/instances.go`): Replaced `HasDrift bool` with `Command string` + `ExitCode int`.
2. **API client** (`pkg/pro/api_client_instance_status.go`): Sends `{ command, exit_code }` instead of `{ status }`.
3. **Upload logic** (`internal/exec/pro.go`): Removed exit code filtering (previously skipped non-0/non-2). Removed `HasDrift` interpretation. Passes raw `SubCommand` and `exitCode`.
4. **Upload scope** (`internal/exec/pro.go`): `shouldUploadStatus()` now accepts `apply` in addition to `plan`.
5. **Apply wiring** (`internal/exec/terraform.go`): Apply uploads automatically when `settings.pro.enabled` is true — no `--upload-status` flag required.

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
