---
slug: workflow-file-auto-discovery
title: "Workflow File Auto-Discovery: Run Workflows Without Specifying Files"
sidebar_label: "Workflow Auto-Discovery"
authors: [osterman]
tags: [enhancement, workflows, developer-experience]
date: 2025-11-18
---

The `atmos workflow` command now automatically discovers workflow files, eliminating the need to specify `--file` for uniquely named workflows. This developer experience improvement makes running workflows faster and more intuitive.

<!--truncate-->

## What's New

Previously, running a workflow required explicitly specifying the workflow file:

```bash
# Before: Always needed --file flag
atmos workflow deploy --file workflows/deploy.yaml
```

Now, if your workflow name is unique across all workflow files, Atmos automatically finds it:

```bash
# After: Just specify the workflow name
atmos workflow deploy
```

## Why This Matters

### Faster Workflow Execution

**Before:** You had to remember which file contained each workflow:

```bash
# Which file was it in again?
atmos workflow deploy --file workflows/deploy.yaml
atmos workflow test --file workflows/ci.yaml
atmos workflow cleanup --file workflows/cleanup.yaml
```

**After:** Just run the workflow by name:

```bash
# Atmos finds it automatically
atmos workflow deploy
atmos workflow test
atmos workflow cleanup
```

### Better Developer Experience

The `--file` flag is now **optional** for most workflows. You only need it when:
- Multiple workflow files contain a workflow with the same name
- You want to explicitly specify which file to use

### Consistent with Other Commands

This brings workflow execution in line with other Atmos commands that auto-discover resources:

```bash
# Components auto-discovery (existing)
atmos terraform plan vpc -s prod

# Workflows auto-discovery (new)
atmos workflow deploy
```

## How It Works

When you run `atmos workflow <name>` without `--file`:

1. **Scans workflow directory** - Atmos searches all YAML files in your configured workflows path
2. **Finds matching workflows** - Looks for workflows with the specified name
3. **Auto-selects if unique** - If only one file contains that workflow name, it runs automatically
4. **Prompts if multiple** - If multiple files have the same workflow name, shows an interactive selector

### Interactive Selection for Duplicates

If multiple workflow files contain the same workflow name, Atmos presents an interactive selector:

```bash
$ atmos workflow deploy

Found workflow 'deploy' in multiple files:
  1. workflows/production.yaml
  2. workflows/staging.yaml
  3. workflows/development.yaml

Select which workflow to run:
```

You can still use `--file` to skip the prompt:

```bash
atmos workflow deploy --file workflows/production.yaml
```

## Backward Compatibility

All existing workflows continue to work exactly as before:

```bash
# Explicit --file flag still works
atmos workflow deploy --file workflows/deploy.yaml

# Auto-discovery is purely additive
atmos workflow deploy
```

## Examples

### Basic Usage

Run a workflow by name (auto-discovers the file):

```bash
$ atmos workflow deploy
```

### With Additional Flags

All workflow flags work with auto-discovery:

```bash
# Run with dry-run
$ atmos workflow deploy --dry-run

# Run for specific stack
$ atmos workflow deploy --stack prod

# Resume from a specific step
$ atmos workflow deploy --from-step validate

# Specify identity for authentication
$ atmos workflow deploy --identity prod-admin
```

### Explicit File Selection

When you need precise control:

```bash
# Explicitly specify which file
$ atmos workflow deploy --file workflows/production.yaml

# Useful in scripts or CI/CD
$ atmos workflow deploy \
    --file workflows/production.yaml \
    --stack prod \
    --identity prod-deployer
```

## Interactive TUI Still Available

Running `atmos workflow` without any arguments still launches the interactive TUI:

```bash
# Interactive workflow browser
$ atmos workflow
```

This shows all available workflows across all files, allowing you to browse and select interactively.

## Implementation Details

### Resilient File Discovery

The auto-discovery implementation is robust and handles edge cases gracefully:

- **Skips unreadable files** - Permission errors don't stop discovery
- **Ignores invalid YAML** - Malformed files are skipped with warnings
- **Handles missing keys** - Files without `workflows:` key are ignored
- **Logs warnings** - All skipped files are logged for debugging

### Performance Considerations

File discovery is fast and efficient:

- **Cached results** - Workflow metadata is cached during discovery
- **Minimal I/O** - Only reads files once during the search
- **Early termination** - Stops searching after finding a unique match

## Migration Guide

No migration needed! This feature is fully backward compatible. You can:

1. **Keep using `--file`** - All existing scripts continue to work
2. **Gradually adopt auto-discovery** - Remove `--file` from workflows where convenient
3. **Mix both approaches** - Use auto-discovery for ad-hoc commands, `--file` for scripts

## Technical Details

### For Developers

Implementation highlights:

- **Smart file scanning** - Recursively searches configured workflow paths
- **Error handling** - Gracefully skips invalid files with informative warnings
- **Interactive prompts** - Uses Atmos TUI components for multiple-match scenarios
- **Test coverage** - Comprehensive test suite covering all discovery scenarios

Key files:

- `internal/exec/workflow_utils.go` - Auto-discovery implementation
- `internal/exec/workflow_test.go` - Test coverage for discovery logic
- `cmd/workflow/workflow.go` - Updated command implementation

## Get Started

This feature is available now. Just upgrade to the latest Atmos release:

```bash
# Check your version
atmos version

# Try auto-discovery
atmos workflow <your-workflow-name>
```

## Get Involved

We're building Atmos in the open and welcome your feedback:

- 💬 **Discuss** - Share thoughts in [GitHub Discussions](https://github.com/cloudposse/atmos/discussions)
- 🐛 **Report Issues** - Found a bug? [Open an issue](https://github.com/cloudposse/atmos/issues)
- 🚀 **Contribute** - Want to add features? Review our [contribution guide](https://atmos.tools/community/contributing)

---

**Related Documentation:**
- [Workflows Guide](https://atmos.tools/core-concepts/workflows)
- [Workflow Command Reference](https://atmos.tools/cli/commands/workflow)
