# PRD: AWS Credential Process Support for Atmos Auth

## Overview

Support AWS SDK's `credential_process` configuration as a first-class credential source for AWS identities in Atmos Auth. This enables integration with external credential helper programs (Okta CLI, custom SAML tools, hardware tokens, etc.) following the AWS standard for credential provider chains.

## Background

### The Problem

Organizations use external processes to obtain temporary AWS credentials from various sources:
- SAML-based SSO providers (Okta, Azure AD, Google Workspace)
- Hardware security tokens
- Custom credential vending systems
- Corporate identity management tools

Currently, Atmos Auth supports:
- IAM user long-lived credentials (access key + secret key)
- STS session tokens with MFA
- Role assumption chains
- IAM Identity Center (SSO)

However, users cannot integrate external credential helper programs that follow the AWS `credential_process` standard.

### User Story

**As a** platform engineer using corporate SSO via external tools,
**I want** to configure Atmos to obtain credentials from an external process,
**So that** I can use my organization's existing credential tooling with Atmos workflows.

**Example configuration:**
```yaml
auth:
  identities:
    staging:
      kind: aws/user
      credentials:
        credential_process: '{{getenv "HOME"}}/.local/bin/okta-credential-helper staging'
        region: eu-west-1
```

### Current Workaround

Users must manually run credential helper tools, copy temporary credentials, and either:
- Set environment variables before running Atmos
- Store credentials in `~/.aws/credentials` and configure Atmos to read from there

Both approaches break Atmos's unified authentication model and require manual credential refresh.

## Goals

1. **Standards Compliance**: Support AWS SDK's `credential_process` specification exactly as documented
2. **Seamless Integration**: Work with existing Atmos Auth workflows (shell, whoami, identity chaining)
3. **Credential Lifecycle**: Handle expiration, caching, and refresh automatically
4. **Security**: Execute external processes safely with proper error handling
5. **Observability**: Log credential source and expiration for debugging

## Non-Goals

- Creating a new identity kind `aws/process` (use existing `aws/user`)
- Supporting non-AWS credential processes
- Implementing credential helper programs (only consume them)
- Validating external process security (user's responsibility)

## Technical Design

### 1. Configuration Schema

Extend `aws/user` identity to accept `credential_process` in credentials map:

```yaml
auth:
  identities:
    <identity-name>:
      kind: aws/user
      credentials:
        # Option 1: External credential process (NEW)
        credential_process: '<command with arguments>'
        region: <aws-region>  # Optional, defaults to us-east-1

        # Option 2: Long-lived credentials (EXISTING)
        access_key_id: '{{getenv "AWS_ACCESS_KEY_ID"}}'
        secret_access_key: '{{getenv "AWS_SECRET_ACCESS_KEY"}}'
        mfa_arn: 'arn:aws:iam::123456789012:mfa/username'  # Optional
        region: <aws-region>  # Optional

      session:
        duration: '12h'  # Optional, applies if external process returns non-session credentials
```

**Mutual Exclusivity:**
- `credential_process` is mutually exclusive with `access_key_id`/`secret_access_key`
- If both are configured, return validation error

### 2. Credential Process Specification

Follow AWS SDK specification exactly:

**Command Execution:**
- Execute command via shell with template variable substitution
- Support `{{getenv "VAR"}}`, `{{atmos.Component()}}`, etc.
- Pass current environment variables to subprocess
- Capture stdout only (stderr ignored or logged as debug)
- Timeout after 30 seconds (configurable via settings)

**Expected JSON Response Format:**
```json
{
  "Version": 1,
  "AccessKeyId": "ASIA...",
  "SecretAccessKey": "...",
  "SessionToken": "...",
  "Expiration": "2025-11-15T18:30:00Z"
}
```

**Required Fields:**
- `Version`: Must be `1`
- `AccessKeyId`: AWS access key ID
- `SecretAccessKey`: AWS secret access key

**Optional Fields:**
- `SessionToken`: For temporary credentials
- `Expiration`: RFC3339 timestamp (e.g., `2025-11-15T18:30:00Z`)

**Error Handling:**
- Non-zero exit code: Return error with stderr output
- Invalid JSON: Return parse error
- Missing required fields: Return validation error
- Expired credentials: Return expiration error

### 3. Implementation Architecture

#### 3.1 Credential Resolution Precedence

Update `resolveLongLivedCredentials()` in `pkg/auth/identities/aws/user.go`:

```
Priority (highest to lowest):
1. credential_process (external command) - NEW
2. YAML credentials (access_key_id + secret_access_key)
3. Keyring credentials
```

#### 3.2 Process Execution Flow

```go
func (i *userIdentity) credentialsFromProcess(ctx context.Context, command string) (*types.AWSCredentials, error) {
    // 1. Expand template variables in command
    expandedCmd := expandTemplate(command, i.config)

    // 2. Execute command with timeout
    ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
    defer cancel()

    output, err := execCommandWithContext(ctx, expandedCmd)
    if err != nil {
        return nil, fmt.Errorf("credential_process failed: %w", err)
    }

    // 3. Parse JSON response
    var resp CredentialProcessResponse
    if err := json.Unmarshal(output, &resp); err != nil {
        return nil, fmt.Errorf("credential_process returned invalid JSON: %w", err)
    }

    // 4. Validate response
    if err := validateCredentialProcessResponse(&resp); err != nil {
        return nil, err
    }

    // 5. Convert to AWSCredentials
    return &types.AWSCredentials{
        AccessKeyID:     resp.AccessKeyId,
        SecretAccessKey: resp.SecretAccessKey,
        SessionToken:    resp.SessionToken,
        Region:          i.resolveRegion(),
        Expiration:      resp.Expiration,
    }, nil
}
```

#### 3.3 Credential Caching Strategy

**Cache Behavior:**
- Cache credentials in memory during Atmos execution
- Respect `Expiration` field from process output
- If no expiration provided, assume credentials valid for current operation only
- Do NOT cache in keyring (external process is source of truth)

**Refresh Behavior:**
- Check expiration before using cached credentials
- Re-execute process if credentials expired or missing
- Log credential refresh for observability

**File Storage:**
- Write temporary credentials to AWS config files (same as existing flow)
- Include expiration metadata in INI comments
- This enables AWS SDK tools to use cached credentials

#### 3.4 Session Token Handling

**If external process returns SessionToken:**
- Use credentials directly without calling STS GetSessionToken
- Skip MFA prompt (external process handles authentication)
- Write credentials to AWS files with session token

**If external process returns long-lived credentials (no SessionToken):**
- Fall back to existing GetSessionToken flow
- Prompt for MFA if `mfa_arn` configured
- Generate session token with configured duration

#### 3.5 Integration with Existing Features

**Identity Chaining:**
```yaml
auth:
  identities:
    corp-sso:
      kind: aws/user
      credentials:
        credential_process: '/usr/local/bin/okta-aws staging'

    staging-admin:
      kind: aws/assume-role
      via:
        identity: corp-sso
      principal:
        role_arn: 'arn:aws:iam::111111111111:role/admin'
```

**Atmos Auth Shell:**
```bash
$ atmos auth shell --identity corp-sso
# Executes credential_process, spawns shell with AWS credentials
```

**Atmos Terraform:**
```bash
$ atmos terraform plan vpc -s staging --identity corp-sso
# Executes credential_process, runs terraform with credentials
```

### 4. Security Considerations

**Command Execution Safety:**
- Execute commands via `exec.CommandContext` with timeout
- Do NOT use shell expansion unless explicitly needed
- Validate command path exists before execution
- Log command execution (mask sensitive arguments)

**Credential Handling:**
- Never log credential values (access keys, session tokens)
- Mask credentials in error messages
- Clear sensitive data from memory after use
- Follow existing secret masking patterns

**Process Isolation:**
- Execute with same user/group as Atmos process
- Pass minimal environment variables
- Capture and sanitize stderr before logging

### 5. Error Handling

**Error Categories:**

1. **Configuration Errors** (fail fast at validation):
   - Both `credential_process` and `access_key_id` configured
   - Empty `credential_process` string
   - Invalid template syntax

2. **Execution Errors** (fail at runtime):
   - Process not found or not executable
   - Process timeout (30s default)
   - Non-zero exit code

3. **Response Errors** (fail at runtime):
   - Invalid JSON format
   - Missing required fields
   - Invalid Version number
   - Malformed expiration timestamp

4. **Credential Errors** (fail at use):
   - Credentials expired
   - Credentials invalid (STS validation fails)

**Error Messages:**
```
❌ credential_process failed: command not found: /path/to/helper
❌ credential_process timeout after 30s
❌ credential_process returned invalid JSON: unexpected token
❌ credential_process missing required field: AccessKeyId
❌ credential_process returned expired credentials (expired at 2025-11-15T12:00:00Z)
```

### 6. Configuration Validation

Add validation in `pkg/auth/types/interfaces.go` Validator:

```go
func (v *validator) ValidateIdentity(name string, identity *schema.Identity, providers map[string]*schema.Provider) error {
    if identity.Kind == "aws/user" {
        hasCredProcess := identity.Credentials["credential_process"] != nil
        hasAccessKey := identity.Credentials["access_key_id"] != nil
        hasSecretKey := identity.Credentials["secret_access_key"] != nil

        // Mutual exclusivity check
        if hasCredProcess && (hasAccessKey || hasSecretKey) {
            return fmt.Errorf("identity %q: credential_process is mutually exclusive with access_key_id/secret_access_key", name)
        }

        // Validate credential_process command
        if hasCredProcess {
            cmd, ok := identity.Credentials["credential_process"].(string)
            if !ok || cmd == "" {
                return fmt.Errorf("identity %q: credential_process must be a non-empty string", name)
            }
        }
    }
    return nil
}
```

### 7. Testing Strategy

**Unit Tests:**
- Credential process execution with mock subprocess
- JSON response parsing (valid and invalid cases)
- Expiration handling
- Error handling (timeout, non-zero exit, invalid JSON)
- Template variable expansion
- Precedence order (credential_process > YAML > keyring)

**Integration Tests:**
- Mock credential helper script returning valid credentials
- Mock credential helper returning expired credentials
- Mock credential helper with non-zero exit
- Mock credential helper with invalid JSON
- Credential caching and refresh
- Identity chaining with credential_process

**Test Fixtures:**
```bash
# tests/fixtures/credential-helpers/mock-aws-helper.sh
#!/bin/bash
cat <<EOF
{
  "Version": 1,
  "AccessKeyId": "ASIATESTACCESSKEY",
  "SecretAccessKey": "test-secret-access-key",
  "SessionToken": "test-session-token",
  "Expiration": "$(date -u -d '+1 hour' +%Y-%m-%dT%H:%M:%SZ)"
}
EOF
```

### 8. Documentation Requirements

**User Documentation:**
- Add section to AWS authentication guide: "Using External Credential Processes"
- Document credential_process JSON format
- Provide examples for common tools (Okta CLI, aws-vault, etc.)
- Security best practices for external processes

**Reference Documentation:**
- Update `website/docs/cli/commands/auth/` with credential_process examples
- Add credential_process to identity schema reference
- Document precedence order in credential resolution

**Migration Guide:**
- How to migrate from manual credential helpers to integrated credential_process
- How to test credential_process configuration
- Troubleshooting common issues

## Success Metrics

1. **Functionality**: Users can configure credential_process and authenticate successfully
2. **Compatibility**: Works with common credential helpers (Okta CLI, aws-vault, custom scripts)
3. **Reliability**: Credentials refresh automatically before expiration
4. **Performance**: Credential process execution completes within 30s timeout
5. **Observability**: Clear logging when credentials sourced from external process

## Risks and Mitigations

| Risk | Impact | Mitigation |
|------|--------|------------|
| External process hangs indefinitely | High - Atmos blocks | Implement 30s timeout with context cancellation |
| External process returns invalid credentials | Medium - Authentication fails | Validate response format before use |
| Security: process execution vulnerability | High - Code execution | Use exec.CommandContext, avoid shell expansion |
| Credential leakage in logs | High - Security breach | Mask credentials in all log output |
| Breaking changes to aws/user behavior | Medium - User confusion | Maintain backward compatibility, add validation |

## Open Questions

1. **Command execution**: Use `sh -c` for shell features (pipes, env vars) or direct execution?
   - **Decision**: Direct execution for security, support template variables for flexibility

2. **Timeout configuration**: Should timeout be configurable per-identity or global?
   - **Decision**: Global default (30s), consider per-identity override in future

3. **Credential refresh**: Proactive refresh before expiration or on-demand?
   - **Decision**: On-demand (check expiration before use), aligns with existing behavior

4. **Keyring storage**: Should credentials from external process be cached in keyring?
   - **Decision**: No, external process is source of truth (avoid stale credentials)

5. **MFA handling**: If external process returns long-lived credentials, should we still call GetSessionToken?
   - **Decision**: Yes, for consistency and to enable MFA enforcement

## Implementation Phases

### Phase 1: Core Implementation (MVP)
- Add credential_process support to aws/user identity
- Process execution with timeout
- JSON response parsing and validation
- Credential caching with expiration
- Unit tests

### Phase 2: Integration
- Identity chaining with credential_process
- Integration with auth shell, whoami
- Integration tests with mock credential helper
- Error handling improvements

### Phase 3: Documentation and Polish
- User documentation with examples
- Reference documentation updates
- Troubleshooting guide
- Performance optimization

## References

- [AWS CLI Credential Process Documentation](https://docs.aws.amazon.com/cli/latest/userguide/cli-configure-sourcing-external.html)
- [AWS SDK Credential Process Specification](https://docs.aws.amazon.com/sdkref/latest/guide/feature-process-credentials.html)
- [Okta AWS CLI Tool](https://github.com/okta/okta-aws-cli)
- [aws-vault](https://github.com/99designs/aws-vault)

## Related PRDs

- `docs/prd/command-registry-pattern.md` - Command architecture
- `docs/prd/error-handling-strategy.md` - Error handling patterns
- `docs/prd/testing-strategy.md` - Testing approach

## Appendix: Example Configurations

### Example 1: Okta AWS CLI
```yaml
auth:
  identities:
    staging:
      kind: aws/user
      credentials:
        credential_process: 'okta-aws-cli --profile staging --oidc-client-id 0oa123456789abcdef'
        region: us-east-1
```

### Example 2: Custom SAML Script
```yaml
auth:
  identities:
    production:
      kind: aws/user
      credentials:
        credential_process: '{{getenv "HOME"}}/.local/bin/saml-to-aws --account production --role admin'
        region: us-west-2
      session:
        duration: '8h'  # Used if process returns long-lived credentials
```

### Example 3: Identity Chaining
```yaml
auth:
  identities:
    corp-base:
      kind: aws/user
      credentials:
        credential_process: '/usr/local/bin/corporate-sso-helper'

    staging-poweruser:
      kind: aws/assume-role
      via:
        identity: corp-base
      principal:
        role_arn: 'arn:aws:iam::111111111111:role/PowerUser'

    staging-readonly:
      kind: aws/assume-role
      via:
        identity: corp-base
      principal:
        role_arn: 'arn:aws:iam::111111111111:role/ReadOnly'
```

### Example 4: AWS Vault Integration
```yaml
auth:
  identities:
    my-account:
      kind: aws/user
      credentials:
        credential_process: 'aws-vault exec my-profile --json'
        region: eu-central-1
```
