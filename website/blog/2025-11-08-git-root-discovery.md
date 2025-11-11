---
slug: git-root-discovery
title: "Run Atmos from Any Subdirectory"
authors: [osterman]
tags: [atmos, cli, productivity, developer-experience]
date: 2025-11-08
---

Atmos now automatically discovers your repository root and runs from there, just like Git. No more `cd`-ing back to the root directory.

<!--truncate-->

## The Git-Like Behavior

If you've used Git, you know you can run `git status` from any subdirectory in your repository, and Git automatically finds the repository root. Atmos now works the same way.

**Before:**
```bash
cd components/terraform/vpc
atmos terraform plan vpc -s prod
# Error: Could not find atmos.yaml
cd ../../..
atmos terraform plan vpc -s prod  # Now it works
```

**Now:**
```bash
cd components/terraform/vpc
atmos terraform plan vpc -s prod  # Just works
```

## How It Works

When you run Atmos from a subdirectory:

1. Atmos detects you're in a Git repository
2. It finds the repository root (where `.git` lives)
3. It uses that as the base path for all operations
4. Your `atmos.yaml` at the repository root is found automatically

Just like Git, Atmos walks up the directory tree to find the repository root.

## Local Configuration Always Wins

If you have an `atmos.yaml` in your current directory, Atmos uses that instead. This ensures local overrides work as expected:

```bash
cd experiments/
echo "base_path: ." > atmos.yaml
atmos terraform plan  # Uses ./atmos.yaml, not repository root
```

Atmos respects these local configuration indicators:
- `atmos.yaml` - Main config file
- `.atmos.yaml` - Hidden config file
- `.atmos/` - Config directory
- `.atmos.d/` - Default imports directory
- `atmos.d/` - Alternate imports directory

If any of these exist in your current directory, they take precedence over git root discovery.

## Disabling the Feature

For testing or if you prefer the old behavior, set an environment variable:

```bash
export ATMOS_GIT_ROOT_BASEPATH=false
atmos terraform plan  # Uses current directory as base path
```

This is automatically set for Atmos's internal test suite to prevent test pollution.

## Why This Matters

This small change eliminates a common frustration: having to remember where you are in your repository structure. Now you can:

- Navigate to component directories to review code
- Run Atmos commands without changing directories
- Write simpler automation scripts
- Work more naturally within your repository

Just like Git changed your mental model from "I must be at the root" to "I can work anywhere," Atmos now does the same for infrastructure orchestration.
