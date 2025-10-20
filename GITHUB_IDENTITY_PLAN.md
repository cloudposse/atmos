# GitHub Identity Integration Plan

## Overview
Add GitHub identity support to Atmos, enabling authentication for both GitHub Users (via Device Flow) and GitHub Apps. This integrates with the existing auth system and allows GitHub tokens to be injected into environments.

---

## 1. Configuration Schema Design

### 1.1 GitHub User Identity (Device Flow)

**Provider Configuration:**
```yaml
auth:
  providers:
    github-user:
      kind: github/user
      client_id: "Iv1.abc123def456"  # GitHub App Client ID for Device Flow
      scopes:  # Optional, defaults to minimal read access
        - repo
        - read:org
        - workflow
      keychain_service: "atmos-github"  # Optional, OS keychain service name
      token_lifetime: 8h  # Optional, default 8h
      default: false
```

**Identity Configuration:**
```yaml
auth:
  identities:
    my-github-user:
      kind: github/user
      via:
        provider: github-user
      env:
        - key: GITHUB_TOKEN
          value: "{{ .Token }}"
        - key: GH_TOKEN
          value: "{{ .Token }}"
```

**Available OAuth Scopes for GitHub Users:**
- `repo` - Full control of private repositories
- `public_repo` - Access to public repositories only
- `read:org` - Read organization membership
- `write:org` - Full control of orgs and teams
- `admin:org` - Full org management
- `gist` - Write access to gists
- `notifications` - Access notifications
- `user` - Update user profile
- `user:email` - Access user email addresses
- `user:follow` - Follow/unfollow users
- `delete_repo` - Delete repositories
- `write:discussion` - Read/write discussions
- `read:discussion` - Read discussions
- `admin:enterprise` - Manage enterprise (GitHub Enterprise only)
- `workflow` - Update GitHub Actions workflow files
- `write:packages` - Upload packages
- `read:packages` - Download packages
- `delete:packages` - Delete packages
- `admin:gpg_key` - Full control of GPG keys
- `write:gpg_key` - Write GPG keys
- `read:gpg_key` - Read GPG keys
- `codespace` - Full control of codespaces
- `project` - Full control of projects

### 1.2 GitHub App Identity (Installation Tokens)

**Provider Configuration:**
```yaml
auth:
  providers:
    github-app:
      kind: github/app
      app_id: "123456"  # GitHub App ID
      installation_id: "789012"  # Installation ID (or can be auto-discovered)
      private_key_path: "/path/to/private-key.pem"  # Path to GitHub App private key
      # OR
      private_key_env: "GITHUB_APP_PRIVATE_KEY"  # Env var containing private key
      # OR
      private_key_store: "aws-ssm:/prod/github/app/key"  # Store reference

      permissions:  # Optional, defaults to all permissions granted to app
        contents: write
        issues: write
        pull_requests: write
        metadata: read

      repositories:  # Optional, limit to specific repos (max 500)
        - "cloudposse/atmos"
        - "cloudposse/terraform-aws-*"  # Glob patterns supported

      token_lifetime: 1h  # Optional, default 1h (GitHub enforced max)
      default: false
```

**Identity Configuration:**
```yaml
auth:
  identities:
    atmos-bot:
      kind: github/app
      via:
        provider: github-app
      env:
        - key: GITHUB_TOKEN
          value: "{{ .Token }}"
        - key: GH_TOKEN
          value: "{{ .Token }}"
        - key: GITHUB_APP_ID
          value: "{{ .AppID }}"
        - key: GITHUB_INSTALLATION_ID
          value: "{{ .InstallationID }}"
```

**Available Permissions for GitHub Apps:**
GitHub Apps use granular permissions instead of OAuth scopes:
- `actions` - read, write
- `administration` - read, write
- `checks` - read, write
- `contents` - read, write
- `deployments` - read, write
- `environments` - read, write
- `issues` - read, write
- `metadata` - read (always granted)
- `packages` - read, write
- `pages` - read, write
- `pull_requests` - read, write
- `repository_hooks` - read, write
- `repository_projects` - read, write, admin
- `secret_scanning_alerts` - read, write
- `secrets` - read, write
- `security_events` - read, write
- `single_file` - read, write
- `statuses` - read, write
- `vulnerability_alerts` - read, write
- `workflows` - write
- `members` - read, write (org-level)
- `organization_administration` - read, write (org-level)
- `organization_hooks` - read, write (org-level)
- `organization_plan` - read (org-level)
- `organization_projects` - read, write, admin (org-level)
- `organization_secrets` - read, write (org-level)
- `organization_self_hosted_runners` - read, write (org-level)
- `organization_user_blocking` - read, write (org-level)
- `team_discussions` - read, write (org-level)

---

## 2. Implementation Architecture

### 2.1 Store Interface Implementation

Both GitHub User and GitHub App stores will implement the existing `Store` interface:

```go
type Store interface {
    Set(stack string, component string, key string, value any) error
    Get(stack string, component string, key string) (any, error)
    GetKey(key string) (any, error)
}
```

**File Structure:**
```
pkg/store/
├── github_user_store.go          # GitHub User authentication (Device Flow)
├── github_user_store_test.go     # Tests with mocks
├── github_app_store.go            # GitHub App authentication (JWT + Installation tokens)
├── github_app_store_test.go      # Tests with mocks
└── registry.go                    # Update to register new store types
```

### 2.2 Authentication Flow Integration

**For `atmos auth exec`:**
```bash
# GitHub User identity
$ atmos auth exec -i my-github-user -- gh api user

# GitHub App identity
$ atmos auth exec -i atmos-bot -- terraform apply
```

**Environment variables injected:**
```bash
export GITHUB_TOKEN='ghs_...'
export GH_TOKEN='ghs_...'
export GITHUB_APP_ID='123456'              # App only
export GITHUB_INSTALLATION_ID='789012'     # App only
```

**For `atmos auth env`:**
```bash
# Export as bash
$ eval "$(atmos auth env -i my-github-user)"

# Export as dotenv
$ atmos auth env -i my-github-user -f dotenv > .env

# Export as JSON
$ atmos auth env -i my-github-user -f json | jq
```

### 2.3 Token Lifecycle Management

**GitHub User Tokens:**
- Default lifetime: 8 hours
- Stored in OS keychain (macOS Keychain, Windows Credential Manager, Linux Secret Service)
- Automatic refresh on expiration via Device Flow re-authentication
- User prompted to re-authenticate when token expires

**GitHub App Tokens:**
- Fixed lifetime: 1 hour (GitHub enforced)
- Generated on-demand using JWT signed with private key
- Cached for duration of command execution
- Automatic regeneration when expired

### 2.4 Security Considerations

1. **Private Key Storage:**
   - Support reading from file, environment variable, or existing store
   - Never log or expose private keys
   - Validate PEM format before use

2. **Token Storage:**
   - OS keychain for user tokens (encrypted at rest)
   - In-memory only for app tokens (short-lived)
   - Never write tokens to disk

3. **Scope Validation:**
   - Validate scopes against GitHub's allowed list
   - Warn if requesting scopes beyond app's granted permissions
   - Document security implications of each scope

---

## 3. Usage Examples

### 3.1 Complete Configuration Example

```yaml
# atmos.yaml
auth:
  providers:
    # GitHub User authentication via Device Flow
    github-user-prod:
      kind: github/user
      client_id: "Iv1.abc123def456"
      scopes:
        - repo
        - workflow
        - read:org
      keychain_service: "atmos-github-prod"

    # GitHub App authentication
    terraform-automation-app:
      kind: github/app
      app_id: "123456"
      installation_id: "789012"
      private_key_store: "aws-ssm:/prod/github/terraform-app/key"
      permissions:
        contents: write
        pull_requests: write
        issues: write
      repositories:
        - "cloudposse/*"

  identities:
    # Developer personal identity
    erik-dev:
      kind: github/user
      default: true
      via:
        provider: github-user-prod
      env:
        - key: GITHUB_TOKEN
          value: "{{ .Token }}"
        - key: GH_TOKEN
          value: "{{ .Token }}"

    # Automation bot identity
    terraform-bot:
      kind: github/app
      via:
        provider: terraform-automation-app
      env:
        - key: GITHUB_TOKEN
          value: "{{ .Token }}"
        - key: GH_TOKEN
          value: "{{ .Token }}"
        - key: GITHUB_APP_ID
          value: "{{ .AppID }}"

# Component configuration can override
components:
  terraform:
    vpc:
      vars:
        github_token: '{{ atmos.Store "github-user" "" "" "token" }}'
```

### 3.2 Command Examples

```bash
# Interactive Device Flow authentication (first time)
$ atmos auth login -i erik-dev
To authenticate with GitHub:
1. Visit: https://github.com/login/device
2. Enter code: ABCD-1234

Waiting for authentication...
✓ Successfully authenticated as erikosterman

# Check authentication status
$ atmos auth whoami -i erik-dev
Identity: erik-dev
Provider: github-user-prod
User: erikosterman
Scopes: repo, workflow, read:org
Token Expires: 2025-10-20T22:00:00Z

# Use GitHub identity with gh CLI
$ atmos auth exec -i erik-dev -- gh repo list cloudposse

# Use GitHub identity with Terraform
$ atmos auth exec -i terraform-bot -- terraform plan

# Export for use in scripts
$ eval "$(atmos auth env -i erik-dev)"
$ gh api user

# Use with GitHub Actions
$ atmos auth exec -i terraform-bot -- gh workflow run deploy.yml
```

### 3.3 GitHub Actions Integration

```yaml
# .github/workflows/deploy.yml
name: Deploy Infrastructure

on:
  push:
    branches: [main]

jobs:
  deploy:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - name: Setup Atmos
        uses: cloudposse/github-action-setup-atmos@v2

      - name: Configure GitHub App
        env:
          GITHUB_APP_PRIVATE_KEY: ${{ secrets.TERRAFORM_APP_KEY }}
        run: |
          atmos auth login -i terraform-bot

      - name: Deploy with Terraform
        run: |
          atmos auth exec -i terraform-bot -- atmos terraform apply vpc -s prod
```

---

## 4. Documentation Updates

### 4.1 New Documentation Pages

1. **`website/docs/cli/commands/auth/providers/github-user.mdx`**
   - How to set up GitHub User authentication
   - Creating a GitHub App for Device Flow
   - Configuring scopes
   - Security best practices

2. **`website/docs/cli/commands/auth/providers/github-app.mdx`**
   - How to set up GitHub App authentication
   - Creating a GitHub App
   - Managing private keys
   - Configuring permissions
   - Repository access patterns

3. **`website/docs/cli/commands/auth/tutorials/github-authentication.mdx`**
   - Complete tutorial for setting up GitHub authentication
   - Use cases: personal development, CI/CD, automation
   - Migration from GITHUB_TOKEN environment variable

### 4.2 Updated Documentation Pages

1. **`website/docs/core-concepts/projects/configuration/stores.mdx`**
   - Add GitHub User and GitHub App store types
   - Configuration examples
   - Use with template functions

2. **`website/docs/cli/commands/auth/usage.mdx`**
   - Update with GitHub examples

---

## 5. Blog Post Outline

**Title:** "Secure GitHub Authentication with Atmos: User Tokens & GitHub Apps"

**Target Audience:** User-facing (feature announcement)

**Tags:** `[feature, github, authentication, security]`

**Structure:**

```markdown
---
slug: github-authentication
title: "Secure GitHub Authentication with Atmos: User Tokens & GitHub Apps"
authors: [atmos]
tags: [feature, github, authentication, security]
---

Atmos now supports native GitHub authentication for both users and GitHub Apps,
enabling secure, short-lived token management for your infrastructure workflows.

<!--truncate-->

## What Changed

Atmos now includes two new authentication providers:

1. **GitHub User Authentication** - Secure Device Flow for personal tokens
2. **GitHub App Authentication** - Installation tokens for automation

Both integrate seamlessly with `atmos auth exec` and `atmos auth env`.

## Why This Matters

- **Security First**: Short-lived tokens (8h for users, 1h for apps) reduce risk
- **No More Token Sprawl**: Centralized token management in OS keychain
- **Granular Permissions**: Configure exact OAuth scopes and app permissions
- **Seamless Integration**: Works with Terraform, gh CLI, and other GitHub tools

## How to Use It

### GitHub User Authentication

```yaml
# atmos.yaml
auth:
  providers:
    github:
      kind: github/user
      client_id: "Iv1.abc123def456"
      scopes: [repo, workflow]

  identities:
    dev:
      kind: github/user
      via:
        provider: github
```

```bash
$ atmos auth exec -i dev -- gh repo list
```

### GitHub App Authentication

```yaml
# atmos.yaml
auth:
  providers:
    bot:
      kind: github/app
      app_id: "123456"
      installation_id: "789012"
      private_key_store: "aws-ssm:/prod/github/key"
      permissions:
        contents: write
```

```bash
$ atmos auth exec -i bot -- terraform apply
```

## Use Cases

1. **Personal Development**: Secure token management for local development
2. **CI/CD Pipelines**: GitHub App tokens for automation
3. **Multi-Repository Operations**: Single identity for all repos
4. **Compliance**: Audit trail with short-lived credentials

## Migration Guide

**Before (manual token management):**
```bash
export GITHUB_TOKEN="ghp_..."
terraform apply
```

**After (Atmos-managed):**
```bash
atmos auth exec -i dev -- terraform apply
```

## Security Benefits

- **Automatic Expiration**: Tokens expire after 1-8 hours
- **OS Keychain Storage**: Encrypted at rest
- **Scope Limitation**: Request only needed permissions
- **No Token Leakage**: Never committed to version control

## Get Involved

- [GitHub User Auth Docs](/cli/commands/auth/providers/github-user)
- [GitHub App Auth Docs](/cli/commands/auth/providers/github-app)
- [Auth Tutorial](/cli/commands/auth/tutorials/github-authentication)
- [Discuss on GitHub](https://github.com/cloudposse/atmos/discussions)
```

---

## 6. Implementation Checklist

### Phase 1: Core Implementation
- [ ] Add `github-user` store implementation (pkg/store/github_user_store.go)
- [ ] Add `github-app` store implementation (pkg/store/github_app_store.go)
- [ ] Register stores in registry (pkg/store/registry.go)
- [ ] Add configuration schema validation
- [ ] Implement token caching and refresh logic

### Phase 2: Integration
- [ ] Integrate with auth manager for identity resolution
- [ ] Add environment variable injection for tokens
- [ ] Support template function access: `{{ atmos.Store "github-user" ... }}`
- [ ] Add `atmos auth login` support for Device Flow

### Phase 3: Testing
- [ ] Unit tests with mocked GitHub API
- [ ] Integration tests with test fixtures
- [ ] End-to-end tests for auth exec/env commands
- [ ] Security tests for token storage/expiration

### Phase 4: Documentation
- [ ] Create provider documentation pages
- [ ] Create tutorial page
- [ ] Update configuration reference
- [ ] Add examples to auth command docs
- [ ] Write blog post

### Phase 5: Release
- [ ] Update CHANGELOG
- [ ] Tag version (minor release)
- [ ] Publish blog post
- [ ] Announce in community channels

---

## 7. Open Questions

1. **Installation ID Discovery:** Should we support auto-discovering installation IDs from app ID?
2. **Multi-Installation Support:** How to handle apps installed in multiple orgs?
3. **Token Refresh UI:** Should we show expiration warnings before tokens expire?
4. **Keychain Backend:** Support for alternative keychain providers (HashiCorp Vault, 1Password)?
5. **Repository Glob Patterns:** Implement glob matching for repository filters?

---

## 8. Dependencies

**New Go Dependencies:**
- `github.com/suzuki-shunsuke/ghtkn` - Device Flow authentication
- `github.com/golang-jwt/jwt/v5` - JWT signing for GitHub Apps
- `github.com/google/go-github/v59` - GitHub API client (already present)

**OS Dependencies:**
- macOS: Keychain Access
- Windows: Credential Manager
- Linux: Secret Service (GNOME Keyring, KWallet)
