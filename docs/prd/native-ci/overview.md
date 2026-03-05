# Native CI Integration - Overview

> Related: [CI Detection](./ci-detection.md) | [Configuration](./configuration.md) | [Implementation Status](./implementation-status.md)

## Executive Summary

When you see complex bash scripts and conditional logic in GitHub Actions workflows, that's a signal: the underlying tool wasn't designed for CI. Native CI integration fixes this by making Atmos work identically in CI and locally—no wrapper scripts, no special actions, no hidden complexity.

## Problem Statement

### The Hidden Complexity Problem

Look at any mature infrastructure repository's CI workflows. You'll find:

- Bash scripts parsing terraform output with `grep` and `awk`
- Conditional logic to handle plan files across jobs
- Custom actions wrapping CLI tools with CI-specific behaviors
- Environment variable gymnastics to pass data between steps

This complexity isn't accidental—it's compensation. The underlying tools weren't designed for CI, so teams build layers of glue code to bridge the gap.

**The cost is real:**
- Workflows that work in CI fail locally (and vice versa)
- Debugging requires reproducing the entire CI environment
- Every CI provider needs different glue code
- Tribal knowledge accumulates in workflow files

### The Reproducibility Principle

Infrastructure tools should follow a simple principle: **the same command should produce the same behavior everywhere.**

```bash
# This should work identically:
atmos terraform plan vpc -s prod    # locally
atmos terraform plan vpc -s prod    # in GitHub Actions
atmos terraform plan vpc -s prod    # in GitLab CI
```

When a tool is truly CI-native, your workflow files become trivial:

```yaml
# Before: Complex workflow with hidden logic
- name: Plan
  run: |
    output=$(atmos terraform plan vpc -s prod 2>&1)
    echo "$output"
    # Parse for changes...
    # Upload artifacts...
    # Post PR comment...

# After: CI-native tool
- name: Plan
  run: atmos terraform plan vpc -s prod
```

### Current State

Today, Atmos users rely on external GitHub Actions:
- `github-action-atmos-terraform-plan` - Wraps CLI with CI behaviors
- `github-action-atmos-terraform-apply` - Wraps CLI with CI behaviors

This creates the exact hidden complexity we want to eliminate:
1. **Different behavior** - Local runs differ from CI runs
2. **Two codebases** - CLI and actions evolve separately
3. **Platform lock-in** - Actions are GitHub-specific
4. **Glue scripts** - Bash to connect artifacts, comments, outputs
5. **Debugging friction** - Can't reproduce CI locally

### Desired State

Atmos becomes CI-native. The CLI detects when it's running in CI and automatically:
- Writes rich summaries to `$GITHUB_STEP_SUMMARY`
- Posts status checks ("Plan in progress" → "3 to add, 1 to destroy")
- Exports outputs to `$GITHUB_OUTPUT`
- Stores planfiles for later apply
- Updates PR comments with results

Locally, `--ci` produces identical output—same formatting, same behavior. Debug CI issues on your laptop.

## Non-Functional Requirements

### NFR-1: Performance

**Requirement**: CI operations add minimal overhead.

**Targets**:
- Summary generation: < 100ms
- Output variable writing: < 50ms per variable
- Planfile upload (excluding transfer): < 200ms
- CI detection: < 10ms

### NFR-2: Reliability

**Requirement**: CI feature failures do not block terraform operations.

**Behavior**:
- Summary write failure logs warning, does not fail command
- Output variable write failure logs warning per variable
- Planfile upload failure fails command (data integrity)
- Status check failure logs warning, does not fail command

### NFR-3: Security

**Requirement**: CI integration respects security boundaries.

**Behavior**:
- Never log sensitive terraform outputs
- Planfile storage inherits terraform state security model
- GitHub token scoped to minimum required permissions
- No secrets in job summaries or PR comments

### NFR-4: Extensibility

**Requirement**: CI behaviors are configurable and extensible.

**Behavior**:
- Template overrides for summary and comment formats
- Per-feature enable/disable configuration
- Storage backend registry pattern for new backends
- Provider interface for new CI platforms

## Success Criteria

1. **Parity with GitHub Actions** - All features of existing actions work natively
2. **Local reproducibility** - `--ci` flag produces identical output locally and in CI
3. **No DynamoDB** - Planfile metadata stored in same backend as planfiles
4. **Multi-backend support** - S3, Azure, GCS, GitHub Artifacts all working
5. **Documentation** - Complete docs for migration from GitHub Actions
6. **Performance** - No significant overhead compared to direct terraform calls

## Migration Path

Users currently using the GitHub Actions can migrate incrementally:

1. **Install updated Atmos** - Ensure version with native CI support
2. **Configure `ci:` section** - Add planfile storage configuration
3. **Update workflows** - Replace action with `atmos terraform plan --ci`
4. **Test locally** - Verify behavior matches using `--ci` flag
5. **Remove action dependencies** - Clean up workflow files

## FAQ

### Q: Will this replace the GitHub Actions?

**A:** Yes, the goal is to deprecate the external GitHub Actions in favor of native CLI support. The actions will be archived with migration documentation.

### Q: Can I use different storage backends for different environments?

**A:** Yes, configure multiple named stores in `components.terraform.planfiles.stores` and specify which to use via `--store` flag or per-stack configuration. Backend selection uses the `priority` list with environment-based availability detection.

### Q: How does authentication work?

**A:** Uses existing Atmos auth infrastructure (`atmos auth`). For GitHub API, uses `GITHUB_TOKEN` or `ATMOS_GITHUB_TOKEN`. For cloud storage, uses standard SDK credential chains.

### Q: What about GitLab CI?

**A:** The architecture supports multiple providers via the `Provider` interface. GitLab CI implementation is planned for Phase 2+.

### Q: How do terraform outputs get exported?

**A:** After a successful `terraform apply --ci`, all terraform outputs are exported to `$GITHUB_OUTPUT` using the format options from `pkg/terraform/output/`. This includes support for flattening nested outputs and uppercase conversion for environment variable compatibility.

## Scope

### In Scope

- **GitHub Actions provider** - Full implementation
- **S3, Azure Blob, GCS, GitHub Artifacts storage** - All backends
- **PR comments** - Create, update, upsert behaviors
- **Job summaries** - `$GITHUB_STEP_SUMMARY` integration
- **Status checks** - Post check runs when operations start/complete (requires `checks: write`)
- **CI outputs** - `$GITHUB_OUTPUT` for both plan and apply
- **Terraform outputs export** - After successful apply
- **`--format=matrix`** - For `describe affected` command
- **`--verify-plan`** - Using existing plan-diff
- **`atmos ci status`** - Show PR/commit status like `gh pr status`

### Out of Scope (Phase 2+)

- **`--affected`/`--all` with artifacts** - Running across all affected components
- **Infracost integration** - Cost estimation
- **GitLab CI provider** - Architecture ready but not implemented
- **Other CI providers** - CircleCI, Azure DevOps, etc.

## References

- [Existing CI Detection](../../pkg/telemetry/ci.go) - Detects 24+ CI providers
- [Lifecycle Hooks](../../pkg/hooks/) - Hook system for terraform events
- Plan-Diff (`internal/exec/terraform_plan_diff*.go`) - Semantic plan comparison (planned)
- [Store Registry](../../pkg/store/registry.go) - Pattern for planfile stores
- Terraform Output Package (`pkg/terraform/output/`) - Output formatting (planned, tf-output-format branch)
- [tfcmt](https://github.com/suzuki-shunsuke/tfcmt) - Inspiration for PR comments
- [GitHub Artifacts API v4](https://docs.github.com/en/actions/using-workflows/storing-workflow-data-as-artifacts)
