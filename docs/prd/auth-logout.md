# Product Requirements Document: Atmos Auth Logout

## Executive Summary

The `atmos auth logout` command provides secure, comprehensive cleanup of locally cached authentication credentials and session data. It removes credentials from both the system keyring and provider-specific file storage (e.g., AWS credential files), ensuring a clean logout experience while maintaining security best practices.

## 1. Problem Statement

### Current Challenges

- **No Logout Mechanism**: Users cannot remove cached credentials without manually deleting files and keyring entries
- **Security Concerns**: Stale credentials may remain in the keyring and on disk after authentication expires
- **Developer Experience**: No clear way to switch between identities or clear authentication state
- **Compliance Requirements**: Organizations need audit trails showing when users logged out of systems
- **Session Management**: Browser sessions remain active even after local credential cleanup

### Success Metrics

- **Credential Cleanup**: 100% removal of all cached credentials for logged-out identities
- **Security**: Zero false positives (never delete credentials for identities not specified)
- **User Experience**: Clear, informative output showing what was removed
- **Error Resilience**: Best-effort cleanup continues even if individual steps fail

## 2. Core Requirements

### 2.1 Functional Requirements

#### FR-001: Identity-Specific Logout

- **Description**: Remove credentials for a specific identity
- **Acceptance Criteria**:
  - Accept identity name as argument: `atmos auth logout <identity>`
  - Build complete authentication chain for the identity
  - Remove keyring entries for all identities in the chain
  - Remove provider-specific files (AWS: `~/.aws/atmos/<provider>/`)
  - Display list of removed identities
  - Handle missing credentials gracefully (treat as already logged out)
- **Priority**: P0 (Must Have)

#### FR-002: Provider-Specific Logout

- **Description**: Remove all credentials for a specific provider
- **Acceptance Criteria**:
  - Support `--provider <name>` flag
  - Remove all credentials associated with the provider
  - Clean up provider-specific file storage
  - Display confirmation of removed provider data
  - Support AWS as initial provider implementation
- **Priority**: P0 (Must Have)

#### FR-003: Interactive Identity Selection

- **Description**: Prompt user when no identity/provider specified
- **Acceptance Criteria**:
  - List all available identities from configuration
  - Use Charmbracelet Huh for styled selection
  - Allow "All identities" option
  - Apply Atmos theme to prompts
  - Exit gracefully on Ctrl+C
- **Priority**: P0 (Must Have)

#### FR-004: Comprehensive Credential Cleanup

- **Description**: Remove all credential storage locations
- **Acceptance Criteria**:
  - Delete entries from system keyring using `go-keyring`
  - Remove AWS credential files: `<base_path>/<provider>/credentials`
  - Remove AWS config files: `<base_path>/<provider>/config`
  - Support configurable base path via `spec.files.base_path` (default: `~/.aws/atmos`)
  - Remove empty provider directories after cleanup
  - Use native Go file operations (`os.RemoveAll`, not shell commands)
  - Continue cleanup even if individual steps fail
- **Priority**: P0 (Must Have)

#### FR-005: Clear User Communication

- **Description**: Inform users about logout scope and limitations
- **Acceptance Criteria**:
  - Display which identities were logged out
  - Show file paths that were removed
  - Clarify that browser sessions are NOT affected
  - Provide summary of cleanup actions
  - Use Charmbracelet styling for output
- **Priority**: P0 (Must Have)

#### FR-006: Error Resilience

- **Description**: Best-effort cleanup continues despite errors
- **Acceptance Criteria**:
  - Continue cleanup if keyring deletion fails
  - Continue cleanup if file deletion fails
  - Collect and report all errors at end
  - Exit with error code only if zero cleanup succeeded
  - Log detailed error information for debugging
- **Priority**: P0 (Must Have)

### 2.2 Non-Functional Requirements

#### NFR-001: Security

- **Never delete unrelated credentials**
- Validate identity/provider names before deletion
- Log all deletion operations for audit trail
- Ensure proper permissions on remaining files

#### NFR-002: Performance

- Logout operation completes in <1 second for single identity
- Minimal memory overhead (no credential loading into memory)
- Efficient file system operations (batch deletions where possible)

#### NFR-003: Cross-Platform Compatibility

- Works on macOS, Linux, and Windows
- Uses Go's standard library for file operations
- Leverages `go-keyring` for cross-platform keyring access
- Handles platform-specific path separators correctly

#### NFR-004: Testability

- Unit tests for each logout component
- Integration tests for full logout flow
- Mock keyring for testing without system dependencies
- Mock file system for testing file cleanup

## 3. User Experience Design

### 3.1 Command Syntax

```bash
# Logout from specific identity
atmos auth logout <identity>

# Logout from specific provider
atmos auth logout --provider <provider-name>

# Interactive prompt (no arguments)
atmos auth logout

# Help
atmos auth logout --help
```

### 3.2 Interactive Flow

When run without arguments:

```
? Choose what to logout from:
  ❯ Identity: dev-admin
    Identity: prod-admin
    Identity: dev-readonly
    Provider: aws-sso (removes all identities)
    All identities (complete logout)
```

### 3.3 Output Examples

#### Successful Logout

```bash
$ atmos auth logout dev-admin

Logging out from identity: dev-admin

Building authentication chain...
  ✓ Chain: aws-sso → dev-org-admin → dev-admin

Removing credentials...
  ✓ Keyring: aws-sso
  ✓ Keyring: dev-org-admin
  ✓ Keyring: dev-admin
  ✓ Files: ~/.aws/atmos/aws-sso/

Successfully logged out from 3 identities

⚠️  Note: This only removes local credentials. Your browser session
   may still be active. Visit your identity provider to end your
   browser session.
```

#### Provider Logout

```bash
$ atmos auth logout --provider aws-sso

Logging out from provider: aws-sso

Removing all credentials for provider...
  ✓ Keyring: aws-sso
  ✓ Keyring: dev-org-admin (via aws-sso)
  ✓ Keyring: dev-admin (via aws-sso)
  ✓ Keyring: prod-admin (via aws-sso)
  ✓ Files: ~/.aws/atmos/aws-sso/

Successfully logged out from 4 identities

⚠️  Note: This only removes local credentials.
```

#### Partial Failure

```bash
$ atmos auth logout dev-admin

Logging out from identity: dev-admin

Building authentication chain...
  ✓ Chain: aws-sso → dev-admin

Removing credentials...
  ✓ Keyring: aws-sso
  ✗ Keyring: dev-admin (not found - already logged out)
  ✓ Files: ~/.aws/atmos/aws-sso/

Logged out with warnings (2/3 successful)

Errors encountered:
  • dev-admin: credential not found in keyring
```

#### Already Logged Out

```bash
$ atmos auth logout dev-admin

Identity 'dev-admin' is already logged out.
No credentials found in keyring or file storage.
```

### 3.4 Error Scenarios

#### Identity Not Found

```bash
$ atmos auth logout nonexistent

Error: identity "nonexistent" not found in configuration

Available identities:
  • dev-admin
  • prod-admin
  • dev-readonly

Run 'atmos auth logout' without arguments for interactive selection.
```

#### Provider Not Found

```bash
$ atmos auth logout --provider nonexistent

Error: provider "nonexistent" not found in configuration

Available providers:
  • aws-sso
  • github-oidc

Run 'atmos auth logout' without arguments for interactive selection.
```

## 4. Technical Design

### 4.1 Configuration

#### AWS Files Base Path

AWS providers support configurable file storage paths via `spec.files.base_path`:

```yaml
# atmos.yaml
auth:
  providers:
    aws-sso:
      kind: aws/iam-identity-center
      start_url: https://example.awsapps.com/start
      region: us-east-1
      spec:
        files:
          base_path: ~/.custom/aws/credentials  # Optional, defaults to ~/.aws/atmos
```

**Configuration Precedence**:
1. `spec.files.base_path` in provider configuration
2. `ATMOS_AWS_FILES_BASE_PATH` environment variable
3. Default: `~/.aws/atmos`

**Validation**:
- Path must not be empty or whitespace-only
- Path must not contain null bytes, carriage returns, or newlines
- Tilde (`~`) expansion is supported via `go-homedir`
- Validation occurs during `atmos auth validate`

**Use Cases**:
- **Custom Directories**: Store credentials in non-standard locations
- **Container Environments**: Use volume mounts at custom paths
- **Multi-User Systems**: Isolate credentials per user/project

### 4.2 Interface Extensions

#### Provider Interface

```go
// pkg/auth/types/interfaces.go
type Provider interface {
    // ... existing methods

    // Logout removes provider-specific credential storage.
    // Returns error only if cleanup fails for critical resources.
    Logout(ctx context.Context) error
}
```

#### Identity Interface

```go
// pkg/auth/types/interfaces.go
type Identity interface {
    // ... existing methods

    // Logout removes identity-specific credential storage.
    // Receives file manager for provider-specific cleanup.
    Logout(ctx context.Context) error
}
```

#### AuthManager Interface

```go
// pkg/auth/types/interfaces.go
type AuthManager interface {
    // ... existing methods

    // Logout removes credentials for the specified identity and its chain.
    Logout(ctx context.Context, identityName string) error

    // LogoutProvider removes all credentials for the specified provider.
    LogoutProvider(ctx context.Context, providerName string) error

    // LogoutAll removes all credentials for all identities.
    LogoutAll(ctx context.Context) error
}
```

### 4.2 Implementation Components

#### Component 1: AuthManager Logout Methods

**File**: `pkg/auth/manager.go`

```go
// Logout removes credentials for a specific identity and its authentication chain.
func (m *manager) Logout(ctx context.Context, identityName string) error {
    // 1. Validate identity exists in configuration
    // 2. Build authentication chain
    // 3. Delete keyring entries for each chain step (best-effort)
    // 4. Call provider-specific cleanup
    // 5. Collect and report errors
    // 6. Return aggregated error if all steps failed
}

// LogoutProvider removes all credentials for a provider.
func (m *manager) LogoutProvider(ctx context.Context, providerName string) error {
    // 1. Find all identities using this provider
    // 2. Call Logout for each identity
    // 3. Clean up provider-specific storage
    // 4. Return aggregated errors
}

// LogoutAll removes all cached credentials.
func (m *manager) LogoutAll(ctx context.Context) error {
    // 1. Iterate all identities in configuration
    // 2. Call Logout for each
    // 3. Return aggregated errors
}
```

#### Component 2: AWS Provider Logout

**File**: `pkg/auth/providers/aws/sso.go`, `pkg/auth/providers/aws/saml.go`

```go
func (p *SSOProvider) Logout(ctx context.Context) error {
    fileManager, err := NewAWSFileManager()
    if err != nil {
        return err
    }
    return fileManager.Cleanup(p.name)
}
```

#### Component 3: AWS Identity Logout

**File**: `pkg/auth/identities/aws/permission_set.go`, `pkg/auth/identities/aws/assume_role.go`

```go
func (i *PermissionSetIdentity) Logout(ctx context.Context) error {
    // AWS identities rely on provider cleanup
    // No additional cleanup needed at identity level
    return nil
}
```

#### Component 4: File Manager Cleanup

**File**: `pkg/auth/cloud/aws/files.go`

```go
// Cleanup removes all files for a provider (already exists).
func (m *AWSFileManager) Cleanup(providerName string) error {
    providerDir := filepath.Join(m.baseDir, providerName)

    if err := os.RemoveAll(providerDir); err != nil {
        if os.IsNotExist(err) {
            // Already removed - not an error
            return nil
        }
        return ErrCleanupAWSFiles
    }

    return nil
}

// CleanupAll removes entire ~/.aws/atmos directory.
func (m *AWSFileManager) CleanupAll() error {
    if err := os.RemoveAll(m.baseDir); err != nil {
        if os.IsNotExist(err) {
            return nil
        }
        return ErrCleanupAWSFiles
    }

    return nil
}
```

#### Component 5: CLI Command

**File**: `cmd/auth_logout.go`

```go
var authLogoutCmd = &cobra.Command{
    Use:   "logout [identity]",
    Short: "Remove locally cached credentials and session data",
    Long:  `Removes cached credentials from the system keyring and local credential files.`,
    RunE:  executeAuthLogoutCommand,
}

func executeAuthLogoutCommand(cmd *cobra.Command, args []string) error {
    // 1. Load atmos config
    // 2. Create auth manager
    // 3. Determine what to logout (identity, provider, or prompt)
    // 4. Perform logout
    // 5. Display results with Charmbracelet styling
    // 6. Show browser session warning
}

func init() {
    authLogoutCmd.Flags().String("provider", "", "Logout from specific provider")
    authCmd.AddCommand(authLogoutCmd)
}
```

### 4.3 Error Handling Strategy

Following Atmos error handling conventions:

```go
// errors/errors.go - Add new error sentinels
var (
    ErrLogoutFailed = errors.New("logout failed")
    ErrPartialLogout = errors.New("partial logout")
)

// Usage in logout code
func (m *manager) Logout(ctx context.Context, identityName string) error {
    var errs []error

    // Collect errors but continue
    if err := m.credentialStore.Delete(identityName); err != nil {
        errs = append(errs, fmt.Errorf("%w: keyring deletion failed: %w",
            ErrLogoutFailed, err))
    }

    // More cleanup steps...

    if len(errs) > 0 {
        return errors.Join(errs...)
    }
    return nil
}
```

### 4.4 Logging Strategy

Use structured logging without affecting execution:

```go
log.Debug("Starting logout", "identity", identityName)
log.Debug("Authentication chain built", "chain", chain)
log.Debug("Removing keyring entry", "alias", alias)

if err != nil {
    log.Debug("Keyring deletion failed", "alias", alias, "error", err)
}

log.Info("Logout completed", "identity", identityName, "removed", count)
```

### 4.5 Telemetry

```go
// Automatic via RootCmd.ExecuteC()
// No additional telemetry code needed
// Captures: command path, error state (boolean only)
```

## 5. Testing Strategy

### 5.1 Unit Tests

**Test File**: `pkg/auth/manager_logout_test.go`

```go
func TestManager_Logout_SingleIdentity(t *testing.T)
func TestManager_Logout_ChainedIdentity(t *testing.T)
func TestManager_Logout_MissingCredentials(t *testing.T)
func TestManager_Logout_PartialFailure(t *testing.T)
func TestManager_LogoutProvider(t *testing.T)
func TestManager_LogoutAll(t *testing.T)
```

**Test File**: `pkg/auth/cloud/aws/files_logout_test.go`

```go
func TestAWSFileManager_Cleanup(t *testing.T)
func TestAWSFileManager_CleanupAll(t *testing.T)
func TestAWSFileManager_CleanupNonExistent(t *testing.T)
```

**Test File**: `cmd/auth_logout_test.go`

```go
func TestAuthLogoutCmd_WithIdentity(t *testing.T)
func TestAuthLogoutCmd_WithProvider(t *testing.T)
func TestAuthLogoutCmd_InvalidIdentity(t *testing.T)
func TestAuthLogoutCmd_AlreadyLoggedOut(t *testing.T)
```

### 5.2 Integration Tests

**Test File**: `tests/auth_logout_integration_test.go`

```go
func TestAuthLogout_EndToEnd(t *testing.T) {
    // 1. Setup: Login with test identity
    // 2. Verify credentials exist
    // 3. Logout
    // 4. Verify credentials removed
    // 5. Verify files removed
}

func TestAuthLogout_MultipleIdentities(t *testing.T) {
    // Test logging out from multiple identities
}
```

### 5.3 Test Preconditions

Use existing precondition patterns:

```go
func TestAuthLogout_AWS(t *testing.T) {
    if testing.Short() {
        t.Skipf("Skipping test requiring AWS profile setup")
    }

    // Test AWS-specific logout
}
```

## 6. Documentation Requirements

### 6.1 CLI Documentation

**File**: `website/docs/cli/commands/auth/logout.mdx`

- Command syntax and examples
- Flag descriptions
- Interactive mode explanation
- Security considerations
- Browser session clarification

### 6.2 Blog Post

**File**: `website/blog/2025-10-17-auth-logout-feature.md`

- Feature announcement
- Usage examples
- Security best practices
- Migration guide (if needed)

### 6.3 User Guide Updates

**File**: `pkg/auth/docs/UserGuide.md`

- Add logout section
- Update authentication workflow diagrams
- Add troubleshooting for logout issues

## 7. Security Considerations

### 7.1 Credential Validation

- Validate identity/provider names exist in configuration before deletion
- Never accept arbitrary paths from user input
- Use authentication chain to determine what to delete

### 7.2 Audit Trail

- Log all logout operations with identity names
- Log file paths being removed
- Log keyring keys being deleted
- Enable compliance auditing

### 7.3 Browser Session Warning

Display prominently after logout:

```
⚠️  Important: This command only removes locally cached credentials.

Your browser session with the identity provider (AWS SSO, Okta, etc.)
may still be active. To completely end your session:

1. Visit your identity provider's website
2. Sign out from the browser session
3. Close all browser windows

Local credentials have been securely removed.
```

### 7.4 Permissions

- Ensure proper file permissions on parent directories after cleanup
- Don't leave world-readable directories
- Remove empty provider directories

## 8. Rollout Plan

### Phase 1: Core Implementation

- [ ] Extend Provider/Identity interfaces with Logout methods
- [ ] Implement AuthManager logout methods
- [ ] Implement AWS provider/identity logout
- [ ] Add file cleanup to AWSFileManager
- [ ] Create CLI command structure

### Phase 2: User Experience

- [ ] Add interactive prompts with Charmbracelet Huh
- [ ] Implement styled output with Charmbracelet lipgloss
- [ ] Add progress indicators for multi-step cleanup
- [ ] Add browser session warning message

### Phase 3: Testing

- [ ] Write unit tests for all components
- [ ] Write integration tests
- [ ] Test on macOS, Linux, Windows
- [ ] Test with various identity chain configurations

### Phase 4: Documentation

- [ ] Write PRD (this document)
- [ ] Create CLI documentation
- [ ] Write blog post
- [ ] Update user guide
- [ ] Build website and verify

### Phase 5: Release

- [ ] Create pull request
- [ ] Code review
- [ ] CI/CD validation
- [ ] Merge to main
- [ ] Release notes

## 9. Future Enhancements

### 9.1 Selective Logout (v2)

Keep provider credentials but remove identity:

```bash
atmos auth logout dev-admin --keep-provider
```

### 9.2 Logout Confirmation (v2)

Require confirmation for destructive operations:

```bash
atmos auth logout --all --confirm
```

### 9.3 Logout on Expiration (v2)

Automatically remove expired credentials:

```bash
atmos auth cleanup-expired
```

### 9.4 Multi-Provider Support (v2)

Extend logout to Azure, GCP, GitHub:

```go
// Azure Entra ID logout
func (p *AzureProvider) Logout(ctx context.Context) error

// GCP OIDC logout
func (p *GCPProvider) Logout(ctx context.Context) error
```

## 10. Open Questions

### Q1: Should we support `--all` flag or just interactive "All identities" option?

**Decision**: Use interactive option only for initial release. Add `--all` flag in v2 if requested.

**Rationale**: Interactive mode is safer and prevents accidental complete logout.

### Q2: Should we support `--force` to ignore errors?

**Decision**: No. Best-effort is default behavior. Always continue cleanup and report errors.

**Rationale**: Consistent with Atmos philosophy of informative error reporting.

### Q3: Should we add `--dry-run` flag?

**Decision**: Yes, add in initial release.

**Rationale**: Allows users to preview what would be deleted. Consistent with `atmos vendor pull --dry-run`.

### Q4: What exit code for partial success?

**Decision**: Exit 0 if any cleanup succeeded, exit 1 if zero cleanup succeeded.

**Rationale**: Best-effort approach means partial success is still success.

## 11. Success Criteria

### Must Have (P0)

- ✅ Remove credentials for specific identity
- ✅ Remove credentials for specific provider
- ✅ Interactive mode for selection
- ✅ Remove keyring entries
- ✅ Remove AWS credential files
- ✅ Best-effort error handling
- ✅ Clear user output
- ✅ Browser session warning
- ✅ Unit tests (80%+ coverage)
- ✅ Integration tests
- ✅ CLI documentation
- ✅ Blog post

### Should Have (P1)

- ⭕ Dry-run mode
- ⭕ Progress indicators
- ⭕ Colored output with theme
- ⭕ Command completion for identity names

### Could Have (P2)

- ⭕ `--all` flag for complete logout
- ⭕ Confirmation prompts for destructive operations
- ⭕ JSON output format
- ⭕ Automated cleanup of expired credentials

## 12. References

- [PRD: Atmos Auth](pkg/auth/docs/prd/PRD-Atmos-Auth.md)
- [Atmos Error Handling Strategy](docs/prd/error-handling-strategy.md)
- [Atmos Testing Strategy](docs/prd/testing-strategy.md)
- [Charmbracelet Huh Documentation](https://github.com/charmbracelet/huh)
- [Charmbracelet Lipgloss Documentation](https://github.com/charmbracelet/lipgloss)
