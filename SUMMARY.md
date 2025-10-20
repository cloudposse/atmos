# GitHub Identity Integration - Planning Summary

## Completed Deliverables

### 1. Implementation Plan (`GITHUB_IDENTITY_PLAN.md`)
Comprehensive implementation plan covering:
- Configuration schema design with full examples
- Implementation architecture and file structure
- Authentication flow integration with `atmos auth exec` and `atmos auth env`
- Token lifecycle management (8h for users, 1h for apps)
- Security considerations
- Complete usage examples and command reference
- Documentation updates needed
- Blog post outline
- Implementation checklist across 5 phases
- Open questions and dependencies

### 2. Product Requirements Document (`docs/prd/github-authentication-providers.md`)
Full PRD following Atmos standards including:
- Executive summary and problem statement
- 9 functional requirements (FR-001 through FR-009)
- Non-functional requirements (performance, security, usability)
- Complete architecture with component and flow diagrams
- Configuration reference with all options documented
- **Complete OAuth scopes reference** (30+ scopes for user tokens)
- **Complete permissions reference** (30+ permissions for GitHub Apps)
- Use cases with real-world examples
- Migration path from existing solutions
- Testing strategy (unit, integration, security)
- Documentation plan (3 new pages + blog post)
- 6-week timeline
- Success criteria across 3 phases
- Risk analysis and mitigations

### 3. Partial Implementation (`pkg/store/github_user_store.go`)
Started GitHub User store implementation with:
- Store interface implementation skeleton
- Configuration options structure
- Device Flow authentication outline
- OS keychain integration setup

## Key Design Decisions

### Provider Kinds (Namespaced Format)
- **GitHub User**: `kind: github/user`
- **GitHub App**: `kind: github/app`

### Configurable Permissions
Both provider types support fine-grained permission configuration:

**User OAuth Scopes:**
```yaml
scopes:
  - repo           # Full control of private repos
  - workflow       # Update GitHub Actions workflows
  - read:org       # Read organization membership
  - admin:org      # Full org management
  # ... 20+ more scopes
```

**App Permissions:**
```yaml
permissions:
  contents: write         # Read/write repository contents
  pull_requests: write    # Create/edit PRs
  issues: write           # Create/edit issues
  workflows: write        # Update workflow files
  # ... 25+ more permissions
```

### Environment Variable Injection
Tokens automatically injected when using `atmos auth exec` or `atmos auth env`:

```bash
# For all identities
export GITHUB_TOKEN='ghs_...'
export GH_TOKEN='ghs_...'

# Additional for GitHub Apps
export GITHUB_APP_ID='123456'
export GITHUB_INSTALLATION_ID='789012'
```

### Store Interface Integration
Both providers implement the existing `Store` interface, allowing usage in templates:

```yaml
components:
  terraform:
    vpc:
      vars:
        github_token: '{{ atmos.Store "github/user" "" "" "token" }}'
```

### Private Key Sources (GitHub Apps)
Support multiple secure sources:
```yaml
private_key_path: "/path/to/key.pem"        # File system
private_key_env: "GITHUB_APP_PRIVATE_KEY"   # Environment variable
private_key_store: "aws-ssm:/prod/key"      # Existing store
```

## Configuration Examples

### Complete Working Example

```yaml
# atmos.yaml
auth:
  providers:
    # Developer authentication
    github-dev:
      kind: github/user
      client_id: "Iv1.abc123def456"
      scopes: [repo, workflow, read:org]
      keychain_service: "atmos-github-dev"

    # CI/CD automation
    github-ci:
      kind: github/app
      app_id: "123456"
      installation_id: "789012"
      private_key_env: "GITHUB_APP_PRIVATE_KEY"
      permissions:
        contents: write
        pull_requests: write
        issues: write
      repositories:
        - "cloudposse/*"

  identities:
    dev:
      kind: github/user
      default: true
      via:
        provider: github-dev
      env:
        - key: GITHUB_TOKEN
          value: "{{ .Token }}"
        - key: GH_TOKEN
          value: "{{ .Token }}"

    terraform-bot:
      kind: github/app
      via:
        provider: github-ci
      env:
        - key: GITHUB_TOKEN
          value: "{{ .Token }}"
        - key: TF_VAR_github_token
          value: "{{ .Token }}"
```

### Usage Commands

```bash
# Interactive Device Flow authentication
$ atmos auth login -i dev

# Check authentication status
$ atmos auth whoami -i dev

# Execute with GitHub identity
$ atmos auth exec -i dev -- gh repo list cloudposse
$ atmos auth exec -i terraform-bot -- terraform apply

# Export environment variables
$ eval "$(atmos auth env -i dev)"
$ atmos auth env -i dev -f dotenv > .env
$ atmos auth env -i dev -f json | jq
```

## Documentation Structure

### New Pages Required

1. **`website/docs/cli/commands/auth/providers/github-user.mdx`**
   - GitHub User authentication setup
   - OAuth scopes guide
   - Security best practices

2. **`website/docs/cli/commands/auth/providers/github-app.mdx`**
   - GitHub App authentication setup
   - Permissions guide
   - Private key management

3. **`website/docs/cli/commands/auth/tutorials/github-authentication.mdx`**
   - Complete step-by-step tutorial
   - Use cases and examples
   - Migration from GITHUB_TOKEN

### Blog Post

**Title:** "Secure GitHub Authentication with Atmos: User Tokens & GitHub Apps"

**Tags:** `[feature, github, authentication, security]`

**Key Points:**
- Short-lived tokens (8h users, 1h apps) vs static PATs
- Centralized token management in OS keychain
- Granular permission configuration
- Seamless integration with Terraform, gh CLI, GitHub Actions

## Implementation Phases

### Phase 1: Core Implementation (Weeks 1-2)
- GitHub User store with Device Flow
- GitHub App store with JWT signing
- OS keychain integration
- Basic tests

### Phase 2: Integration (Week 3)
- Auth manager integration
- CLI command updates
- Environment variable injection
- Store interface implementation

### Phase 3: Testing & Polish (Week 4)
- Comprehensive test suite
- Error handling improvements
- Performance optimization
- Security audit

### Phase 4: Documentation (Week 5)
- Reference documentation (3 pages)
- Tutorial creation
- Blog post writing
- Example configurations

### Phase 5: Release (Week 6)
- Beta testing
- Bug fixes
- Final documentation review
- Public release

## Security Considerations

1. **Token Storage**
   - User tokens: Encrypted in OS keychain
   - App tokens: In-memory only (never disk)
   - Never log tokens or private keys

2. **Private Key Management**
   - Validate PEM format before use
   - Check file permissions (0600)
   - Support secure stores (AWS SSM, etc.)
   - Never expose in logs or errors

3. **Scope Validation**
   - Validate against GitHub's allowed list
   - Warn if requesting beyond granted permissions
   - Document security implications

4. **Automatic Expiration**
   - User tokens: 8h default (configurable)
   - App tokens: 1h (GitHub enforced)
   - Automatic refresh on expiration

## Open Questions

1. **Installation ID Discovery**: Auto-discover from app ID?
   - Decision: Phase 2 feature, manual for MVP

2. **Multi-Installation Support**: Multiple orgs?
   - Decision: Multiple providers with different installation_ids

3. **Token Refresh UI**: Show expiration warnings?
   - Decision: Yes, warn at 5 minutes before expiration

4. **Keychain Fallback**: If unavailable?
   - Decision: Graceful degradation to env vars with warning

5. **Repository Glob Patterns**: Support wildcards?
   - Decision: Yes, support `*` and `**` patterns

6. **GitHub Enterprise**: Support GHES?
   - Decision: Add optional `base_url` parameter

## Dependencies

### Go Dependencies
- `github.com/suzuki-shunsuke/ghtkn` - Device Flow (already added)
- `github.com/golang-jwt/jwt/v5` - JWT signing (to be added)
- `github.com/google/go-github/v59` - GitHub API (already present)

### OS Dependencies
- macOS: Keychain Access
- Windows: Credential Manager
- Linux: Secret Service (GNOME Keyring, KWallet)

## Next Steps

1. **Review & Approval**
   - Review PRD and implementation plan
   - Approve configuration schema
   - Confirm timeline

2. **Implementation**
   - Complete GitHub User store
   - Implement GitHub App store
   - Add to store registry
   - Update auth manager

3. **Testing**
   - Create mocks for GitHub API
   - Write unit tests
   - Create integration test fixtures
   - Security testing

4. **Documentation**
   - Write provider documentation
   - Create tutorial
   - Write blog post
   - Update examples

5. **Release**
   - Beta testing period
   - Community feedback
   - Final polish
   - Public announcement

## Files Created/Modified

### New Files
- `GITHUB_IDENTITY_PLAN.md` - Implementation plan
- `docs/prd/github-authentication-providers.md` - Full PRD
- `pkg/store/github_user_store.go` - Partial implementation
- `SUMMARY.md` - This file

### Files to Create (Implementation)
- `pkg/store/github_app_store.go` - GitHub App store
- `pkg/store/github_user_store_test.go` - User store tests
- `pkg/store/github_app_store_test.go` - App store tests
- `pkg/github/device_flow.go` - Device Flow helper
- `pkg/github/app_auth.go` - App auth helper
- `pkg/github/keychain.go` - OS keychain integration

### Files to Update (Implementation)
- `pkg/store/registry.go` - Register new store types
- `pkg/schema/schema_auth.go` - Add GitHub provider types
- `cmd/auth_login.go` - Device Flow support
- `cmd/auth_whoami.go` - Show GitHub info
- `go.mod` - Add JWT dependency

### Files to Create (Documentation)
- `website/docs/cli/commands/auth/providers/github-user.mdx`
- `website/docs/cli/commands/auth/providers/github-app.mdx`
- `website/docs/cli/commands/auth/tutorials/github-authentication.mdx`
- `website/blog/YYYY-MM-DD-github-authentication.md`

### Files to Update (Documentation)
- `website/docs/core-concepts/projects/configuration/stores.mdx`
- `website/docs/cli/commands/auth/usage.mdx`
