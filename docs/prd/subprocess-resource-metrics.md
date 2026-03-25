# Subprocess Resource Metrics

## Overview

Atmos captures resource usage metrics (CPU, memory, wall time, page faults, I/O, context switches) from every time-consuming subprocess execution and reports them locally via `ui.Info`. For `plan` and `apply` commands, metrics are uploaded to Atmos Pro alongside the existing `command` + `exit_code` status data (see PR #2216).

This is a pure Go implementation using `ProcessState` and `syscall.Rusage` — equivalent to `/usr/bin/time -v` with zero external dependencies.

## Linear

TBD

## Problem

Operators have no visibility into which components consume the most CPU, memory, and time. Atmos Pro wants to surface the most resource-intensive components to help teams identify optimization targets, but has no data. Locally, users get no feedback on execution cost after running terraform commands.

## Solution

### New Package: `pkg/metrics/process`

A reusable package that collects process-level metrics from `os/exec.Cmd` after execution.

#### Core Type

```go
type ProcessMetrics struct {
    // Universal (all platforms)
    WallTime      time.Duration // elapsed real time
    UserCPUTime   time.Duration // ProcessState.UserTime() — includes children
    SystemCPUTime time.Duration // ProcessState.SystemTime() — includes children
    ExitCode      int           // ProcessState.ExitCode()

    // Unix only (from syscall.Rusage via ProcessState.SysUsage())
    MaxRSSBytes      int64 // peak resident set size across process tree
    MinorPageFaults  int64 // page reclaims (soft faults)
    MajorPageFaults  int64 // page faults (hard faults, required I/O)
    InBlockOps       int64 // filesystem input operations
    OutBlockOps      int64 // filesystem output operations
    VolCtxSwitches   int64 // voluntary context switches (process yielded)
    InvolCtxSwitches int64 // involuntary context switches (preempted)
}
```

#### Collection Pattern

```go
func Collect(cmd *exec.Cmd) (*ProcessMetrics, error) {
    start := time.Now()
    err := cmd.Run()
    elapsed := time.Since(start)

    m := &ProcessMetrics{
        WallTime: elapsed,
        ExitCode: -1,
    }

    if ps := cmd.ProcessState; ps != nil {
        m.ExitCode = ps.ExitCode()
        m.UserCPUTime = ps.UserTime()
        m.SystemCPUTime = ps.SystemTime()

        if ru, ok := ps.SysUsage().(*syscall.Rusage); ok {
            switch runtime.GOOS {
            case "linux":
                m.MaxRSSBytes = ru.Maxrss * 1024 // Linux: KiB
            case "darwin":
                m.MaxRSSBytes = ru.Maxrss // macOS: bytes
            default:
                m.MaxRSSBytes = ru.Maxrss
            }
            m.MinorPageFaults = ru.Minflt
            m.MajorPageFaults = ru.Majflt
            m.InBlockOps = ru.Inblock
            m.OutBlockOps = ru.Oublock
            m.VolCtxSwitches = ru.Nvcsw
            m.InvolCtxSwitches = ru.Nivcsw
        }
    }

    return m, err
}
```

#### Key Design Decisions

- **No polling, no goroutines.** All data comes from `ProcessState` after `cmd.Run()` completes.
- **CPU time includes children.** Go documents `UserTime()` and `SystemTime()` as covering the exited process and its waited-for children. For terraform, this includes provider plugins and any other subprocesses terraform spawns.
- **Peak RSS includes children.** `Rusage.Maxrss` reports the peak resident set size of the process or any of its descendants (whichever was highest).
- **No external dependencies.** Pure Go stdlib: `os/exec`, `syscall`, `runtime`, `time`.
- **Platform normalization.** Linux reports `ru_maxrss` in KiB, macOS reports it in bytes. The package normalizes to bytes.
- **Graceful degradation.** On Windows, `SysUsage()` returns nil — the Rusage fields remain zero. Wall time and CPU time work on all platforms.

### Architecture: Automatic Integration via `ExecuteShellCommand`

All component types in Atmos funnel through a single execution function:

```
Terraform ─┐
Helmfile  ─┤
Packer    ─┼──▶ ExecuteShellCommand()  ──▶  cmd.Run()  ──▶  ProcessState  ──▶  Metrics
Workflows ─┤         shell_utils.go
AWS EKS   ─┘
```

**Call sites that go through `ExecuteShellCommand`:**

| Component | File | Call sites |
|---|---|---|
| Terraform | `internal/exec/terraform_execute_helpers_exec.go` | main command (plan/apply/destroy/etc.) via `executeMainTerraformCommand()` |
| Terraform | `internal/exec/terraform.go` | version, init, workspace select/new |
| Helmfile | `internal/exec/helmfile.go` | version, kubeconfig update, main command (sync/diff/apply/etc.) |
| Packer | `internal/exec/packer.go` | version, main command (build/validate/etc.) |
| Workflows | `internal/exec/workflow_adapters.go` | nested atmos commands |
| AWS EKS | `internal/exec/aws_eks_update_kubeconfig.go` | kubeconfig update |

**What bypasses it (no metrics needed):**

- Interactive shells (`terraform shell`, `auth shell`) — use `os.StartProcess()` directly
- OpenTofu detection — one-off version check
- Git/downloader/Azure CLI utilities in `pkg/`

### Integration Point 1: Collect Metrics in `ExecuteShellCommand`

**File:** `internal/exec/shell_utils.go`

`ExecuteShellCommand` calls `cmd.Run()` at line 176 and returns `error`. The change replaces `cmd.Run()` with `process.Collect(cmd)` and exposes metrics to callers via a functional option callback.

**Approach:** Add a new `WithMetricsCallback` option to the existing `ShellCommandOption` pattern:

```go
// New option — fits existing pattern (WithStdoutCapture, WithStderrCapture, etc.)
func WithMetricsCallback(fn func(*process.ProcessMetrics)) ShellCommandOption {
    return func(c *shellCommandConfig) {
        c.metricsCallback = fn
    }
}
```

Inside `ExecuteShellCommand`:

```go
// Replace cmd.Run() with metrics-aware execution
metrics, err := process.Collect(cmd)

// Invoke callback if provided — caller decides what to do with metrics
if cfg.metricsCallback != nil && metrics != nil {
    cfg.metricsCallback(metrics)
}
```

**This is zero-change for existing callers.** Callers that don't pass `WithMetricsCallback` see no difference. Callers that want metrics (terraform plan/apply, helmfile sync, etc.) pass the callback and get them automatically.

### Integration Point 2: Local Display

**Files:** `internal/exec/terraform_execute_helpers_exec.go`, `internal/exec/helmfile.go`, `internal/exec/packer.go`

Each orchestrator passes `WithMetricsCallback` for its main command execution. The callback formats and displays via `ui.Info`:

```
ℹ Completed in 45.2s | CPU: 12.3s user, 4.1s sys | Peak memory: 512 MB
```

**Which calls get the callback (show metrics):**

| Component | Call site | Gets callback? |
|---|---|---|
| Terraform main command (plan/apply/destroy/init) | `terraform_execute_helpers_exec.go:291` via `executeMainTerraformCommand()` | Yes |
| Terraform workspace select/new | `terraform_execute_helpers_exec.go` via `runWorkspaceSetup()` | No |
| Terraform version | `terraform.go` | No |
| Helmfile main command (sync/diff/apply) | `helmfile.go` | Yes |
| Helmfile version | `helmfile.go` | No |
| Packer main command (build/validate) | `packer.go` | Yes |
| Packer version | `packer.go` | No |

**Settings gate:**

```yaml
settings:
  metrics:
    enabled: true  # default
```

When `settings.metrics.enabled` is `false`, the callback still fires (for Pro upload) but skips the `ui.Info` display. No new CLI flags.

### Integration Point 3: Atmos Pro Upload

**Files:** `pkg/pro/dtos/instances.go`, `internal/exec/pro.go`, `pkg/pro/api_client_instance_status.go`

#### DTO Changes

Extend `InstanceStatusUploadRequest` with metrics fields. Note that `AtmosVersion`, `AtmosOS`, and `AtmosArch` already exist from PR #2216:

```go
type InstanceStatusUploadRequest struct {
    // Existing fields
    AtmosProRunID string `json:"atmos_pro_run_id,omitempty"`
    AtmosVersion  string `json:"atmos_version,omitempty"`
    AtmosOS       string `json:"atmos_os,omitempty"`
    AtmosArch     string `json:"atmos_arch,omitempty"`
    GitSHA        string `json:"git_sha,omitempty"`
    RepoURL       string `json:"repo_url,omitempty"`
    RepoName      string `json:"repo_name"`
    RepoOwner     string `json:"repo_owner"`
    RepoHost      string `json:"repo_host,omitempty"`
    Component     string `json:"component"`
    Stack         string `json:"stack"`
    Command       string `json:"command"`
    ExitCode      int    `json:"exit_code"`
    LastRun        string `json:"last_run,omitempty"`

    // New: timing
    WallTimeMs       *int64 `json:"wall_time_ms,omitempty"`
    UserCPUTimeMs    *int64 `json:"user_cpu_time_ms,omitempty"`
    SysCPUTimeMs     *int64 `json:"sys_cpu_time_ms,omitempty"`

    // New: memory
    PeakRSSBytes     *int64 `json:"peak_rss_bytes,omitempty"`

    // New: page faults
    MinorPageFaults  *int64 `json:"minor_page_faults,omitempty"`
    MajorPageFaults  *int64 `json:"major_page_faults,omitempty"`

    // New: I/O
    InBlockOps       *int64 `json:"in_block_ops,omitempty"`
    OutBlockOps      *int64 `json:"out_block_ops,omitempty"`

    // New: context switches
    VolCtxSwitches   *int64 `json:"vol_ctx_switches,omitempty"`
    InvolCtxSwitches *int64 `json:"invol_ctx_switches,omitempty"`
}
```

#### API Client: Switch to Full DTO Marshaling

The current `UploadInstanceStatus()` in `pkg/pro/api_client_instance_status.go` manually builds a `map[string]interface{}` with only `command`, `exit_code`, and `last_run`. This means `AtmosVersion`, `AtmosOS`, `AtmosArch` (added in PR #2216) are populated in the DTO but never sent to the server.

**Fix:** Replace the manual map construction with direct DTO marshaling:

```go
// Before (current — lossy):
payload := map[string]interface{}{
    "command":   dto.Command,
    "exit_code": dto.ExitCode,
}

// After (proposed — complete):
data, err := json.Marshal(dto)
```

This sends all DTO fields automatically. The `omitempty` tags on optional fields ensure Windows clients (where Rusage metrics are zero-valued) and older CLIs (without metrics) produce clean payloads. The `last_run` field moves into the DTO itself (computed in `uploadStatus()`) so it's included in the marshaled output.

**Benefits:**
- Fixes the existing gap where version/OS/arch are collected but never sent.
- New metrics fields are sent automatically — no manual map maintenance.
- Adding future fields to the DTO automatically includes them in the payload.

#### Upload Call Chain

The metrics flow through the existing upload path:

```
executeMainTerraformCommand()          (terraform_execute_helpers_exec.go:278)
  → ExecuteShellCommand() with WithMetricsCallback
  → uploadCommandStatus()              (terraform_execute_helpers_exec.go:320)
    → uploadStatus(info, exitCode, metrics, client, gitRepo)  (pro.go:218)
      → client.UploadInstanceStatus(dto)
```

`uploadStatus()` gains a `*process.ProcessMetrics` parameter. When non-nil, it populates the DTO's timing/memory/IO fields before calling the API client.

**Upload scope:** `plan` and `apply` only (same as PR #2216 status upload).

### API Contract Extension

The existing PATCH to `/api/v1/repos/{owner}/{repo}/instances?stack={stack}&component={component}` gains optional fields. All new fields use `omitempty` for backward compatibility:

```json
{
  "command": "plan",
  "exit_code": 0,
  "last_run": "2026-03-17T12:00:00Z",
  "atmos_version": "1.234.0",
  "atmos_os": "linux",
  "atmos_arch": "amd64",
  "wall_time_ms": 45200,
  "user_cpu_time_ms": 12300,
  "sys_cpu_time_ms": 4100,
  "peak_rss_bytes": 536870912,
  "minor_page_faults": 42000,
  "major_page_faults": 12,
  "in_block_ops": 1500,
  "out_block_ops": 800,
  "vol_ctx_switches": 3200,
  "invol_ctx_switches": 150
}
```

Atmos Pro ignores unknown fields, so this is backward-compatible. The server-side counterpart should be documented in a separate Atmos Pro PRD.

### Atmos Pro Use Cases

With these metrics, Atmos Pro can:

- **Surface the most resource-intensive components** — rank by wall time, CPU time, or memory
- **Detect memory-heavy providers** — identify components with unusually high peak RSS
- **Flag I/O-bound operations** — high block ops or major page faults suggest disk/network bottlenecks
- **Track performance regressions** — compare metrics across runs to detect degradation
- **Identify contention** — high involuntary context switches suggest CPU contention in CI runners

## Cross-Platform Behavior

| Metric | Linux | macOS | Windows |
|---|---|---|---|
| Wall time | yes | yes | yes |
| User CPU time | yes | yes | yes |
| System CPU time | yes | yes | yes |
| Exit code | yes | yes | yes |
| Peak RSS | yes (KiB → bytes) | yes (native bytes) | zero |
| Page faults | yes | yes | zero |
| Block I/O ops | yes | yes | zero |
| Context switches | yes | yes | zero |

## Files Modified

| File | Change |
|---|---|
| `pkg/metrics/process/` (new) | New package with `ProcessMetrics` type and `Collect()` function |
| `internal/exec/shell_utils.go` | Replace `cmd.Run()` with `process.Collect(cmd)`, add `WithMetricsCallback` option |
| `internal/exec/terraform_execute_helpers_exec.go` | Pass `WithMetricsCallback` to `ExecuteShellCommand` in `executeMainTerraformCommand()`, pass metrics to `uploadCommandStatus()` |
| `internal/exec/helmfile.go` | Pass `WithMetricsCallback` for main command, display via `ui.Info` |
| `internal/exec/packer.go` | Pass `WithMetricsCallback` for main command, display via `ui.Info` |
| `internal/exec/pro.go` | `uploadStatus()` accepts `*ProcessMetrics`, populates DTO metrics fields, computes `LastRun` in DTO |
| `pkg/pro/dtos/instances.go` | Add metrics fields + `LastRun` to DTO, add `omitempty` tags to optional existing fields |
| `pkg/pro/api_client_instance_status.go` | Replace manual `map[string]interface{}` with `json.Marshal(dto)` — fixes version/OS/arch gap and sends metrics |
| `pkg/schema/` | Add `settings.metrics.enabled` to config schema |

## Predecessors

- `docs/prd/instance-status-raw-upload.md` (PR #2216) — established raw `command` + `exit_code` upload pattern.

### Existing Gap: Version/OS/Arch Fields Not Sent

PR #2216 added `AtmosVersion`, `AtmosOS`, and `AtmosArch` fields to the `InstanceStatusUploadRequest` DTO and populates them in `uploadStatus()` (`internal/exec/pro.go:242-244`). However, the API client (`pkg/pro/api_client_instance_status.go`) manually builds a `map[string]interface{}` payload containing only `command`, `exit_code`, and `last_run` — the version/OS/arch fields never reach the server. This PRD's implementation fixes this gap as a side effect (see Integration Point 3).
