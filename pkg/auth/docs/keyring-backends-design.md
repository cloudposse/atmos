# Keyring Backends Design

## Overview

This document describes the design for pluggable keyring backends in Atmos Auth. The implementation supports multiple storage backends for credentials:

- **System Keyring** (default) - Uses OS-native credential storage via Zalando go-keyring
- **File Keyring** - Encrypted file-based storage via 99designs keyring with interactive password prompting
- **Memory Keyring** - In-memory storage for testing (no persistence)

## Goals

1. **Backward Compatibility**: Existing configurations default to system keyring
2. **Testing Support**: Memory keyring enables fast, isolated testing without system dependencies
3. **Portability**: File keyring works across all platforms without OS keyring requirements
4. **Security**: File keyring uses encryption with interactive password prompting
5. **User Experience**: Seamless integration with Charm Bracelet for password input

## Architecture

### Configuration Schema

```yaml
# atmos.yaml
auth:
  keyring:
    type: system  # Options: "system" | "file" | "memory"
    spec:
      # File keyring specific options
      path: ~/.atmos/keyring       # Default: ~/.atmos/keyring
      password_env: ATMOS_KEYRING_PASSWORD  # Optional: env var for password
```

### Keyring Types

#### 1. System Keyring (Default)

**Backend**: Zalando `go-keyring` (existing implementation)

**Features**:
- Uses OS-native secure storage (macOS Keychain, Windows Credential Manager, Linux Secret Service)
- No additional configuration required
- Best for single-user workstations
- Cannot list all stored credentials (API limitation)

**Configuration**:
```yaml
auth:
  keyring:
    type: system  # Or omit entirely for default
```

**Implementation**: `keyring_system.go`

#### 2. File Keyring

**Backend**: 99designs `keyring` with file backend

**Features**:
- Encrypted file storage
- Cross-platform compatibility
- Interactive password prompting via Charm Bracelet `huh`
- Password can be provided via environment variable for automation
- Supports listing all stored credentials

**Configuration**:
```yaml
auth:
  keyring:
    type: file
    spec:
      path: ~/.atmos/keyring  # Optional: custom path
      password_env: ATMOS_KEYRING_PASSWORD  # Optional: env var name
```

**Password Resolution Order**:
1. Environment variable (if `password_env` specified)
2. Interactive prompt using `charmbracelet/huh` (if TTY available)
3. Error if neither available

**Implementation**: `keyring_file.go`

**Interactive Password Prompt**:
```go
import "github.com/charmbracelet/huh"

func promptForPassword(prompt string) (string, error) {
    var password string
    err := huh.NewForm(
        huh.NewGroup(
            huh.NewInput().
                Title(prompt).
                EchoMode(huh.EchoModePassword).
                Value(&password).
                Validate(func(s string) error {
                    if len(s) < 8 {
                        return errors.New("password must be at least 8 characters")
                    }
                    return nil
                }),
        ),
    ).Run()
    return password, err
}
```

#### 3. Memory Keyring

**Backend**: In-memory map

**Features**:
- No persistence (data lost when process exits)
- No external dependencies
- Fast and isolated
- Perfect for unit and integration tests
- Supports full CRUD operations including list

**Configuration**:
```yaml
auth:
  keyring:
    type: memory
```

**Use Cases**:
- Unit tests for auth components
- Integration tests without system keyring access
- CI/CD pipelines where system keyring is unavailable
- Testing keyring migration logic
- Ephemeral credential storage

**Implementation**: `keyring_memory.go`

```go
type memoryKeyringStore struct {
    mu    sync.RWMutex
    items map[string]string  // alias -> JSON data
}

func (s *memoryKeyringStore) Store(alias string, creds types.ICredentials) error {
    s.mu.Lock()
    defer s.mu.Unlock()

    // Serialize and store in memory
    data, err := marshalCredentials(creds)
    if err != nil {
        return err
    }

    s.items[alias] = string(data)
    return nil
}

func (s *memoryKeyringStore) List() ([]string, error) {
    s.mu.RLock()
    defer s.mu.RUnlock()

    aliases := make([]string, 0, len(s.items))
    for alias := range s.items {
        aliases = append(aliases, alias)
    }
    return aliases, nil
}
```

### Factory Pattern

**File**: `pkg/auth/credentials/store.go`

```go
package credentials

import (
    "fmt"
    "os"

    "github.com/cloudposse/atmos/pkg/auth/types"
    "github.com/cloudposse/atmos/pkg/schema"
)

// NewCredentialStore creates a credential store based on auth configuration.
func NewCredentialStore(authConfig *schema.AuthConfig) (types.CredentialStore, error) {
    keyringType := "system" // Default for backward compatibility

    if authConfig != nil && authConfig.Keyring.Type != "" {
        keyringType = authConfig.Keyring.Type
    }

    // Override with environment variable for testing
    if envType := os.Getenv("ATMOS_KEYRING_TYPE"); envType != "" {
        keyringType = envType
    }

    switch keyringType {
    case "system":
        return newSystemKeyringStore()
    case "file":
        return newFileKeyringStore(authConfig)
    case "memory":
        return newMemoryKeyringStore()
    default:
        return nil, fmt.Errorf("unsupported keyring type: %s", keyringType)
    }
}
```

### File Structure

```
pkg/auth/credentials/
├── store.go              # CredentialStore interface & factory
├── keyring_system.go     # System keyring (Zalando go-keyring)
├── keyring_file.go       # File keyring (99designs keyring + Charm Bracelet)
├── keyring_memory.go     # Memory keyring (in-memory map)
├── keyring_common.go     # Shared utilities (marshaling, validation)
├── store_test.go         # Factory and interface tests
├── keyring_system_test.go
├── keyring_file_test.go
└── keyring_memory_test.go
```

## Implementation Details

### Schema Changes

**File**: `pkg/schema/schema_auth.go`

```go
// AuthConfig defines the authentication configuration structure.
type AuthConfig struct {
    Logs       Logs                `yaml:"logs,omitempty" json:"logs,omitempty" mapstructure:"logs"`
    Keyring    KeyringConfig       `yaml:"keyring,omitempty" json:"keyring,omitempty" mapstructure:"keyring"`
    Providers  map[string]Provider `yaml:"providers" json:"providers" mapstructure:"providers"`
    Identities map[string]Identity `yaml:"identities" json:"identities" mapstructure:"identities"`
}

// KeyringConfig defines keyring backend configuration.
type KeyringConfig struct {
    Type string                 `yaml:"type,omitempty" json:"type,omitempty" mapstructure:"type"` // "system", "file", or "memory"
    Spec map[string]interface{} `yaml:"spec,omitempty" json:"spec,omitempty" mapstructure:"spec"` // Type-specific config
}
```

### Password Management for File Keyring

**Resolution Order**:

1. **Environment Variable** (highest priority):
   ```bash
   export ATMOS_KEYRING_PASSWORD="my-secure-password"
   ```

2. **Interactive Prompt** (if TTY available):
   - Uses `charmbracelet/huh` for secure password input
   - Password is never echoed to terminal
   - Minimum length validation (8 characters)
   - Cached in memory for session duration

3. **Error** (if neither available):
   - Useful error message explaining how to set password
   - Suggests environment variable or running in interactive mode

**Implementation**:
```go
func getFileKeyringPassword(spec map[string]interface{}) (string, error) {
    // 1. Check environment variable
    if passwordEnv := getStringFromSpec(spec, "password_env", "ATMOS_KEYRING_PASSWORD"); passwordEnv != "" {
        if password := os.Getenv(passwordEnv); password != "" {
            return password, nil
        }
    }

    // 2. Interactive prompt if TTY
    if isatty.IsTerminal(os.Stdin.Fd()) {
        return promptForPassword("Enter keyring password:")
    }

    // 3. Error
    return "", errors.New("keyring password required: set ATMOS_KEYRING_PASSWORD or run in interactive mode")
}
```

### Testing Strategy

#### Unit Tests

**Memory Keyring**:
- Test all CRUD operations
- Test concurrent access (thread safety)
- Test credential expiration checking
- Test List() functionality

**File Keyring**:
- Test with fixed password (via environment variable)
- Test file creation and permissions
- Test encryption/decryption
- Test password validation
- Mock TTY for interactive prompt testing

**System Keyring**:
- Test existing functionality
- Test List() returns appropriate error

**Factory**:
- Test correct backend selection based on config
- Test environment variable override
- Test default to system keyring

#### Integration Tests

```go
func TestAuthWithMemoryKeyring(t *testing.T) {
    t.Setenv("ATMOS_KEYRING_TYPE", "memory")

    // Run auth flow
    // Verify credentials stored and retrieved
    // Verify no persistence after process exit
}

func TestAuthWithFileKeyring(t *testing.T) {
    t.Setenv("ATMOS_KEYRING_TYPE", "file")
    t.Setenv("ATMOS_KEYRING_PASSWORD", "test-password")

    // Run auth flow
    // Verify encrypted file created
    // Verify credentials persist across process restarts
}
```

## Migration Path

### Backward Compatibility

1. **No Configuration Changes**:
   - Existing `atmos.yaml` files without `auth.keyring` section continue to work
   - Default to `type: system` automatically
   - Zero breaking changes

2. **Opt-In to New Backends**:
   - Users explicitly choose file or memory keyring
   - Clear documentation on when to use each type

### Future Enhancements

1. **Migration Command**:
   ```bash
   atmos auth keyring migrate --from system --to file
   ```

2. **Additional Backends**:
   - HashiCorp Vault integration
   - AWS Secrets Manager
   - Azure Key Vault (separate from credential storage)
   - Google Secret Manager

3. **Advanced Features**:
   - Keyring rotation
   - Multi-factor authentication for file keyring
   - Shared team keyrings (encrypted with team key)

## Security Considerations

### System Keyring
- Relies on OS security
- Credentials stored in OS-native secure storage
- Access controlled by OS permissions

### File Keyring
- File permissions: 0600 (user read/write only)
- AES-256 encryption via 99designs keyring
- Password never logged or exposed
- Password not stored on disk (must be provided each time)
- Secure password prompting via Charm Bracelet

### Memory Keyring
- **NOT for production use**
- No encryption (plain text in memory)
- No persistence (ephemeral)
- Suitable only for testing

## Dependencies

### New Dependencies

```go
// go.mod
require (
    github.com/99designs/keyring v1.x.x  // File keyring backend
    // charmbracelet/huh already exists for password prompting
)
```

### Existing Dependencies

- `github.com/zalando/go-keyring` - System keyring (keep for backward compatibility)
- `github.com/charmbracelet/huh` - Password prompting (already in use)

## Documentation Updates

### User-Facing Documentation

**File**: `website/docs/cli/commands/auth/usage.mdx`

Add section:
```markdown
## Keyring Configuration

Atmos supports multiple credential storage backends:

### System Keyring (Default)

Uses your operating system's secure credential storage.

```yaml
auth:
  keyring:
    type: system
```

### File Keyring

Stores credentials in an encrypted file.

```yaml
auth:
  keyring:
    type: file
    spec:
      path: ~/.atmos/keyring  # Optional custom path
```

You'll be prompted for a password when accessing credentials.
For automation, set `ATMOS_KEYRING_PASSWORD` environment variable.

### Memory Keyring (Testing Only)

Stores credentials in memory. Not persistent.

```yaml
auth:
  keyring:
    type: memory
```
```

### Developer Documentation

**File**: `pkg/auth/docs/ARCHITECTURE.md`

Add section on keyring backends and factory pattern.

## Examples

### Development/Testing

```yaml
# atmos.yaml for local development
auth:
  keyring:
    type: memory  # Fast, no setup required
  providers:
    # ...
```

### CI/CD Pipeline

```bash
# .github/workflows/test.yml
env:
  ATMOS_KEYRING_TYPE: memory

- name: Run Atmos Auth Tests
  run: go test ./pkg/auth/...
```

### Shared Team Environment

```yaml
# atmos.yaml for shared server
auth:
  keyring:
    type: file
    spec:
      path: /etc/atmos/keyring
      password_env: ATMOS_KEYRING_PASSWORD
```

### Personal Workstation

```yaml
# atmos.yaml (default)
auth:
  # No keyring config = uses system keyring
  providers:
    # ...
```

## Current State: Tests with Zalando MockInit

The existing tests in `pkg/auth/credentials/store_test.go` already use `keyring.MockInit()` to provide an in-memory mock:

```go
// Ensure the keyring uses an in-memory mock backend for tests.
func init() {
	keyring.MockInit()
}
```

This works well for unit tests but has limitations:
- Global state (all tests share the mock)
- Cannot test real keyring backends
- Limited to Zalando's mock implementation

## Disabled Integration Tests

### Current Disabled Tests

**`cmd/auth_integration_test.go`** - Line 20-22:
```go
func TestAuthCLIIntegrationWithCloudProvider(t *testing.T) {
    // Skip integration tests in CI or if no auth config is available
    if os.Getenv("CI") != "" {
        t.Skipf("Skipping integration tests in CI environment.")
    }
    // ...
}
```

**Reason**: Relies on system keyring which may not be available in CI environments.

### Re-enablement Strategy

Once memory/file keyring is implemented:

1. **Remove CI skip** - Use memory keyring in CI:
   ```go
   func TestAuthCLIIntegrationWithCloudProvider(t *testing.T) {
       // Use memory keyring for CI, system keyring otherwise
       if os.Getenv("CI") != "" {
           t.Setenv("ATMOS_KEYRING_TYPE", "memory")
       }
       // ... test continues without skip
   }
   ```

2. **Add file keyring tests** - Test with real file backend:
   ```go
   func TestAuthCLIIntegrationWithFileKeyring(t *testing.T) {
       tempDir := t.TempDir()
       t.Setenv("ATMOS_KEYRING_TYPE", "file")
       t.Setenv("ATMOS_KEYRING_PATH", tempDir)
       t.Setenv("ATMOS_KEYRING_PASSWORD", "test-password")
       // ... test with file persistence
   }
   ```

3. **Parallel backend tests** - Test all backends in table-driven tests:
   ```go
   func TestAuthWithDifferentBackends(t *testing.T) {
       tests := []struct {
           name       string
           keyringType string
           setup      func(*testing.T)
       }{
           {"memory", "memory", func(t *testing.T) {}},
           {"file", "file", func(t *testing.T) {
               t.Setenv("ATMOS_KEYRING_PASSWORD", "test")
           }},
           // system keyring only on developer machines
       }
       // ...
   }
   ```

## Implementation Checklist

- [ ] Identify all disabled auth integration tests that depend on keyring
- [ ] Update `pkg/schema/schema_auth.go` with `KeyringConfig`
- [ ] Create `pkg/auth/credentials/keyring_system.go` (refactor existing)
- [ ] Create `pkg/auth/credentials/keyring_file.go` (99designs + Charm)
- [ ] Create `pkg/auth/credentials/keyring_memory.go` (in-memory map)
- [ ] Create `pkg/auth/credentials/keyring_common.go` (shared utilities)
- [ ] Update `pkg/auth/credentials/store.go` (add factory)
- [ ] Add unit tests for all backends
- [ ] Add integration tests for all backends
- [ ] Re-enable `cmd/auth_integration_test.go` with memory keyring in CI
- [ ] Add file keyring persistence tests
- [ ] Update JSON schemas
- [ ] Update `pkg/auth/docs/ARCHITECTURE.md`
- [ ] Update `website/docs/cli/commands/auth/usage.mdx`
- [ ] Add blog post about keyring backends
- [ ] Update CI workflows to set `ATMOS_KEYRING_TYPE=memory` for auth tests

## Questions & Decisions

### Q: Should file keyring password be stored in system keyring?

**Decision**: No. This creates a circular dependency. User provides password each time or sets environment variable.

### Q: Should memory keyring warn users it's not secure?

**Decision**: Yes. Log warning when memory keyring is used outside of test environments. Check for `ATMOS_TEST_SKIP_PRECONDITION_CHECKS` or similar test indicators.

### Q: Should we support keyring auto-detection?

**Decision**: Not in initial implementation. Explicit configuration is clearer and more predictable.

### Q: Should file keyring use different password for each user/identity?

**Decision**: No. Single password per keyring file keeps it simple. Future enhancement could add per-identity encryption.

## Success Criteria

1. ✅ All existing auth tests pass with system keyring (backward compatibility)
2. ✅ Memory keyring enables testing without OS keyring dependencies
3. ✅ File keyring works on all platforms (Linux, macOS, Windows)
4. ✅ Interactive password prompt uses Charm Bracelet (consistent UX)
5. ✅ CI/CD pipelines can use memory keyring for fast, isolated tests
6. ✅ Documentation clearly explains when to use each backend
7. ✅ Zero breaking changes for existing users
