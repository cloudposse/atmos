# Native CI Integration - CI Detection & Command Parity

> Related: [Overview](../overview.md) | [Configuration](./configuration.md) | [Hooks Integration](./hooks-integration.md)

## FR-1: CI Environment Detection

**Requirement**: Atmos automatically detects CI environments without explicit flags.

**Behavior**:
- Detect GitHub Actions via `GITHUB_ACTIONS=true` environment variable
- Detect other CI providers via standard `CI=true` environment variable
- Allow explicit override via `--ci` flag for local testing
- Gracefully degrade when CI features unavailable (e.g., missing `$GITHUB_STEP_SUMMARY`)

**`ci.enabled` controls auto-detection gate:**

`ci.enabled` in `atmos.yaml` is a `bool` field (not `*bool`), so "unset" and `false` are identical (both `false`). The `RunCIHooks()` check in `pkg/hooks/hooks.go` is:

```go
if !forceCIMode && atmosConfig != nil && !atmosConfig.CI.Enabled {
    return nil  // Skip CI hooks
}
```

This means `--ci` flag (`forceCIMode`) **bypasses** the `ci.enabled` check. When `forceCIMode` is true, CI hooks run regardless of `ci.enabled`.

> **Note**: The `--ci` flag is bound to `CI` and `ATMOS_CI` env vars via `flags.WithEnvVars("ci", "ATMOS_CI", "CI")`. In CI environments where `CI=true` is set (GitHub Actions, GitLab CI, etc.), `forceCIMode` is automatically true.

| `ci.enabled` config | `--ci` flag / `CI` env var | Platform detected | Result |
|--------------------:|:--------------------------:|:-----------------:|--------|
| false/unset | true | yes | CI enabled (detected provider) |
| false/unset | true | no | CI enabled (generic fallback) |
| false/unset | false | any | **CI disabled** (both gates fail) |
| true | true | yes | CI enabled (detected provider) |
| true | true | no | CI enabled (generic fallback) |
| true | false | yes | CI enabled (auto-detected provider) |
| true | false | no | CI disabled (no provider available) |

> **Design note**: The original PRD intended `ci.enabled: false` to be a "hard kill switch" that overrides `--ci`. The current implementation does not enforce this — `--ci` always bypasses `ci.enabled`. This may be revisited in a future refactoring to use `*bool` for `Enabled` and check it independently of `forceCIMode`.

**Validation**:
- Running in GitHub Actions automatically enables CI mode (via `CI=true` env var → `--ci` flag)
- Running locally with `--ci` produces identical output format
- `ci.enabled: false` without `--ci` disables CI hooks (but `--ci` bypasses this check)
- Missing CI environment variables do not cause errors

## FR-7: Command Parity

**Requirement**: Same command produces same behavior in CI and locally.

**Behavior**:
- `atmos terraform plan vpc -s prod` works identically everywhere
- CI mode adds outputs (summary, variables) without changing core behavior
- Local `--ci` flag enables CI output for testing
- No CI-specific command variations

**Validation**:
- Plan output content identical in CI and local
- Resource change detection identical
- Exit codes identical

## Same Command Everywhere

The same command works identically in CI and locally:

```bash
# In GitHub Actions (auto-detected)
atmos terraform plan vpc -s plat-ue2-dev

# Local development (force CI mode)
atmos terraform plan vpc -s plat-ue2-dev --ci
```

## Key Design Decision: Use Atmos Lifecycle Hooks (IMPLEMENTED)

CI behaviors are triggered via `RunCIHooks()` (defined in `pkg/hooks/hooks.go`, called from `cmd/terraform/utils.go`), which calls `ci.Execute()`. The executor dispatches to plugin hook bindings:

```go
BeforeTerraformPlan  = "before.terraform.plan"   // ActionCheck: create check run (in_progress)
AfterTerraformPlan   = "after.terraform.plan"    // ActionSummary + ActionOutput + ActionUpload + ActionCheck
BeforeTerraformApply = "before.terraform.apply"  // ActionDownload: download planfile from store
AfterTerraformApply  = "after.terraform.apply"   // ActionSummary + ActionOutput
```

This keeps CI behaviors modular — each plugin defines its own hook bindings.

## Flag Changes

**Terraform (persistent):**

| Flag | Environment Variable | Description |
|------|---------------------|-------------|
| `--ci` | `CI` | Enable CI mode (auto-detected from `CI` env var) |

The `--ci` flag is bound to the `CI` environment variable, which is set by all major CI providers (GitHub Actions, GitLab CI, CircleCI, Jenkins, etc.). This means CI behaviors are automatically enabled when running in CI without requiring explicit flags.

```go
// pkg/flags binding
flags.WithBoolFlag("ci", "", false, "Enable CI mode"),
flags.WithEnvVars("ci", "ATMOS_CI", "CI"),  // ATMOS_CI takes precedence over CI
```
