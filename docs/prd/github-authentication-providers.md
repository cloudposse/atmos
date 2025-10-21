# Product Requirements Document: GitHub Authentication Providers

## Executive Summary

Atmos GitHub Authentication Providers enable secure, short-lived token management for GitHub interactions through two distinct authentication methods: **GitHub User Authentication** via OAuth Device Flow and **GitHub App Authentication** via installation tokens. This feature integrates with Atmos's existing auth system to provide centralized, secure GitHub credential management for infrastructure workflows, CI/CD pipelines, and developer tooling.

## 1. Problem Statement

### Current Challenges

- **Long-lived Personal Access Tokens**: Developers use static PATs that pose security risks if leaked
- **Token Sprawl**: GitHub tokens stored in multiple locations (env vars, config files, CI secrets)
- **No Centralized Management**: No unified way to manage GitHub authentication across tools
- **Manual Expiration**: Tokens don't automatically expire, requiring manual rotation
- **Overly Broad Permissions**: PATs often granted more permissions than needed
- **Poor Audit Trail**: Difficult to track who accessed what and when
- **CI/CD Complexity**: Managing GitHub tokens in automation requires separate credential systems

### User Personas

1. **Platform Engineers**: Need secure GitHub access for infrastructure automation (Terraform, Helmfile)
2. **Developers**: Want seamless GitHub authentication for local development workflows
3. **DevOps Teams**: Require automated GitHub access for CI/CD pipelines
4. **Security Teams**: Need audit trails and short-lived credentials for compliance

### Success Metrics

- **Reduced Token Lifetime**: 90% reduction in average token lifetime (from weeks to hours)
- **Elimination of Static Tokens**: 100% removal of hardcoded GitHub tokens from configurations
- **Developer Adoption**: 80%+ of team using Atmos-managed GitHub authentication within 3 months
- **Security Incidents**: Zero GitHub token leaks from Atmos-managed credentials
- **Time to Authenticate**: <30 seconds for initial setup, <5 seconds for subsequent use

## 2. Core Requirements

### 2.1 Functional Requirements

#### FR-001: GitHub User Authentication (Device Flow)

- **Description**: Support OAuth Device Flow for secure user token generation
- **Acceptance Criteria**:
  - Implement OAuth 2.0 Device Authorization Grant flow
  - Generate 8-hour short-lived user access tokens
  - Store tokens securely in OS keychain (macOS Keychain, Windows Credential Manager, Linux Secret Service)
  - Support configurable OAuth scopes (repo, workflow, read:org, etc.)
  - Automatic token refresh when expired
  - Interactive browser-based authentication flow
- **Priority**: P0 (Must Have)
- **Dependencies**: ghtkn library, OS keychain APIs

#### FR-002: GitHub App Authentication (Installation Tokens)

- **Description**: Support GitHub App installation token generation
- **Acceptance Criteria**:
  - Generate JWT tokens signed with GitHub App private key
  - Create 1-hour installation access tokens
  - Support configurable permissions (contents, issues, pull_requests, etc.)
  - Support repository filtering (single repos, org wildcards, glob patterns)
  - Automatic token regeneration on expiration
  - Support multiple private key sources (file, env var, store)
  - Validate private key format (PEM) before use
- **Priority**: P0 (Must Have)
- **Dependencies**: golang-jwt library, go-github library

#### FR-003: Configurable Permissions & Scopes

- **Description**: Fine-grained control over GitHub permissions
- **Acceptance Criteria**:
  - Support all GitHub OAuth scopes (25+ scopes)
  - Support all GitHub App permissions (30+ permissions with read/write levels)
  - Validate requested permissions against GitHub API
  - Warn if requesting permissions beyond granted permissions
  - Default to minimal required permissions if not specified
  - Document security implications of each permission
- **Priority**: P0 (Must Have)

#### FR-004: Environment Variable Injection

- **Description**: Inject GitHub tokens as environment variables
- **Acceptance Criteria**:
  - Set `GITHUB_TOKEN` environment variable with valid token
  - Set `GH_TOKEN` environment variable (gh CLI compatibility)
  - For apps, set `GITHUB_APP_ID` and `GITHUB_INSTALLATION_ID`
  - Work with `atmos auth exec` command
  - Work with `atmos auth env` command (bash, json, dotenv formats)
  - Tokens injected only in command scope (not persistent)
- **Priority**: P0 (Must Have)

#### FR-005: Store Interface Integration

- **Description**: Implement existing Store interface for GitHub tokens
- **Acceptance Criteria**:
  - Implement `Store` interface (Get, Set, GetKey methods)
  - Support `{{ atmos.Store "github-user" ... }}` template function
  - Register as `github-user` and `github-app` store types
  - Integration with existing store registry
  - Support use in component configurations
- **Priority**: P1 (Should Have)

#### FR-006: Token Lifecycle Management

- **Description**: Automatic token expiration and refresh
- **Acceptance Criteria**:
  - User tokens expire after 8 hours (configurable)
  - App tokens expire after 1 hour (GitHub enforced)
  - Automatic refresh when token expires
  - Cache tokens for command duration to avoid repeated API calls
  - Clear expired tokens from keychain
  - Show expiration warnings before token expires (5min)
- **Priority**: P0 (Must Have)

#### FR-007: CLI Commands

- **Description**: User-friendly CLI interface for GitHub authentication
- **Acceptance Criteria**:
  - `atmos auth login -i <identity>` initiates Device Flow
  - `atmos auth whoami -i <identity>` shows GitHub user/app info
  - `atmos auth validate -i <identity>` validates configuration
  - `atmos auth exec -i <identity> -- <command>` runs commands with token
  - `atmos auth env -i <identity>` exports token as env vars
  - Interactive prompts with clear instructions
  - Error messages with actionable guidance
- **Priority**: P0 (Must Have)

#### FR-008: Configuration Schema

- **Description**: Extend auth configuration schema for GitHub providers
- **Acceptance Criteria**:
  - Add `github-user` provider kind to schema
  - Add `github-app` provider kind to schema
  - JSON Schema validation for all GitHub-specific fields
  - Support for templated values in configuration
  - Validation of OAuth scopes and app permissions
  - Clear error messages for invalid configurations
- **Priority**: P0 (Must Have)

#### FR-009: Security & Compliance

- **Description**: Secure credential handling and storage
- **Acceptance Criteria**:
  - Never log GitHub tokens or private keys
  - Store user tokens encrypted in OS keychain
  - Keep app tokens in-memory only (never disk)
  - Validate private key permissions (readable only by owner)
  - Support reading private keys from secure stores (AWS SSM, etc.)
  - Audit log for authentication events
  - Redact tokens from error messages
- **Priority**: P0 (Must Have)

### 2.2 Non-Functional Requirements

#### NFR-001: Performance

- Initial authentication: <10 seconds (excluding user interaction)
- Token retrieval from cache: <100ms
- Token refresh: <3 seconds
- Environment variable injection overhead: <50ms

#### NFR-002: Reliability

- 99.9% success rate for token generation (excluding user errors)
- Automatic retry with exponential backoff for API failures
- Graceful degradation if keychain unavailable
- Clear error messages for all failure modes

#### NFR-003: Security

- Zero plaintext token storage in configuration files
- Encrypted token storage via OS keychain
- Automatic token expiration enforcement
- Private key validation before use
- Rate limit handling for GitHub API

#### NFR-004: Usability

- Interactive Device Flow with clear instructions
- Progress indicators for token generation
- Helpful error messages with resolution steps
- Documentation with copy-paste examples
- IDE autocomplete support for configuration

#### NFR-005: Compatibility

- Support macOS 10.14+, Windows 10+, Linux (systemd)
- Compatible with existing Atmos auth system
- No breaking changes to existing configurations
- Works with all Atmos commands (terraform, helmfile, etc.)
- Compatible with GitHub Enterprise Server

## 3. Architecture

### 3.1 Component Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                    Atmos CLI                                 │
├─────────────────────────────────────────────────────────────┤
│                                                              │
│  ┌────────────────────────────────────────────┐             │
│  │         Auth Manager                       │             │
│  │  (pkg/auth/manager/)                       │             │
│  └────────────────────────────────────────────┘             │
│           │                        │                         │
│           │                        │                         │
│           ▼                        ▼                         │
│  ┌─────────────────┐    ┌──────────────────────┐           │
│  │  GitHub User    │    │  GitHub App          │           │
│  │  Provider       │    │  Provider            │           │
│  │                 │    │                      │           │
│  │  - Device Flow  │    │  - JWT Signing       │           │
│  │  - OAuth        │    │  - Installation      │           │
│  │  - Keychain     │    │    Tokens            │           │
│  │  - 8h tokens    │    │  - 1h tokens         │           │
│  └─────────────────┘    └──────────────────────┘           │
│           │                        │                         │
│           │                        │                         │
│           ▼                        ▼                         │
│  ┌────────────────────────────────────────────┐             │
│  │         GitHub API Client                  │             │
│  │  (github.com/google/go-github/v59)         │             │
│  └────────────────────────────────────────────┘             │
│                                                              │
│  ┌────────────────────────────────────────────┐             │
│  │         OS Keychain                        │             │
│  │  - macOS Keychain                          │             │
│  │  - Windows Credential Manager              │             │
│  │  - Linux Secret Service                    │             │
│  └────────────────────────────────────────────┘             │
│                                                              │
└─────────────────────────────────────────────────────────────┘
```

### 3.2 Authentication Flow

#### GitHub User Authentication Flow

```
User                  Atmos CLI              GitHub API         OS Keychain
  │                       │                       │                  │
  │ atmos auth login      │                       │                  │
  ├──────────────────────>│                       │                  │
  │                       │                       │                  │
  │                       │ Check cached token    │                  │
  │                       ├──────────────────────────────────────────>│
  │                       │                       │                  │
  │                       │<── Token expired/none ────────────────────┤
  │                       │                       │                  │
  │                       │ Start Device Flow     │                  │
  │                       ├──────────────────────>│                  │
  │                       │                       │                  │
  │                       │<── device_code ───────┤                  │
  │                       │    user_code          │                  │
  │                       │    verification_uri   │                  │
  │                       │                       │                  │
  │<── Show instructions ─┤                       │                  │
  │    Visit URL          │                       │                  │
  │    Enter code         │                       │                  │
  │                       │                       │                  │
  │ [Browser Auth]        │                       │                  │
  │───────────────────────────────────────────────>│                  │
  │                       │                       │                  │
  │                       │ Poll for token        │                  │
  │                       ├──────────────────────>│                  │
  │                       │                       │                  │
  │                       │<── access_token ──────┤                  │
  │                       │                       │                  │
  │                       │ Store token           │                  │
  │                       ├──────────────────────────────────────────>│
  │                       │                       │                  │
  │<── Success ───────────┤                       │                  │
  │                       │                       │                  │
```

#### GitHub App Authentication Flow

```
User                  Atmos CLI              GitHub API         Store/File
  │                       │                       │                  │
  │ atmos auth exec       │                       │                  │
  ├──────────────────────>│                       │                  │
  │                       │                       │                  │
  │                       │ Load private key      │                  │
  │                       ├──────────────────────────────────────────>│
  │                       │                       │                  │
  │                       │<── private key PEM ────────────────────────┤
  │                       │                       │                  │
  │                       │ Generate JWT          │                  │
  │                       │ (signed with key)     │                  │
  │                       │                       │                  │
  │                       │ Request installation  │                  │
  │                       │ token (with JWT)      │                  │
  │                       ├──────────────────────>│                  │
  │                       │                       │                  │
  │                       │<── installation token ┤                  │
  │                       │     (1h validity)     │                  │
  │                       │                       │                  │
  │                       │ Cache token (memory)  │                  │
  │                       │                       │                  │
  │                       │ Execute command with  │                  │
  │                       │ GITHUB_TOKEN env var  │                  │
  │                       │                       │                  │
  │<── Command output ────┤                       │                  │
  │                       │                       │                  │
```

### 3.3 File Organization

```
pkg/
├── store/
│   ├── store.go                        # Store interface
│   ├── github_user_store.go            # GitHub User store implementation
│   ├── github_user_store_test.go       # Tests with mocks
│   ├── github_app_store.go             # GitHub App store implementation
│   ├── github_app_store_test.go        # Tests with mocks
│   └── registry.go                     # Update to register new stores
│
├── github/
│   ├── client.go                       # Existing GitHub client
│   ├── device_flow.go                  # NEW: Device Flow implementation
│   ├── device_flow_test.go             # NEW: Device Flow tests
│   ├── app_auth.go                     # NEW: App authentication
│   ├── app_auth_test.go                # NEW: App auth tests
│   └── keychain.go                     # NEW: OS keychain integration
│
└── schema/
    └── schema_auth.go                  # Update with GitHub provider types

cmd/
└── auth/
    ├── auth.go                         # Existing auth command
    ├── auth_login.go                   # Update for GitHub Device Flow
    └── auth_whoami.go                  # Update to show GitHub info

docs/prd/
└── github-authentication-providers.md  # This document

website/docs/
└── cli/commands/auth/
    ├── providers/
    │   ├── github-user.mdx             # NEW: GitHub User docs
    │   └── github-app.mdx              # NEW: GitHub App docs
    └── tutorials/
        └── github-authentication.mdx   # NEW: Complete tutorial
```

## 4. Configuration Reference

### 4.1 GitHub User Provider

```yaml
auth:
  providers:
    github-personal:
      kind: github/user

      # Optional: GitHub OAuth App Client ID for Device Flow
      # If not specified, uses official Atmos OAuth App (zero-config)
      client_id: "Iv1.abc123def456"

      # Optional: OAuth scopes (defaults to minimal read access)
      scopes:
        - repo                  # Full control of private repos
        - workflow              # Update GitHub Actions workflows
        - read:org              # Read organization membership
        - admin:public_key      # Manage SSH keys

      # Optional: OS keychain service name (default: "atmos-github")
      keychain_service: "atmos-github-prod"

      # Optional: Token lifetime (default: 8h, max: 24h)
      token_lifetime: 8h

      # Optional: Make this the default provider
      default: false

  identities:
    dev:
      kind: github/user
      via:
        provider: github-personal

      # Optional: Environment variables to export
      env:
        - key: GITHUB_TOKEN
          value: "{{ .Token }}"
        - key: GH_TOKEN
          value: "{{ .Token }}"
        - key: GITHUB_USER
          value: "{{ .Username }}"
```

### 4.2 GitHub App Provider

```yaml
auth:
  providers:
    terraform-bot:
      kind: github/app

      # Required: GitHub App ID
      app_id: "123456"

      # Required: Installation ID (or auto-discover from app_id)
      installation_id: "789012"

      # Required: Private key (one of these methods)
      private_key_path: "/path/to/private-key.pem"
      # OR
      private_key_env: "GITHUB_APP_PRIVATE_KEY"
      # OR
      private_key_store: "aws-ssm:/prod/github/terraform-app/key"

      # Optional: Permissions (defaults to all granted to app)
      permissions:
        contents: write           # Read/write repository contents
        issues: write             # Create/edit issues
        pull_requests: write      # Create/edit PRs
        metadata: read            # Always granted
        workflows: write          # Update workflow files

      # Optional: Repository access (glob patterns supported)
      repositories:
        - "cloudposse/atmos"
        - "cloudposse/terraform-*"
        - "example-org/*"

      # Optional: Token lifetime (default: 1h, GitHub max: 1h)
      token_lifetime: 1h

      # Optional: Make this the default provider
      default: false

  identities:
    bot:
      kind: github/app
      via:
        provider: terraform-bot

      # Optional: Environment variables to export
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

### 4.3 Complete Example

```yaml
# atmos.yaml
auth:
  providers:
    # Developer authentication (zero-config)
    github-dev:
      kind: github/user
      # client_id is optional - uses official Atmos OAuth App by default
      scopes: [repo, workflow, read:org]
      keychain_service: "atmos-github-dev"

    # CI/CD automation
    github-ci:
      kind: github/app
      app_id: "456789"
      installation_id: "987654"
      private_key_env: "GITHUB_CI_APP_KEY"
      permissions:
        contents: write
        pull_requests: write
        issues: write

  identities:
    # Default developer identity
    dev:
      kind: github/user
      default: true
      via:
        provider: github-dev
      env:
        - key: GITHUB_TOKEN
          value: "{{ .Token }}"

    # Terraform automation identity
    terraform-bot:
      kind: github/app
      via:
        provider: github-ci
      env:
        - key: GITHUB_TOKEN
          value: "{{ .Token }}"
        - key: TF_VAR_github_token
          value: "{{ .Token }}"

# Component-specific override
components:
  terraform:
    vpc:
      settings:
        auth:
          identities:
            custom-bot:
              kind: github/app
              via:
                provider: github-ci
              principal:
                installation_id: "123456"  # Different installation
```

## 5. OAuth Scopes Reference

### User Token Scopes

| Scope | Description | Use Case |
|-------|-------------|----------|
| `(no scope)` | Read-only public data | Public repo access only |
| `repo` | Full control of private repos | Terraform provider, gh CLI |
| `repo:status` | Access commit status | CI/CD status checks |
| `repo_deployment` | Access deployment status | Deployment automation |
| `public_repo` | Access public repos only | Public repo operations |
| `repo:invite` | Repository invitations | Team management |
| `security_events` | Read/write security events | Security scanning |
| `admin:repo_hook` | Full control of repo webhooks | Webhook management |
| `write:repo_hook` | Write repo hooks | Webhook creation |
| `read:repo_hook` | Read repo hooks | Webhook inspection |
| `admin:org` | Full control of orgs | Organization management |
| `write:org` | Read/write org access | Team management |
| `read:org` | Read org membership | Org visibility |
| `admin:public_key` | Full control of SSH keys | SSH key management |
| `write:public_key` | Write SSH keys | SSH key creation |
| `read:public_key` | Read SSH keys | SSH key inspection |
| `admin:org_hook` | Full control of org webhooks | Org webhook management |
| `gist` | Create gists | Gist management |
| `notifications` | Access notifications | Notification management |
| `user` | Update user profile | Profile management |
| `user:email` | Access user email | Email verification |
| `user:follow` | Follow/unfollow users | Social features |
| `project` | Full control of projects | Project management |
| `read:project` | Read project access | Project visibility |
| `delete_repo` | Delete repositories | Repo cleanup |
| `write:packages` | Upload packages | Package publishing |
| `read:packages` | Download packages | Package consumption |
| `delete:packages` | Delete packages | Package cleanup |
| `admin:gpg_key` | Full control of GPG keys | GPG key management |
| `write:gpg_key` | Write GPG keys | GPG key creation |
| `read:gpg_key` | Read GPG keys | GPG key inspection |
| `codespace` | Full control of codespaces | Codespace management |
| `workflow` | Update workflow files | GitHub Actions automation |

## 6. GitHub App Permissions Reference

### Repository Permissions

| Permission | Levels | Description |
|------------|--------|-------------|
| `actions` | read, write | Manage GitHub Actions |
| `administration` | read, write | Manage repository settings |
| `checks` | read, write | Manage check runs/suites |
| `contents` | read, write | Manage repository contents |
| `deployments` | read, write | Manage deployments |
| `environments` | read, write | Manage environments |
| `issues` | read, write | Manage issues |
| `metadata` | read | Repository metadata (always granted) |
| `packages` | read, write | Manage packages |
| `pages` | read, write | Manage GitHub Pages |
| `pull_requests` | read, write | Manage pull requests |
| `repository_hooks` | read, write | Manage webhooks |
| `repository_projects` | read, write, admin | Manage projects |
| `secret_scanning_alerts` | read, write | Manage secret scanning |
| `secrets` | read, write | Manage secrets |
| `security_events` | read, write | Manage security events |
| `single_file` | read, write | Access single file |
| `statuses` | read, write | Manage commit statuses |
| `vulnerability_alerts` | read, write | Manage vulnerability alerts |
| `workflows` | write | Update workflow files |

### Organization Permissions

| Permission | Levels | Description |
|------------|--------|-------------|
| `members` | read, write | Manage organization members |
| `organization_administration` | read, write | Manage organization settings |
| `organization_hooks` | read, write | Manage organization webhooks |
| `organization_plan` | read | View organization plan |
| `organization_projects` | read, write, admin | Manage organization projects |
| `organization_secrets` | read, write | Manage organization secrets |
| `organization_self_hosted_runners` | read, write | Manage self-hosted runners |
| `organization_user_blocking` | read, write | Block/unblock users |
| `team_discussions` | read, write | Manage team discussions |

## 7. Use Cases

### 7.1 Personal Development

**Scenario**: Developer needs GitHub access for Terraform operations

```yaml
# atmos.yaml
auth:
  providers:
    github:
      kind: github/user
      # client_id is optional - uses official Atmos OAuth App by default
      scopes: [repo, workflow]

  identities:
    dev:
      kind: github/user
      default: true
      via:
        provider: github
```

```bash
# First time: authenticate
$ atmos auth login
To authenticate with GitHub:
1. Visit: https://github.com/login/device
2. Enter code: ABCD-1234

✓ Successfully authenticated as erikosterman

# Use with Terraform
$ atmos terraform plan vpc -s prod
# Token automatically injected

# Use with gh CLI
$ atmos auth exec -- gh repo list cloudposse
```

### 7.2 CI/CD Automation

**Scenario**: GitHub Actions workflow needs to run Terraform

```yaml
# atmos.yaml
auth:
  providers:
    ci-bot:
      kind: github/app
      app_id: "123456"
      installation_id: "789012"
      private_key_env: "GITHUB_APP_PRIVATE_KEY"
      permissions:
        contents: write
        pull_requests: write

  identities:
    terraform-bot:
      kind: github/app
      via:
        provider: ci-bot
```

```yaml
# .github/workflows/deploy.yml
jobs:
  deploy:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: cloudposse/github-action-setup-atmos@v2

      - name: Deploy
        env:
          GITHUB_APP_PRIVATE_KEY: ${{ secrets.APP_PRIVATE_KEY }}
        run: |
          atmos auth exec -i terraform-bot -- \
            atmos terraform apply vpc -s prod
```

### 7.3 Multi-Repository Operations

**Scenario**: Bulk operations across organization repos

```yaml
# atmos.yaml
auth:
  providers:
    org-admin:
      kind: github/app
      app_id: "456789"
      installation_id: "987654"
      private_key_store: "aws-ssm:/prod/github/admin-key"
      permissions:
        contents: write
        pull_requests: write
      repositories:
        - "cloudposse/*"  # All cloudposse repos

  identities:
    admin:
      kind: github/app
      via:
        provider: org-admin
```

```bash
# Update all repos
$ atmos auth exec -i admin -- gh repo list cloudposse --json name | \
  jq -r '.[].name' | \
  xargs -I {} gh repo edit cloudposse/{} --enable-auto-merge
```

### 7.4 Store Integration

**Scenario**: Use GitHub token in Terraform variables

```yaml
# Component configuration
components:
  terraform:
    github-resources:
      vars:
        # Fetch token from GitHub User store
        github_token: '{{ atmos.Store "github-user" "" "" "token" }}'

        # Or use in templates
        github_org: "cloudposse"

# In Terraform
variable "github_token" {
  type      = string
  sensitive = true
}

provider "github" {
  token = var.github_token
}
```

## 8. Migration Path

### From Environment Variables

**Before:**
```bash
# Manual token management
export GITHUB_TOKEN="ghp_xxxxxxxxxxxxxxxxxxxx"
terraform apply
```

**After:**
```yaml
# atmos.yaml
auth:
  identities:
    dev:
      kind: github/user
      default: true
```

```bash
# Atmos-managed tokens
atmos terraform apply vpc -s prod
```

### From GitHub CLI

**Before:**
```bash
# Use gh auth for everything
gh auth login
gh auth setup-git
terraform apply  # Reads ~/.config/gh/hosts.yml
```

**After:**
```yaml
# atmos.yaml
auth:
  providers:
    github:
      kind: github/user
      # client_id is optional - uses official Atmos OAuth App by default
      scopes: [repo]
```

```bash
# Unified auth system
atmos auth login
atmos terraform apply vpc -s prod
```

## 9. Testing Strategy

### Unit Tests

- Mock GitHub API responses for Device Flow
- Mock keychain operations
- Test JWT signing with test keys
- Validate scope/permission parsing
- Test token expiration logic

### Integration Tests

- End-to-end Device Flow (with test app)
- App token generation (with test app)
- Keychain storage/retrieval
- Environment variable injection
- Store interface compliance

### Security Tests

- Private key validation
- Token redaction in logs
- Keychain encryption verification
- Rate limit handling
- Error message sanitization

## 10. Documentation Plan

### 10.1 Reference Documentation

1. **GitHub User Provider** (`website/docs/cli/commands/auth/providers/github-user.mdx`)
   - Configuration reference
   - OAuth scope guide
   - Security best practices
   - Troubleshooting

2. **GitHub App Provider** (`website/docs/cli/commands/auth/providers/github-app.mdx`)
   - Configuration reference
   - Permission guide
   - Private key management
   - Repository filtering

3. **GitHub Authentication Tutorial** (`website/docs/cli/commands/auth/tutorials/github-authentication.mdx`)
   - Step-by-step setup
   - Use case examples
   - Migration guide
   - FAQ

### 10.2 Blog Post

**Title**: "Secure GitHub Authentication with Atmos: User Tokens & GitHub Apps"

**Content**:
- Problem statement (token sprawl, security risks)
- Solution overview (Device Flow, App tokens)
- Usage examples
- Security benefits
- Migration guide

**Tags**: `[feature, github, authentication, security]`

## 11. Open Questions

1. **Installation ID Discovery**: Should we auto-discover installation IDs from app ID?
   - **Decision**: Phase 2 feature, manual specification for MVP

2. **Multi-Installation Support**: How to handle apps installed in multiple orgs?
   - **Decision**: Support via multiple providers with different installation_ids

3. **Token Refresh UI**: Should we show expiration warnings?
   - **Decision**: Yes, warn at 5 minutes before expiration

4. **Keychain Fallback**: What if keychain unavailable?
   - **Decision**: Graceful degradation to environment variables with warning

5. **Repository Glob Patterns**: Implement glob matching for repository filters?
   - **Decision**: Yes, support `*` and `**` wildcards in repository patterns

6. **GitHub Enterprise Support**: How to support GHES?
   - **Decision**: Add optional `base_url` parameter for custom GitHub instances

## 12. Success Criteria

### Phase 1: MVP (Must Have)
- ✅ GitHub User authentication via Device Flow
- ✅ GitHub App authentication via JWT
- ✅ OS keychain integration
- ✅ Environment variable injection
- ✅ Basic configuration schema
- ✅ CLI commands (login, whoami, exec, env)

### Phase 2: Enhancement (Should Have)
- Store interface implementation
- Auto-discovery of installation IDs
- Repository glob pattern matching
- GitHub Enterprise support
- Advanced permission validation

### Phase 3: Polish (Nice to Have)
- Token expiration warnings
- Interactive permission selector
- Multi-cloud identity chaining (GitHub → AWS)
- Audit logging integration
- Performance optimizations

## 13. Risks & Mitigations

| Risk | Impact | Probability | Mitigation |
|------|--------|-------------|------------|
| GitHub API rate limits | High | Medium | Implement caching, backoff, warn users |
| OS keychain unavailable | Medium | Low | Fallback to env vars with clear warning |
| Private key leakage | Critical | Low | Validate permissions, never log keys |
| Token expiration mid-command | Medium | Medium | Refresh before executing long commands |
| User abandons Device Flow | Low | Medium | Clear timeout messages, easy retry |
| Breaking changes to ghtkn | Medium | Low | Vendor dependencies, pin versions |

## 14. Timeline

### Week 1-2: Foundation
- Implement GitHub User store
- Implement GitHub App store
- OS keychain integration
- Basic tests

### Week 3: Integration
- Auth manager integration
- CLI command updates
- Environment variable injection
- Store interface implementation

### Week 4: Testing & Polish
- Comprehensive test suite
- Error handling improvements
- Performance optimization
- Security audit

### Week 5: Documentation
- Reference documentation
- Tutorial creation
- Blog post writing
- Example configurations

### Week 6: Release
- Beta testing
- Bug fixes
- Final documentation review
- Public release

## 15. Appendix

### A. GitHub API References

- [OAuth Device Flow](https://docs.github.com/en/apps/oauth-apps/building-oauth-apps/authorizing-oauth-apps#device-flow)
- [OAuth Scopes](https://docs.github.com/en/apps/oauth-apps/building-oauth-apps/scopes-for-oauth-apps)
- [GitHub Apps Authentication](https://docs.github.com/en/apps/creating-github-apps/authenticating-with-a-github-app)
- [GitHub App Permissions](https://docs.github.com/en/rest/overview/permissions-required-for-github-apps)
- [Installation Access Tokens](https://docs.github.com/en/apps/creating-github-apps/authenticating-with-a-github-app/generating-an-installation-access-token-for-a-github-app)

### B. Related Atmos Features

- Atmos Auth System (`docs/prd/PRD-Atmos-Auth.md`)
- Store System (`pkg/store/`)
- Template Functions (`internal/exec/template_funcs.go`)
- Command Registry Pattern (`docs/prd/command-registry-pattern.md`)

### C. Security Considerations

1. **Private Key Storage**
   - Never store in plain text
   - Validate file permissions (0600)
   - Support external secret stores
   - Rotate regularly

2. **Token Handling**
   - Never log tokens
   - Redact in error messages
   - Clear from memory after use
   - Encrypt at rest in keychain

3. **OAuth Flow Security**
   - Use Device Flow (not Web Flow) for CLI
   - Validate state parameters
   - Implement PKCE if available
   - Clear browser cache after auth

4. **Rate Limiting**
   - Respect GitHub API limits
   - Cache tokens appropriately
   - Implement exponential backoff
   - Warn users of limits

### D. Glossary

- **Device Flow**: OAuth 2.0 authorization grant for input-constrained devices
- **Installation Token**: Short-lived token for GitHub App installations
- **JWT**: JSON Web Token, used to authenticate GitHub Apps
- **OAuth Scope**: Permission level for user access tokens
- **Permission**: Granular access control for GitHub Apps
- **Keychain**: OS-provided encrypted credential storage
- **Personal Access Token (PAT)**: Long-lived user token (legacy approach)
