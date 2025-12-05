# Demo: Using `atmos env` with GitHub Provider

This example demonstrates how to use `atmos env` to export environment variables
for Terraform providers that authenticate via environment variables.

## Overview

The GitHub Terraform provider authenticates using the `GITHUB_TOKEN` environment
variable. This example shows how to:

1. Configure `GITHUB_TOKEN` in `atmos.yaml` using the `!exec` YAML function
2. Export it using `atmos env`
3. Use it with Terraform to fetch repository data

## Prerequisites

1. **GitHub CLI** - Install from https://cli.github.com/
2. **Authenticate with GitHub**:
   ```bash
   gh auth login
   ```
3. **Terraform** >= 1.0
4. **Atmos CLI**

## Usage

### 1. Export environment variables

```bash
# View what will be exported
atmos env

# Export to your shell
eval $(atmos env)
```

### 2. Run Terraform via Atmos

```bash
# Plan
atmos terraform plan github-repo -s demo

# Apply
atmos terraform apply github-repo -s demo
```

### 3. View outputs

```bash
atmos terraform output github-repo -s demo
```

Example output:
```
default_branch = "main"
description = "Universal Tool for DevOps and Cloud Automation"
html_url = "https://github.com/cloudposse/atmos"
repository = "cloudposse/atmos"
```

## How It Works

The `atmos.yaml` configures `GITHUB_TOKEN` in the global `env` section:

```yaml
env:
  GITHUB_TOKEN: !exec gh auth token
```

When you run `eval $(atmos env)`, it:
1. Executes `gh auth token` to get your GitHub token
2. Exports it as `GITHUB_TOKEN`
3. Makes it available to all subsequent commands

The GitHub provider in Terraform automatically uses this environment variable
for authentication.

## GitHub Actions

For CI/CD, use `--format=github` to write directly to `$GITHUB_ENV`:

```yaml
- name: Export Atmos environment
  run: atmos env --format=github

- name: Run Terraform
  run: atmos terraform apply github-repo -s demo --auto-approve
```
