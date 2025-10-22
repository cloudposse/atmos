# Keyring Backend System PRD

## Executive Summary

This document defines the credential storage backend system for Atmos authentication, providing multiple keyring implementations (system, file, memory) with a unified interface. The system enables secure, cross-platform credential storage while supporting different deployment scenarios from local development to CI/CD pipelines.

## Problem Statement

### Background

Atmos's authentication system (`atmos auth`) needs to securely store temporary credentials obtained from identity providers. These credentials include:
- AWS temporary credentials (access key, secret key, session token)
- OIDC tokens
- Expiration timestamps
- Provider and identity metadata

### Requirements

1. **Secure storage**: Credentials must be encrypted at rest
2. **Cross-platform support**: Work on Linux, macOS, and Windows
3. **Multiple backends**: Support different storage mechanisms for various use cases
4. **Isolated sessions**: Prevent credential leakage between environments
5. **XDG compliance**: Follow platform standards for data storage locations
6. **Testing support**: Enable hermetic testing without system dependencies
7. **Configuration flexibility**: Allow users to choose storage backend

### Use Cases

| Use Case | Backend | Rationale |
|----------|---------|-----------|
| Developer workstation | System | OS-native encryption (Keychain, Credential Manager, Secret Service) |
| Shared servers | File | User-specific encrypted files with password protection |
| CI/CD pipelines | Memory | Ephemeral storage with no persistence |
| Testing | Memory | Isolated, fast, no filesystem dependencies |

## Design Goals

1. **Unified interface**: Single `CredentialStore` interface for all backends
2. **Transparent switching**: Change backends via configuration without code changes
3. **Graceful degradation**: Fall back to working backends when preferred backend fails
4. **Security by default**: Encryption required for persistent storage
5. **XDG compliance**: Follow XDG Base Directory Specification for file locations
6. **Performance tracking**: Instrument all I/O operations for visibility

## Technical Specification

### Architecture

#### CredentialStore Interface

All backends implement this unified interface:

```go
type CredentialStore interface {
    // Store saves credentials under an alias
    Store(alias string, creds ICredentials) error

    // Retrieve loads credentials by alias
    Retrieve(alias string) (ICredentials, error)

    // Delete removes credentials by alias
    Delete(alias string) error

    // List returns all stored credential aliases
    List() ([]string, error)

    // IsExpired checks if credentials are expired
    IsExpired(alias string) (bool, error)

    // GetAny retrieves arbitrary data by key
    GetAny(key string, out interface{}) error

    // SetAny stores arbitrary data by key
    SetAny(key string, value interface{}) error
}
```

#### Backend Selection

Backends are selected in this priority order:

1. **`ATMOS_KEYRING_TYPE` environment variable** (highest priority)
   - Useful for testing, CI/CD, and user overrides
   - Values: `system`, `file`, `memory`

2. **`atmos.yaml` configuration**:
   ```yaml
   auth:
     keyring:
       type: file  # or system, memory
       spec:
         path: /custom/path/to/keyring  # file backend only
         password_env: MY_KEYRING_PASSWORD  # file backend only
   ```

3. **Default to `system`** (backward compatibility)

#### Fallback Behavior

```go
// If preferred backend fails, fall back to system keyring
if err != nil {
    fmt.Fprintf(os.Stderr, "Warning: failed to create %s keyring (%v), using system keyring\n", keyringType, err)
    store, _ = newSystemKeyringStore()
}
```

### Backend Implementations

#### 1. System Keyring Backend (`type: system`)

**Purpose**: Production use on developer workstations

**Implementation**: Uses `github.com/99designs/keyring` library with OS-native backends:
- **macOS**: Keychain
- **Windows**: Credential Manager
- **Linux**: Secret Service (GNOME Keyring, KWallet)

**Configuration**:
```yaml
auth:
  keyring:
    type: system
```

**Storage**: OS-managed secure storage
**Encryption**: OS-native encryption
**Authentication**: OS-level authentication (biometrics, system password)

**Advantages**:
- Maximum security via OS integration
- No password management needed
- Biometric authentication support (macOS Touch ID, Windows Hello)

**Disadvantages**:
- Requires desktop environment on Linux
- Not suitable for headless servers

#### 2. File Keyring Backend (`type: file`)

**Purpose**: Shared servers, headless environments, custom deployment scenarios

**Implementation**: Encrypted JSON files using `github.com/99designs/keyring` file backend with AES-256-GCM encryption

**XDG Compliance**:
```go
// Default location follows XDG Base Directory Specification
// $XDG_DATA_HOME/atmos/keyring (typically ~/.local/share/atmos/keyring on Linux)
// ~/Library/Application Support/atmos/keyring on macOS
// %LOCALAPPDATA%/atmos/keyring on Windows

// Environment variable precedence:
// 1. ATMOS_XDG_DATA_HOME (Atmos-specific override)
// 2. XDG_DATA_HOME (standard XDG variable)
// 3. XDG library default (via github.com/adrg/xdg)
```

**Configuration**:
```yaml
auth:
  keyring:
    type: file
    spec:
      path: /custom/path/to/keyring  # optional, defaults to XDG location
      password_env: ATMOS_KEYRING_PASSWORD  # optional, defaults to ATMOS_KEYRING_PASSWORD
```

**Password Management**:
```go
// Password resolution order:
// 1. Environment variable (ATMOS_KEYRING_PASSWORD or custom via password_env)
// 2. Interactive prompt (terminal or Charm TUI)
// 3. Error if neither available
```

**Security Features**:
- AES-256-GCM encryption for all files
- Password-based key derivation
- Minimum 8-character password requirement
- File permissions: 0o700 (owner read/write/execute only)

**Advantages**:
- Works in headless/SSH environments
- User-controlled storage location
- No desktop environment required
- Supports custom password sources

**Disadvantages**:
- Requires password management
- User must protect password
- Manual backup responsibility

#### 3. Memory Keyring Backend (`type: memory`)

**Purpose**: CI/CD pipelines, testing, ephemeral workloads

**Implementation**: In-memory map with mutex protection

```go
type memoryKeyringStore struct {
    mu    sync.RWMutex
    items map[string]string // alias -> JSON data
}
```

**Configuration**:
```yaml
auth:
  keyring:
    type: memory
```

**Storage**: RAM only, no persistence
**Encryption**: None (data never written to disk)
**Lifetime**: Process lifetime only

**Advantages**:
- No filesystem dependencies
- Fast (no I/O)
- Perfect for testing
- Automatic cleanup on process exit
- No encryption overhead

**Disadvantages**:
- Lost on process termination
- Not suitable for long-running sessions
- No sharing between processes

**Use Cases**:
- Unit and integration tests
- CI/CD pipelines with short-lived sessions
- Containerized workloads
- Temporary development sessions

### Data Format

#### Credential Envelope

Credentials are wrapped in an envelope with type information for polymorphic storage:

```go
type credentialEnvelope struct {
    Type string          `json:"type"`  // "aws" or "oidc"
    Data json.RawMessage `json:"data"`  // Credential-specific JSON
}
```

**Supported Types**:
```go
const (
    CredentialTypeAWS  = "aws"
    CredentialTypeOIDC = "oidc"
)
```

#### AWS Credentials Format

```json
{
  "type": "aws",
  "data": {
    "access_key_id": "ASIA...",
    "secret_access_key": "...",
    "session_token": "...",
    "region": "us-east-1",
    "expiration": "2025-10-21T12:00:00Z"
  }
}
```

#### OIDC Credentials Format

```json
{
  "type": "oidc",
  "data": {
    "token": "eyJ...",
    "provider": "github",
    "expiration": "2025-10-21T12:00:00Z"
  }
}
```

### Security Considerations

#### File Backend Security

1. **Encryption**: AES-256-GCM for all credential files
2. **File permissions**: 0o700 (owner only)
3. **Password requirements**: Minimum 8 characters
4. **Password storage**: Never stored, only in environment or interactive prompt
5. **Directory permissions**: Restrictive (0o700)

#### System Backend Security

Delegates all security to OS:
- macOS Keychain: Protected by FileVault, Touch ID, system password
- Windows Credential Manager: Protected by DPAPI
- Linux Secret Service: Protected by desktop session unlock

#### Memory Backend Security

- No persistence = no at-rest encryption needed
- Protected by OS process isolation
- Cleared on process termination
- Never written to swap (credential data is short-lived strings)

### Environment Variables

| Variable | Purpose | Default | Backends |
|----------|---------|---------|----------|
| `ATMOS_KEYRING_TYPE` | Select backend | `system` | All |
| `ATMOS_KEYRING_PASSWORD` | File backend password | _(prompt)_ | File only |
| `ATMOS_XDG_DATA_HOME` | Override XDG data directory | _(XDG default)_ | File only |
| `XDG_DATA_HOME` | Standard XDG data directory | `~/.local/share` | File only |

### Performance Instrumentation

All backends instrument I/O-heavy operations with `perf.Track()`:

```go
// Store operations
defer perf.Track(nil, "credentials.fileKeyringStore.Store")()
defer perf.Track(nil, "credentials.systemKeyringStore.Store")()
defer perf.Track(nil, "credentials.memoryKeyringStore.Store")()

// Retrieve operations
defer perf.Track(nil, "credentials.fileKeyringStore.Retrieve")()
// ... etc
```

This enables:
- Performance profiling of credential operations
- Bottleneck identification
- Comparison between backends
- Production monitoring

## Implementation Details

### Error Handling

Follows Atmos error handling strategy (see `docs/prd/error-handling-strategy.md`):

```go
// Static error definitions
var (
    ErrCredentialStore = errors.New("credential store")
    ErrNotSupported = errors.New("not supported")
    ErrPasswordTooShort = errors.New("password must be at least 8 characters")
    ErrUnsupportedCredentialType = errors.New("unsupported credential type")
    ErrCredentialsNotFound = errors.New("credentials not found")
    ErrPasswordRequired = errors.New("keyring password required")
)

// Wrapping errors with context
return fmt.Errorf("%w: %s", ErrCredentialsNotFound, alias)
return errors.Join(ErrCredentialStore, underlyingErr)
```

### XDG Path Resolution

File backend follows XDG Base Directory Specification:

```go
func getDefaultKeyringPath() (string, error) {
    // Bind both ATMOS_XDG_DATA_HOME and XDG_DATA_HOME
    v := viper.New()
    if err := v.BindEnv("XDG_DATA_HOME", "ATMOS_XDG_DATA_HOME", "XDG_DATA_HOME"); err != nil {
        return "", fmt.Errorf("error binding XDG_DATA_HOME: %w", err)
    }

    var dataDir string
    if customDataHome := v.GetString("XDG_DATA_HOME"); customDataHome != "" {
        // Use custom override
        dataDir = filepath.Join(customDataHome, "atmos", "keyring")
    } else {
        // Use XDG library default
        dataDir = filepath.Join(xdg.DataHome, "atmos", "keyring")
    }

    return dataDir, nil
}
```

**Platform Defaults**:
- **Linux**: `~/.local/share/atmos/keyring`
- **macOS**: `~/Library/Application Support/atmos/keyring`
- **Windows**: `%LOCALAPPDATA%\atmos\keyring`

### Testing Strategy

#### Test Isolation

All tests use environment variables for hermetic isolation:

```go
func TestFileKeyring_NewStoreDefaultPath(t *testing.T) {
    tempHome := t.TempDir()
    t.Setenv("HOME", tempHome)
    t.Setenv("XDG_DATA_HOME", filepath.Join(tempHome, ".local", "share"))
    t.Setenv("ATMOS_KEYRING_PASSWORD", "test-password-12345")

    store, err := newFileKeyringStore(nil)
    require.NoError(t, err)

    expectedPath := filepath.Join(tempHome, ".local", "share", "atmos", "keyring")
    assert.Equal(t, expectedPath, store.path)
}
```

#### Backend Testing

Each backend has comprehensive tests:
- **File backend**: 15+ tests covering encryption, persistence, XDG paths
- **System backend**: Tests with fallback to mock when system unavailable
- **Memory backend**: 7+ tests covering concurrency, isolation, lifecycle

#### Mock Generation

System keyring uses mockgen for interface mocking:

```go
//go:generate mockgen -source=interface.go -destination=mock_keyring.go
```

This enables testing without requiring OS keyring availability.

## Configuration Examples

### Production Workstation (macOS/Windows)

```yaml
# atmos.yaml - Use OS keyring (default, can be omitted)
auth:
  keyring:
    type: system
```

### Shared Linux Server

```yaml
# atmos.yaml - Use file backend with custom location
auth:
  keyring:
    type: file
    spec:
      path: /opt/atmos/keyring
      password_env: ATMOS_KEYRING_SECRET
```

```bash
# Set password via environment variable
export ATMOS_KEYRING_SECRET="my-secure-password-12345"
```

### CI/CD Pipeline

```yaml
# .github/workflows/deploy.yml
env:
  ATMOS_KEYRING_TYPE: memory  # Use in-memory backend for CI

steps:
  - name: Authenticate
    run: |
      atmos auth login --identity prod-deployer
      atmos terraform apply vpc -s prod
```

### Development/Testing

```bash
# Use memory backend for fast testing
export ATMOS_KEYRING_TYPE=memory

# Run tests
go test ./...
```

## Migration and Backward Compatibility

### Backward Compatibility

1. **Default to system keyring**: Existing users without configuration continue using OS keyring
2. **Deprecated function supported**: `NewKeyringAuthStore()` still works but logs deprecation warning
3. **Existing credentials preserved**: No migration needed for system keyring users

### Migration Paths

#### From System to File

```bash
# 1. Export credentials (manual, one-time)
atmos auth env > /tmp/creds.env

# 2. Update atmos.yaml
# auth:
#   keyring:
#     type: file

# 3. Re-authenticate
source /tmp/creds.env
# Credentials now in file backend
```

#### From File to System

```bash
# 1. Update atmos.yaml to use system
# 2. Delete file keyring directory
rm -rf ~/.local/share/atmos/keyring
# 3. Re-authenticate with atmos auth login
```

## Open Questions and Future Enhancements

### Potential Future Work

1. **Cloud-based backends**: Support for cloud secret stores (AWS Secrets Manager, GCP Secret Manager, Azure Key Vault)
2. **Multi-backend**: Allow fallback chains (e.g., try system, fall back to file)
3. **Credential migration tool**: Automated migration between backends
4. **Encrypted file sync**: Sync encrypted credentials across machines
5. **Credential sharing**: Team-shared credentials with different encryption keys
6. **Audit logging**: Track credential access for compliance
7. **Expiration notifications**: Proactive warnings before credential expiry
8. **Auto-refresh**: Automatic credential renewal before expiration

### Questions for Discussion

1. Should we support multiple simultaneous backends for different credential types?
2. Should file backend support keyring file rotation/versioning?
3. Should we add a `list` command to show all stored credentials?
4. Should we support credential export/import for backup purposes?

## Success Metrics

1. **Security**: Zero credential leaks, all files encrypted at rest
2. **Reliability**: Graceful fallback when backends unavailable
3. **Performance**: < 100ms for store/retrieve operations
4. **Testing**: 80%+ code coverage on all backends
5. **User Experience**: No breaking changes for existing users

## References

- **XDG Base Directory Specification**: https://specifications.freedesktop.org/basedir-spec/basedir-spec-latest.html
- **99designs/keyring**: https://github.com/99designs/keyring
- **adrg/xdg**: https://github.com/adrg/xdg
- **Error Handling Strategy**: `docs/prd/error-handling-strategy.md`
- **Test Preconditions**: `docs/prd/test-preconditions.md`

## Changelog

| Date | Version | Change |
|------|---------|--------|
| 2025-10-21 | 1.0 | Initial PRD creation (retroactive documentation of implementation) |
