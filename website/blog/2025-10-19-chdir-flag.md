---
slug: introducing-chdir-flag
title: 'Introducing --chdir: Simplify Your Multi-Repo Workflows'
sidebar_label: Introducing --chdir Flag
authors:
  - osterman
tags:
  - feature
  - dx
date: 2025-10-19T00:00:00.000Z
release: v1.195.0
---

We're excited to announce a new global flag that makes working with Atmos across multiple repositories and directories significantly easier: `--chdir` (or `-C` for short).

<!--truncate-->

## The Problem

If you've ever worked with Atmos in a multi-repository setup or during development, you've probably faced these scenarios:

- **Development workflow**: You're building a new Atmos binary and want to test it against your infrastructure repo without installing it globally
- **CI/CD complexity**: Your build runs in one directory, but your infrastructure code lives elsewhere
- **Multi-repo operations**: You need to quickly check configurations across different infrastructure repositories
- **Script complexity**: Your automation scripts have to change directories manually, making them harder to read and maintain

Previously, you had to either:
- Manually `cd` to each directory before running Atmos
- Write wrapper scripts to handle directory changes
- Modify your `PATH` or use absolute paths to atmos binaries
- Copy binaries to specific locations

## The Solution

The new `--chdir` flag (and its short form `-C`) changes Atmos's working directory **before** any other operations, including configuration loading. This mirrors the familiar behavior of tools like `git -C` and `make -C`.

### Basic Usage

```bash
# Long form
atmos --chdir=/path/to/infrastructure describe stacks

# Short form (preferred for brevity)
atmos -C /infra terraform plan vpc -s prod

# Relative paths work too
atmos --chdir=../other-repo list components
```

### Development Workflow

Testing a development build against your infrastructure is now trivial:

```bash
# Build your changes
make build

# Point the dev binary at your infrastructure repo
./build/atmos -C ~/projects/my-infrastructure describe stacks

# No need to change directories or modify PATH
./build/atmos -C ~/projects/my-infrastructure terraform plan vpc -s dev
```

### CI/CD Pipelines

Your CI/CD workflows become cleaner and more explicit:

```bash
# Before: Manual directory management
cd /infrastructure
atmos terraform plan vpc -s prod
cd -

# After: Explicit and clear
atmos -C /infrastructure terraform plan vpc -s prod
```

### Environment Variable Support

For scripts and CI/CD environments, you can use the `ATMOS_CHDIR` environment variable:

```bash
export ATMOS_CHDIR=/infrastructure

# All atmos commands now run in that directory
atmos terraform plan vpc -s prod
atmos describe stacks
atmos list components
```

The CLI flag takes precedence over the environment variable, allowing you to override the default when needed:

```bash
export ATMOS_CHDIR=/default-infra

# Use the default
atmos describe stacks

# Override for specific command
atmos -C /other-infra describe stacks
```

## How It Works with --base-path

It's important to understand the difference between `--chdir` and `--base-path`:

- **`--chdir`**: Changes the **working directory** (like running `cd` first)
- **`--base-path`**: Overrides the **Atmos project root** (where `atmos.yaml` lives)

### Processing Order

1. `--chdir` executes first, changing the working directory
2. `--base-path` is then resolved relative to the new working directory
3. All other configuration loading and operations proceed

### Example

```bash
# Change to /infra directory, then look for atmos.yaml in ./config subdirectory
atmos -C /infra --base-path=./config terraform plan vpc -s dev

# This is equivalent to:
# cd /infra
# atmos --base-path=./config terraform plan vpc -s dev
```

### When to Use Each

- Use `--chdir` when you want Atmos to run as if you had changed directories first
- Use `--base-path` when your Atmos project root is in a non-standard location
- Combine them when your working directory and Atmos project root are in different places

```bash
# Just change working directory (atmos.yaml is in the root)
atmos -C /path/to/infra terraform plan vpc -s prod

# Just override project root (stay in current directory)
atmos --base-path=/custom/location terraform plan vpc -s prod

# Both: change directory AND override project root
atmos -C /infra --base-path=./custom terraform plan vpc -s prod
```

## Real-World Examples

### Multi-Repo Infrastructure Management

```bash
# Check production infrastructure
atmos -C ~/projects/prod-infra describe affected

# Compare with staging
atmos -C ~/projects/staging-infra describe affected

# All without leaving your current directory
```

### Development and Testing

```bash
# Test local changes against test environment
./build/atmos -C ~/infra terraform plan vpc -s test

# Once satisfied, use installed version for production
atmos -C ~/infra terraform apply vpc -s prod
```

### Automated Scripts

```bash
#!/bin/bash
# Script can stay in your tools repo while operating on infrastructure

INFRA_DIR="/path/to/infrastructure"

echo "Validating stacks..."
atmos -C "$INFRA_DIR" validate stacks

echo "Checking for drift..."
atmos -C "$INFRA_DIR" terraform plan vpc -s prod --detailed-exitcode

echo "Generating documentation..."
atmos -C "$INFRA_DIR" docs generate
```

## Error Handling

Atmos provides clear error messages when things go wrong:

- **Directory doesn't exist**: Clear message with the path that was attempted
- **Path is a file, not a directory**: Explicit error explaining the issue
- **Permission denied**: OS-level error with context
- **Invalid path**: Path resolution errors are caught and reported

## Platform Compatibility

The `--chdir` flag works consistently across:
- **Linux** - All distributions
- **macOS** - All versions
- **Windows** - Native support

Both absolute and relative paths work as expected on all platforms, with proper path separator handling.

## Technical Details

For those interested in the implementation:

- **Execution order**: `--chdir` → config loading → `--base-path` resolution → command execution
- **Path resolution**: Relative paths are resolved from the current working directory
- **Symlinks**: Symlinks to directories work correctly
- **Environment variable**: `ATMOS_CHDIR` follows the same precedence rules as other Atmos env vars
- **Flag precedence**: CLI flag > environment variable > current directory

## Getting Started

The `--chdir` flag is available starting in Atmos vX.X.X. To start using it:

```bash
# Upgrade to the latest version
brew upgrade atmos  # macOS
# or download from GitHub releases

# Start using the flag immediately
atmos --chdir=/your/infra describe stacks

# Or set it as an environment variable for your session
export ATMOS_CHDIR=/your/infra
```

## Conclusion

The `--chdir` flag is a small addition that makes a big difference in daily workflows. It removes friction from multi-repo operations, development workflows, and automation scripts.

We designed it to feel natural if you've used similar flags in other tools (`git -C`, `make -C`, `tar -C`), while integrating seamlessly with Atmos's existing flag system.

Try it out and let us know what you think! We'd love to hear how you're using it in your workflows.

## Resources

- [Global Flags Documentation](/cli/global-flags)
- [Atmos CLI Reference](/cli/commands)
- [GitHub Repository](https://github.com/cloudposse/atmos)

---

*Have feedback or questions? Join our [Slack community](https://slack.cloudposse.com/) or [open an issue on GitHub](https://github.com/cloudposse/atmos/issues).*
