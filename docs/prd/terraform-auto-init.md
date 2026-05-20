# PRD: Terraform Auto-Init Detection

## Overview

This document describes intelligent `terraform init` detection for Atmos, which automatically runs `terraform init` only when necessary and retries failed commands when init is detected as required. This reduces unnecessary init runs (improving performance) while ensuring init is automatically triggered when needed (improving reliability).

**Related Issue:** [#620 - Intelligently don't init on every apply/shell](https://github.com/cloudposse/atmos/issues/620)

## Problem Statement

### Current State

Atmos currently runs `terraform init` before every terraform command (plan, apply, destroy, etc.) unless `--skip-init` is explicitly passed:

```go
// internal/exec/terraform.go:452-507
if info.SubCommand != "init" && !info.SkipInit && runInit {
    // Always runs init before the actual command
    err = ExecuteShellCommand(...)
}
```

### User Pain Points

From community feedback (Slack, GitHub #620):

> "As our implementations grow in components, build times increase significantly since each component requires state initialization, state read etc on each plan."

> "On a slow connection, even with cached providers, atmos's behavior of running terraform init every time atmos is called slows down the development loop considerably."

> "Terraform nor terragrunt have this behavior, the first just errors if you don't init, the latter uses a cache dir with hashes of module versions to predict when init is necessary or not."

### Challenges

1. **Unnecessary init runs** - Running init every time adds 2-10+ seconds per command, even when nothing has changed.
2. **No automatic recovery** - If a user runs with `--skip-init` and init was actually needed, the command fails without helpful recovery.
3. **Poor CI/CD experience** - In pipelines with multiple terraform commands (generate planfile → apply), init runs redundantly.
4. **Inconsistent with industry tools** - Terragrunt and Terramate both implement smart init detection.

### How Other Tools Solve This

**Terragrunt** (auto-init feature):
- Checks for `.terragrunt-init-required` marker file.
- Detects source/module changes.
- Limitation: "There might be cases where Terragrunt does not detect that init needs to be called."

**Terramate** (change detection):
- Uses Git-based change detection.
- Runs init only in stacks with changed files.
- More coarse-grained approach.

**Terraform itself**:
- Stores backend config hash in `.terraform/terraform.tfstate`.
- Compares current config hash to stored hash.
- Commands fail with "please run terraform init" message.

## Solution

Implement a two-phase auto-init detection strategy:

### Phase 1: Pre-Check (Fast Path)

Before running any terraform command that requires backend/providers, check if initialization is obviously needed:

```
if NOT exists(.terraform/terraform.tfstate) OR NOT exists(.terraform.lock.hcl):
    run terraform init
```

This catches the common case of a fresh checkout or new component.

### Phase 2: Error-Based Retry (Fallback)

If the terraform command fails and stderr/stdout contains the phrase `please run "terraform init"`:

```
1. Run terraform init (once only)
2. Retry the original command
3. If retry fails, return that error (no infinite loops)
```

This catches edge cases that file-based detection misses:
- Backend configuration changes.
- Provider version constraint changes.
- Module source changes.
- Lock file corruption.

### Why This Approach

| Approach | Pros | Cons |
|----------|------|------|
| Always init (current) | Simple, always works | Slow, wasteful |
| Hash-based detection | Fast, accurate | Complex, reimplements terraform logic |
| File existence check | Fast, simple | Misses some cases |
| Error-based retry | Catches all cases | Slightly slower on first failure |
| **Combined (proposed)** | Fast + reliable | Minimal complexity |

The combined approach gives us:
- **Fast path**: File check catches 90%+ of cases instantly.
- **Reliable fallback**: Error detection catches remaining edge cases.
- **Single retry**: Prevents infinite loops while ensuring recovery.

## Command Interface

### New Flags

| Flag | Short | Default | Description |
|------|-------|---------|-------------|
| `--auto-init` | | `true` | Enable automatic init detection and retry |
| `--skip-init` | | `false` | Skip init entirely (existing flag, unchanged) |

Note: `--skip-init` takes precedence over `--auto-init`.

### Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `ATMOS_TERRAFORM_AUTO_INIT` | `true` | Enable/disable auto-init detection |

### Configuration (atmos.yaml)

```yaml
components:
  terraform:
    # Existing settings
    deploy_run_init: true        # Still respected for deploy command
    init_run_reconfigure: false  # Still used when init runs

    # New setting
    auto_init: true              # Enable intelligent init detection (default: true)
```

## Implementation Details

### Working Directory Awareness

The component path used for file checks MUST be workdir-aware. When Atmos resolves the component path, it considers:
- `base_path` from atmos.yaml.
- Component-specific `terraform_workspace` settings.
- Any path overrides from stack configuration.

```go
// The componentPath passed to NeedsInit is already resolved by Atmos
// from internal/exec/terraform.go - uses info.ComponentFolderPrefix + info.Component
componentPath := constructTerraformComponentWorkingDir(atmosConfig, info)
```

### Detection Logic

```go
// pkg/terraform/init/detector.go

// NeedsInit checks if terraform init is required before running a command.
// componentPath must be the fully-resolved working directory (workdir-aware).
func NeedsInit(componentPath string) bool {
    tfStatePath := filepath.Join(componentPath, ".terraform", "terraform.tfstate")
    lockFilePath := filepath.Join(componentPath, ".terraform.lock.hcl")

    // If either critical file is missing, init is needed
    if !fileExists(tfStatePath) || !fileExists(lockFilePath) {
        return true
    }

    return false
}

// NeedsInitFromError checks if a terraform error indicates init is required.
func NeedsInitFromError(output string) bool {
    // Terraform's canonical error message
    return strings.Contains(strings.ToLower(output), `please run "terraform init"`) ||
           strings.Contains(strings.ToLower(output), "please run 'terraform init'")
}
```

### Leveraging the I/O Layer

Atmos has a well-designed I/O infrastructure in `pkg/io/` that we should extend for output capture:

**Existing Patterns to Leverage:**

1. **`quietModeWriter`** (from `pkg/terraform/output/executor.go`):
   ```go
   type quietModeWriter struct {
       buffer *strings.Builder
   }
   ```
   Used for suppressing output while capturing for error messages.

2. **`dynamicMaskedWriter`** (from `pkg/io/streams.go`):
   Wraps writers with automatic secret masking at write time.

**New Infrastructure:**

Create a `TeeWriter` in `pkg/io/` that both streams to the user AND captures for processing:

```go
// pkg/io/tee.go

// TeeWriter writes to multiple destinations simultaneously.
// Primary use case: Stream to user while capturing for error detection.
type TeeWriter struct {
    primary io.Writer      // os.Stdout or os.Stderr (user sees this)
    capture *bytes.Buffer  // Captured for processing
}

func NewTeeWriter(primary io.Writer) *TeeWriter {
    return &TeeWriter{
        primary: primary,
        capture: &bytes.Buffer{},
    }
}

func (t *TeeWriter) Write(p []byte) (n int, err error) {
    // Write to capture buffer (always succeeds for bytes.Buffer)
    t.capture.Write(p)
    // Write to primary destination
    return t.primary.Write(p)
}

func (t *TeeWriter) String() string {
    return t.capture.String()
}
```

**Integration with Masking:**

The TeeWriter can be wrapped with `io.MaskWriter()` to ensure secrets are masked before reaching either destination:

```go
// Create masked tee writer
tee := io.NewTeeWriter(os.Stderr)
maskedTee := io.MaskWriter(tee)
cmd.Stderr = maskedTee

// After command completes, check captured output
if NeedsInitFromError(tee.String()) {
    // Retry with init
}
```

### Execution Flow

```
ExecuteTerraform(command, args)
│
├─ if skipInit:
│   └─ run command directly
│
├─ if autoInit enabled:
│   ├─ componentPath = constructTerraformComponentWorkingDir(atmosConfig, info)
│   │
│   ├─ if NeedsInit(componentPath):
│   │   └─ run terraform init
│   │
│   ├─ run terraform command (with TeeWriter capturing output)
│   │
│   └─ if command failed AND NeedsInitFromError(capturedOutput):
│       ├─ ui.Warning("Terraform requires initialization. Running 'terraform init'...")
│       ├─ run terraform init
│       └─ retry terraform command (once only)
│
└─ else (autoInit disabled, legacy behavior):
    ├─ run terraform init
    └─ run terraform command
```

### Commands That Trigger Auto-Init

Any terraform command that requires backend or providers:
- `plan`, `apply`, `destroy`
- `output`, `show`, `state *`
- `refresh`, `import`, `taint`, `untaint`
- `workspace select`, `workspace new`
- `validate` (requires providers)
- `console`, `graph`, `providers`

Commands that do NOT require init:
- `init` (obviously)
- `version`
- `fmt`
- `-help` on any command

### Retry Messaging

When auto-init retry occurs, inform the user via the UI layer:

```go
ui.Warning("Terraform requires initialization. Running 'terraform init'...")
// [terraform init output]
ui.Success("Initialization complete. Retrying command...")
// [original command output]
```

## Scope

### In Scope (v1)

- File-based pre-check (`.terraform/terraform.tfstate`, `.terraform.lock.hcl`).
- Error-based retry detection (single retry only).
- `--auto-init` flag and config option.
- Environment variable support.
- Works with existing `--skip-init` flag.
- Workdir-aware path resolution.
- Integration with `pkg/io/` layer via TeeWriter.

### Out of Scope (Future)

- Hash-based backend config detection (reimplementing terraform logic).
- Caching init state across commands in same session.
- Parallel init optimization for `--all` commands.
- Init timing/performance metrics.
- Custom init retry patterns.

## Testing Strategy

### Unit Tests

1. **NeedsInit detection**
   - Missing `.terraform` directory → true.
   - Missing `terraform.tfstate` → true.
   - Missing `.terraform.lock.hcl` → true.
   - Both files present → false.

2. **NeedsInitFromError detection**
   - Contains `please run "terraform init"` → true.
   - Contains `please run 'terraform init'` → true.
   - Other error messages → false.
   - Case insensitivity.

3. **TeeWriter**
   - Writes to both primary and capture.
   - Works with masking wrapper.
   - String() returns captured content.

4. **Flag precedence**
   - `--skip-init` overrides `--auto-init`.
   - Config value respected when no flag.
   - Environment variable works.

### Integration Tests

1. **Fresh component** - Init runs automatically on first plan.
2. **Initialized component** - Init skipped, command runs directly.
3. **Backend change** - Init retry triggered after error.
4. **Skip-init honored** - No init even when needed (command fails).
5. **Retry limit** - Only one retry, then fail.

### Test Fixtures

Add to `tests/test-cases/`:
```
auto-init/
├── fresh-component/      # No .terraform directory
├── initialized/          # Has .terraform with valid state
├── missing-lock/         # Has .terraform but no lock file
└── backend-change/       # Initialized but backend config changed
```

## File Structure

```
pkg/io/
├── tee.go                # TeeWriter for output capture
└── tee_test.go           # TeeWriter tests

pkg/terraform/init/
├── detector.go           # NeedsInit, NeedsInitFromError functions
├── detector_test.go      # Unit tests
└── doc.go               # Package documentation

internal/exec/
└── terraform.go          # Modified execution flow (existing file)

docs/prd/
└── terraform-auto-init.md  # This PRD
```

## Success Metrics

1. **Performance improvement** - 50%+ reduction in init runs for typical workflows.
2. **Zero false negatives** - Init always runs when actually needed (error retry catches edge cases).
3. **Minimal false positives** - Init runs at most once extra per command (on retry).
4. **Backward compatibility** - Existing `--skip-init` workflows unchanged.
5. **Test coverage** - >80% coverage on new detection code.

## References

- [GitHub Issue #620](https://github.com/cloudposse/atmos/issues/620) - Original feature request.
- [Terraform init command](https://developer.hashicorp.com/terraform/cli/commands/init).
- [Terraform backend configuration](https://developer.hashicorp.com/terraform/language/backend).
- [Terragrunt auto-init](https://terragrunt.gruntwork.io/docs/features/auto-init/).
- Existing I/O patterns: `pkg/io/streams.go`, `pkg/terraform/output/executor.go`.
