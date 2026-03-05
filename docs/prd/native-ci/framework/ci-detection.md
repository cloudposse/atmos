# Native CI Integration - CI Detection & Command Parity

> Related: [Overview](../overview.md) | [Configuration](./configuration.md) | [Hooks Integration](./hooks-integration.md)

## FR-1: CI Environment Detection

**Requirement**: Atmos automatically detects CI environments without explicit flags.

**Behavior**:
- Detect GitHub Actions via `GITHUB_ACTIONS=true` environment variable
- Detect other CI providers via standard `CI=true` environment variable
- Allow explicit override via `--ci` flag for local testing
- Gracefully degrade when CI features unavailable (e.g., missing `$GITHUB_STEP_SUMMARY`)

**Validation**:
- Running in GitHub Actions automatically enables CI mode
- Running locally with `--ci` produces identical output format
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

## Key Design Decision: Use Atmos Lifecycle Hooks

Integrate CI behaviors at existing hook points in `pkg/hooks/`:

```go
BeforeTerraformInit  = "before.terraform.init"   // Download planfiles here
AfterTerraformPlan   = "after.terraform.plan"    // Upload planfiles, PR comment, job summary
AfterTerraformApply  = "after.terraform.apply"   // Update PR comment, job summary, export outputs
```

This keeps CI behaviors modular and allows users to extend or replace them.

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
