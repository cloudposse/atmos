# Raw Instance Status Upload to Atmos Pro

## Overview

The Atmos CLI sends raw execution results (command + exit code) to Atmos Pro after `terraform plan` and `terraform apply`. Atmos Pro interprets the data server-side. The CLI is a dumb pipe — it does not map exit codes to status strings.

This is the CLI-side counterpart to the full PRD in the Atmos Pro repo:
**`cloudposse-corp/apps` → `apps/atmos-pro/prd/instance-status-from-workflow-hooks.md`**

Refer to that document for the complete design rationale, server-side interpretation logic, webhook fallback strategy, considered alternatives, and rollout plan.

## Linear

[AP-163](https://linear.app/cloudposse/issue/AP-163/investigate-unknown-status-issue-reported-by-daniel)

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
