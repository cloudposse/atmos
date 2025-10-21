# GitHub OIDC Authentication Test Fixture

This test fixture demonstrates how to configure and use GitHub OIDC authentication with Atmos in GitHub Actions workflows.

## Overview

GitHub Actions provides OIDC (OpenID Connect) tokens that can be used to authenticate to external services without storing long-lived credentials. Atmos supports GitHub OIDC through the `github/oidc` provider.

## Configuration

The `atmos.yaml` in this directory configures:

1. **Provider**: `github-oidc` - Authenticates using GitHub Actions OIDC token
2. **Identity**: `github-actions` - A `github/token` identity that uses the OIDC provider

## How GitHub OIDC Works in Actions

When a GitHub Actions workflow runs with `permissions.id-token: write`, GitHub automatically provides:

- `ACTIONS_ID_TOKEN_REQUEST_URL` - URL to request OIDC tokens
- `ACTIONS_ID_TOKEN_REQUEST_TOKEN` - Bearer token to authenticate the request

Atmos detects these environment variables and uses them to obtain a GitHub token.

## Usage in GitHub Actions

### Step 1: Configure Workflow Permissions

```yaml
name: Deploy with Atmos
on: push

permissions:
  id-token: write  # Required for OIDC
  contents: read

jobs:
  deploy:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - name: Setup Atmos
        uses: cloudposse/github-action-setup-atmos@v2

      - name: Authenticate with GitHub OIDC
        working-directory: path/to/your/atmos/root
        run: |
          atmos auth login --identity github-actions
```

### Step 2: Use Authenticated Environment

```yaml
      - name: Export GitHub token to environment
        run: |
          # Export GITHUB_TOKEN and GH_TOKEN environment variables
          eval $(atmos auth env --identity github-actions)

      - name: Use GitHub token
        run: |
          # Now GITHUB_TOKEN is available
          gh api user
          # Or use with Terraform GitHub provider, etc.
```

## Local Testing (Simulation)

Since OIDC tokens are only available in GitHub Actions, you cannot test real OIDC authentication locally. However, you can:

### Test Configuration Validation

```bash
# Validate auth configuration
cd tests/test-cases/auth-github-oidc
atmos auth validate

# View auth configuration
atmos describe config
```

### Test with GitHub User Authentication (Alternative)

For local development, use `github/user` provider instead:

```yaml
auth:
  providers:
    github-user:
      kind: github/user
  identities:
    dev:
      kind: github/token
      via:
        provider: github-user
```

Then authenticate locally:
```bash
atmos auth login --identity dev
```

## Testing in CI

### Automated Testing (Safe for Fork PRs)

The repository includes tests that validate:
- Configuration parsing ✅
- Provider creation ✅
- Identity resolution ✅

These tests do NOT require real OIDC tokens and run on all PRs.

### Integration Testing (Trusted PRs Only)

Real OIDC authentication is tested in `.github/workflows/test-github-oidc.yml` for:
- Pushes to `main` and `release/*` branches
- PRs from repository collaborators (not forks)

**Why not fork PRs?** GitHub prevents fork PRs from accessing OIDC tokens for security reasons.

## Token Claims

The GitHub OIDC token includes claims such as:

```json
{
  "iss": "https://token.actions.githubusercontent.com",
  "sub": "repo:cloudposse/atmos:ref:refs/heads/main",
  "aud": "https://github.com/cloudposse",
  "repository": "cloudposse/atmos",
  "repository_owner": "cloudposse",
  "workflow": "Deploy",
  "actor": "username",
  "ref": "refs/heads/main"
}
```

External services can validate these claims to ensure requests come from authorized workflows.

## Example Use Cases

1. **Terraform GitHub Provider**: Authenticate to GitHub API for managing repositories
2. **Private Module Access**: Download Terraform modules from private GitHub repositories
3. **GitHub API Calls**: Use `gh` CLI or direct API calls in workflows
4. **Package Publishing**: Publish packages to GitHub Packages
5. **Release Automation**: Create releases and tags via GitHub API

## Troubleshooting

### Error: "OIDC token request URL not available"

This means:
- You're not running in GitHub Actions, OR
- Workflow doesn't have `permissions.id-token: write`

**Solution**: Add permissions to your workflow or use `github/user` provider locally.

### Error: "Failed to obtain OIDC token"

Check that:
1. Workflow has `permissions.id-token: write`
2. Running in a trusted context (not a fork PR)
3. Repository has OIDC enabled (should be by default)

### Testing from Fork PRs

Fork PRs cannot access OIDC tokens. Instead:
1. Unit tests validate the code works
2. After merging, integration tests run with real OIDC
3. Maintainers can trigger workflows manually if needed

## Reference

- [GitHub OIDC Documentation](https://docs.github.com/en/actions/deployment/security-hardening-your-deployments/about-security-hardening-with-openid-connect)
- [Atmos Auth Documentation](https://atmos.tools/cli/commands/auth/)
- [GitHub Token Permissions](https://docs.github.com/en/actions/security-guides/automatic-token-authentication#permissions-for-the-github_token)
