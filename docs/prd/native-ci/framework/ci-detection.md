# Native CI Integration - CI Detection & Command Parity

> Related: [Overview](../overview.md) | [Configuration](./configuration.md) | [Hooks Integration](./hooks-integration.md)

## FR-1: CI Environment Detection

**Requirement**: Atmos automatically detects CI environments without explicit flags.

**Behavior**:
- Detect GitHub Actions via `GITHUB_ACTIONS=true` environment variable
- Detect other CI providers via standard provider-specific environment variables
- Allow explicit override via `--ci` flag for local testing
- Gracefully degrade when CI features unavailable (e.g., missing `$GITHUB_STEP_SUMMARY`)

### `ci.enabled` is a hard kill switch

`ci.enabled` in `atmos.yaml` is a `bool` field (not `*bool`), so "unset" and `false` are identical (both `false`). The `RunCIHooks()` check in `pkg/hooks/hooks.go` is:

```go
if atmosConfig != nil && !atmosConfig.CI.Enabled {
    return nil  // Skip CI hooks — ci.enabled is the authority
}
```

`ci.enabled` is the **authority** for CI hooks. When `ci.enabled` is `false` (or not set, which defaults to `false`), CI hooks are disabled regardless of the `--ci` flag or any environment variables. The `--ci` flag / `ATMOS_CI` env var only controls provider fallback behavior (generic vs auto-detect) but cannot override a disabled config.

> **Note**: The `--ci` flag is bound to both `ATMOS_CI` and the generic `CI` env var via `flags.WithEnvVars("ci", "ATMOS_CI", "CI")`. The generic `CI` env var (set by all CI runners) implicitly activates `--ci` on every CI run — this is safe because `ci.enabled` in `atmos.yaml` is the hard kill switch that gates all CI hooks.

| `ci.enabled` config | `--ci` flag / `ATMOS_CI` / `CI` | Platform detected | Result |
|--------------------:|:------------------------:|:-----------------:|--------|
| false/unset | true | yes | **CI disabled** (`ci.enabled` is the authority) |
| false/unset | true | no | **CI disabled** (`ci.enabled` is the authority) |
| false/unset | false | any | **CI disabled** (`ci.enabled` is the authority) |
| true | true | yes | CI enabled (detected provider) |
| true | true | no | CI enabled (generic fallback) |
| true | false | yes | CI enabled (auto-detected provider) |
| true | false | no | CI disabled (no provider available) |

**Validation**:
- `ci.enabled: true` + running in GitHub Actions enables CI hooks (provider auto-detected)
- `ci.enabled: true` + `--ci` flag locally enables CI hooks with generic provider fallback
- `ci.enabled: false` (or unset) disables CI hooks unconditionally — `--ci` flag cannot override
- Missing CI environment variables do not cause errors (graceful degradation)

## FR-7: Command Parity

**Requirement**: Same command produces same behavior in CI and locally.

**Behavior**:
- `atmos terraform plan vpc -s prod` works identically everywhere
- CI mode adds outputs (summary, variables) without changing core behavior
- Local `--ci` flag enables CI output for testing (requires `ci.enabled: true` in config)
- No CI-specific command variations

**Validation**:
- Plan output content identical in CI and local
- Resource change detection identical
- Exit codes identical

## Same Command Everywhere

The same command works identically in CI and locally:

```bash
# In GitHub Actions (requires ci.enabled: true in atmos.yaml)
atmos terraform plan vpc -s plat-ue2-dev

# Local development (force CI mode — also requires ci.enabled: true)
atmos terraform plan vpc -s plat-ue2-dev --ci
```

## Key Design Decision: Use Atmos Lifecycle Hooks (IMPLEMENTED)

CI behaviors are triggered via `RunCIHooks()` (defined in `pkg/hooks/hooks.go`, called from `cmd/terraform/utils.go`), which calls `ci.Execute()`. The executor resolves the plugin and invokes the handler callback:

```go
"before.terraform.plan"   → onBeforePlan()    // createCheckRun (in_progress)
"after.terraform.plan"    → onAfterPlan()     // writeSummary + writeOutputs + uploadPlanfile + updateCheckRun
"before.terraform.apply"  → onBeforeApply()   // createCheckRun (in_progress)
"after.terraform.apply"   → onAfterApply()    // writeSummary + writeOutputs + updateCheckRun
"before.terraform.deploy" → onBeforeDeploy()  // createCheckRun + downloadPlanfile (with stored.* prefix for verification)
"after.terraform.deploy"  → onAfterDeploy()   // writeSummary + writeOutputs + updateCheckRun (delegates to onAfterApply)
```

This keeps CI behaviors modular — each plugin defines its own hook bindings with handler callbacks.

**Command responsibility:**
- **`plan`** uploads planfiles, writes summaries/checks/outputs
- **`apply`** writes summaries/checks/outputs only — does NOT interact with planfile storage
- **`deploy`** downloads stored planfiles, verifies, applies — the CI-native apply command

> **Wiring status**: All three commands (`plan.go`, `apply.go`, `deploy.go`) fully wire the CI lifecycle. `plan` and `apply` fire CI hooks from `PreRunE`/`PostRunE`. `deploy` fires `before.terraform.deploy` from inside `terraformRunWithOptions` (after `ProcessCommandLineArgs` resolves stacks) because `PreRunE` would eagerly resolve `!store` YAML functions for all stacks before dependencies are deployed. All three commands support `--ci` flag with `ATMOS_CI` env var binding. Deploy fires its own `before/after.terraform.deploy` events (not apply events).

## Flag Changes

**Terraform (persistent):**

| Flag | Environment Variable | Description |
|------|---------------------|-------------|
| `--ci` | `ATMOS_CI`, `CI` | Enable CI mode (force provider detection / generic fallback) |

The `--ci` flag is bound to both `ATMOS_CI` and the generic `CI` environment variable (set by all major CI providers like GitHub Actions, GitLab CI, etc.). This means `--ci` is implicitly `true` on every CI run. This is safe because `ci.enabled` in `atmos.yaml` is the hard kill switch — when `ci.enabled` is `false` (or not set), CI hooks are disabled unconditionally regardless of the `--ci` flag value.

```go
// pkg/flags binding
flags.WithBoolFlag("ci", "", false, "Enable CI mode"),
flags.WithEnvVars("ci", "ATMOS_CI", "CI"),  // Both ATMOS_CI and generic CI — safe because ci.enabled is the authority
```
