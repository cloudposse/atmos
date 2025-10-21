---
slug: vendor-update-and-diff
title: "Automated Component Updates with Vendor Update and Diff"
authors: [atmos]
tags: [feature, vendoring, automation, components]
date: 2025-10-21
---

Keeping vendored components up-to-date is critical for security, compatibility, and accessing new features. Today we're excited to announce two new commands that make managing vendored components safer and more automated: `atmos vendor update` and `atmos vendor diff`.

<!--truncate-->

## The Challenge

Teams using Atmos vendor components from external sources face several challenges:

- **Manual version tracking**: Checking GitHub, GitLab, or other sources for new releases
- **Update anxiety**: Fear of breaking changes when updating components
- **Time-consuming reviews**: Manually reviewing diffs between versions before updating
- **Inconsistent update policies**: Different teams using different versioning strategies
- **Security vulnerabilities**: Delayed updates due to manual overhead

Previously, you had to:
- Manually check GitHub releases or tags for new versions
- Edit `vendor.yaml` to update version numbers
- Run `atmos vendor pull` to download new versions
- Use external tools to diff the changes
- Hope you didn't introduce breaking changes

## The Solution

We've added two powerful commands that automate this workflow while giving you complete control and visibility.

### atmos vendor update

Automatically check for and update to newer versions of vendored components, with intelligent version constraints to ensure safe updates.

```bash
# Update a specific component
atmos vendor update --component vpc

# Update all components
atmos vendor update --all

# Dry run to see what would be updated
atmos vendor update --all --dry-run
```

**Key Features:**

- **Semantic version constraints**: Use caret (`^`), tilde (`~`), ranges, or wildcards
- **Version exclusion**: Blacklist specific versions or patterns
- **Pre-release filtering**: Automatically skip alpha/beta/rc versions
- **Dry run mode**: Preview updates before making changes
- **Automatic vendor.yaml updates**: Updates version fields in place

### atmos vendor diff

Compare two versions of a component before updating, with GitHub's native diff integration for rich, formatted output.

```bash
# Diff current version against latest
atmos vendor diff --component vpc

# Diff between specific versions
atmos vendor diff --component vpc --from 1.323.0 --to 1.400.0

# Focus on specific files
atmos vendor diff --component vpc --file main.tf

# Control output format
atmos vendor diff --component vpc --context 10 --no-color
```

**Key Features:**

- **GitHub Compare API integration**: Rich, formatted diffs for GitHub sources
- **File-specific diffs**: Focus on just the files you care about
- **Customizable context**: Control how many lines of context to show
- **Color/no-color output**: Terminal-friendly or pipeline-ready
- **Version comparison**: Compare any two versions, not just current vs. latest

## How It Works

### Version Constraints

Define update rules directly in your `vendor.yaml`:

```yaml
sources:
  - component: "vpc"
    source: "github.com/cloudposse/terraform-aws-components//modules/vpc?ref={{.Version}}"
    version: "1.323.0"  # Current pinned version
    constraints:
      version: "^1.0.0"           # Stay on v1.x, allow minor/patch updates
      excluded_versions:
        - "1.100.0"                # Skip version with breaking bug
        - "1.5.*"                  # Skip problematic 1.5.x series
      no_prereleases: true         # Only stable releases
    targets:
      - "components/terraform/vpc"
```

**Constraint Syntax:**

- **Caret (`^`)**: Compatible updates (minor and patch)
  - `^1.0.0` → allows `1.0.0` to `1.999.999`, blocks `2.0.0`
- **Tilde (`~`)**: Patch-level updates only
  - `~1.2.0` → allows `1.2.0` to `1.2.999`, blocks `1.3.0`
- **Ranges**: Explicit boundaries
  - `>=1.0.0 <2.0.0` → any 1.x version
- **Wildcards**: Flexible matching
  - `1.x` or `1.*` → any 1.x version

### Update Process

When you run `atmos vendor update --component vpc`:

1. **Fetch available versions** from the source repository
2. **Apply constraints** to filter valid versions
3. **Exclude blacklisted versions** from consideration
4. **Filter pre-releases** if configured
5. **Select latest remaining version**
6. **Update `vendor.yaml`** with new version
7. **Pull new component** (or use `--dry-run` to preview)

### Diff Workflow

Before updating, review what changed:

```bash
# Step 1: Check for updates
atmos vendor update --component vpc --dry-run

# Output:
# Component "vpc" can be updated:
#   Current version: 1.323.0
#   Latest version:  1.400.0

# Step 2: Review the diff
atmos vendor diff --component vpc --from 1.323.0 --to 1.400.0

# Output shows GitHub-style diff of all changes

# Step 3: If satisfied, update
atmos vendor update --component vpc
```

## Real-World Examples

### Safe Automated Updates

Lock to major versions while allowing safe updates:

```yaml
sources:
  - component: "eks"
    source: "github.com/cloudposse/terraform-aws-eks-cluster?ref={{.Version}}"
    version: "2.5.0"
    constraints:
      version: "^2.0.0"        # Stay on v2, allow minor/patch
      no_prereleases: true      # Production stability
    targets:
      - "components/terraform/eks"
```

### Avoiding Known Issues

Skip specific problematic versions:

```yaml
sources:
  - component: "rds"
    source: "github.com/cloudposse/terraform-aws-rds?ref={{.Version}}"
    version: "1.10.0"
    constraints:
      version: "^1.0.0"
      excluded_versions:
        - "1.5.0"      # Has critical bug
        - "1.6.*"      # Entire series has issues
    targets:
      - "components/terraform/rds"
```

### Controlled Pre-release Testing

Test pre-releases in dev, stable in prod:

```yaml
# dev/vendor.yaml
sources:
  - component: "app"
    version: "2.0.0-beta.5"
    constraints:
      version: "^2.0.0-0"  # Allow pre-releases
    targets:
      - "components/terraform/app"

# prod/vendor.yaml
sources:
  - component: "app"
    version: "1.8.0"
    constraints:
      version: "^1.0.0"
      no_prereleases: true   # Stable only
    targets:
      - "components/terraform/app"
```

### Bulk Updates with Review

Update all components safely:

```bash
# 1. See what would change
atmos vendor update --all --dry-run

# 2. Review diffs for critical components
atmos vendor diff --component vpc
atmos vendor diff --component eks
atmos vendor diff --component rds

# 3. Update everything
atmos vendor update --all
```

## Multi-Provider Architecture

The diff functionality uses a provider-based architecture:

- **GitHub sources**: Full diff support using GitHub Compare API
- **Generic Git sources**: Basic operations, diff returns "not implemented"
- **Other sources** (OCI, local, HTTP): Gracefully handled with clear errors

This design allows us to provide the best experience for each source type while maintaining a consistent interface.

## Integration with CI/CD

Automate dependency updates in your pipelines:

```yaml
# .github/workflows/update-components.yml
name: Update Vendored Components
on:
  schedule:
    - cron: '0 0 * * 1'  # Weekly on Monday
  workflow_dispatch:

jobs:
  update:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3

      - name: Install Atmos
        run: |
          curl -L https://github.com/cloudposse/atmos/releases/download/vX.X.X/atmos -o /usr/local/bin/atmos
          chmod +x /usr/local/bin/atmos

      - name: Check for updates
        id: check
        run: |
          atmos vendor update --all --dry-run > updates.txt
          cat updates.txt

      - name: Create PR if updates available
        if: contains(steps.check.outputs.stdout, 'can be updated')
        uses: peter-evans/create-pull-request@v5
        with:
          commit-message: "chore: Update vendored components"
          title: "Update Vendored Components"
          body: |
            Automated component updates detected.

            ```
            $(cat updates.txt)
            ```

            Review the diffs and merge if acceptable.
          branch: automated-vendor-updates
```

## Error Handling and Safety

Both commands provide comprehensive error handling:

- **Version not found**: Clear message with available versions
- **Constraint syntax errors**: Helpful validation messages
- **Network issues**: Graceful degradation with retry suggestions
- **No updates available**: Informative message about current state
- **Unsupported sources**: Clear explanation of capabilities per source type

## Migration Guide

Existing `vendor.yaml` files work without changes. To add constraints:

```yaml
# Before (still works)
sources:
  - component: "vpc"
    source: "github.com/cloudposse/terraform-aws-components//modules/vpc?ref={{.Version}}"
    version: "1.323.0"
    targets:
      - "components/terraform/vpc"

# After (with automated updates)
sources:
  - component: "vpc"
    source: "github.com/cloudposse/terraform-aws-components//modules/vpc?ref={{.Version}}"
    version: "1.323.0"
    constraints:          # Add this section
      version: "^1.0.0"
      no_prereleases: true
    targets:
      - "components/terraform/vpc"
```

## Why This Matters

**Security**: Stay up-to-date with security patches without manual intervention

**Confidence**: Review diffs before updating to catch breaking changes

**Automation**: Integrate into CI/CD for continuous dependency management

**Control**: Fine-grained version constraints prevent unwanted updates

**Visibility**: Clear output shows exactly what changed and why

## Technical Implementation

For contributors and the curious:

- **Provider interface pattern**: Clean abstraction for different source types
- **GitHub Compare API**: Native integration for rich diffs
- **Semver library**: Industry-standard constraint parsing
- **Error wrapping**: Comprehensive error context for debugging
- **Performance tracking**: Instrumented for monitoring and optimization

See [docs/prd/vendor-update.md](https://github.com/cloudposse/atmos/blob/main/docs/prd/vendor-update.md) for complete technical specifications.

## Getting Started

Available in Atmos vX.X.X and later:

```bash
# Install or upgrade Atmos
brew upgrade atmos  # macOS
# or download from GitHub releases

# Try it out
atmos vendor update --component vpc --dry-run
atmos vendor diff --component vpc
```

## Resources

- [`atmos vendor update` Documentation](/cli/commands/vendor/update)
- [`atmos vendor diff` Documentation](/cli/commands/vendor/diff)
- [Vendor Manifest Reference](/core-concepts/vendor/vendor-manifest)
- [Vendoring Cheatsheet](/cheatsheets/vendoring)
- [GitHub Repository](https://github.com/cloudposse/atmos)

## What's Next

We're exploring additional enhancements:

- **Change detection**: Notify when new versions are available
- **Rollback support**: Quickly revert to previous versions
- **Batch operations**: Update multiple components with single command
- **Custom update hooks**: Run validation after updates
- **Provider expansion**: Support for more source types

---

*Have feedback or questions? Join our [Slack community](https://slack.cloudposse.com/) or [open an issue on GitHub](https://github.com/cloudposse/atmos/issues).*
