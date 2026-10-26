package credentials

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/keyring"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/xdg"
)

const (
	// KeyringDirPermissions is the default permission for the keyring directory (owner-only).
	KeyringDirPermissions = 0o700
)

var (
	// ErrUnsupportedCredentialType indicates the credential type is not supported.
	ErrUnsupportedCredentialType = errors.New("unsupported credential type")
	// ErrUnknownCredentialType indicates an unknown credential type was encountered.
	ErrUnknownCredentialType = errors.New("unknown credential type")
	// ErrCredentialsNotFound indicates credentials were not found for the given key.
	ErrCredentialsNotFound = errors.New("credentials not found")
	// ErrDataNotFound indicates data was not found for the given key.
	ErrDataNotFound = errors.New("data not found")
	// ErrPasswordRequired indicates a password is required but not provided.
	ErrPasswordRequired = errors.New("keyring password required")
	// ErrPasswordTooShort indicates the password does not meet the minimum length requirement.
	ErrPasswordTooShort = errors.New("password must be at least 8 characters")
)

// fileKeyringStore is the file-backed credential store. It embeds the generic base and records
// the resolved on-disk directory for tests and diagnostics.
type fileKeyringStore struct {
	*keyringCredentialStore
	path string
}

// newSystemKeyringStore creates a credential store backed by the OS keychain. The probe inside
// keyring.New surfaces an unavailable keyring (e.g. no dbus in a container) as an error so the
// caller can fall back.
func newSystemKeyringStore() (*keyringCredentialStore, error) {
	defer perf.Track(nil, "credentials.newSystemKeyringStore")()

	kr, err := keyring.New(keyring.Config{Type: keyring.TypeSystem, ServiceName: KeyringUser})
	if err != nil {
		return nil, fmt.Errorf("system keyring not available: %w", err)
	}
	return &keyringCredentialStore{kr: kr, storeType: types.CredentialStoreTypeSystemKeyring}, nil
}

// newMemoryKeyringStore creates an in-memory credential store (testing/ephemeral).
func newMemoryKeyringStore() *keyringCredentialStore {
	defer perf.Track(nil, "credentials.newMemoryKeyringStore")()

	// The memory backend never fails to construct.
	kr, _ := keyring.New(keyring.Config{Type: keyring.TypeMemory})
	return &keyringCredentialStore{kr: kr, storeType: types.CredentialStoreTypeMemory}
}

// newNoopKeyringStore creates a no-op credential store for environments without a usable keyring.
func newNoopKeyringStore() *keyringCredentialStore {
	defer perf.Track(nil, "credentials.newNoopKeyringStore")()

	// The noop backend never fails to construct.
	kr, _ := keyring.New(keyring.Config{Type: keyring.TypeNoop})
	return &keyringCredentialStore{kr: kr, storeType: types.CredentialStoreTypeNoop}
}

// newFileKeyringStore creates an encrypted file-based credential store. The directory and
// password-env name come from ATMOS_KEYRING_FILE_PATH / ATMOS_KEYRING_PASSWORD_ENV or auth
// config, defaulting to the XDG data dir and ATMOS_KEYRING_PASSWORD.
func newFileKeyringStore(authConfig *schema.AuthConfig) (*fileKeyringStore, error) {
	defer perf.Track(nil, "credentials.newFileKeyringStore")()

	path, passwordEnv := parseFileKeyringConfig(authConfig)
	if path == "" {
		defaultPath, err := getDefaultKeyringPath()
		if err != nil {
			return nil, errors.Join(ErrCredentialStore, err)
		}
		path = defaultPath
	}

	// keyring expects a directory; if the caller provided a file path, use its parent.
	dir := path
	if filepath.Ext(path) != "" {
		dir = filepath.Dir(path)
	}

	kr, err := keyring.New(keyring.Config{
		Type:        keyring.TypeFile,
		ServiceName: KeyringUser,
		FileDir:     dir,
		PasswordEnv: passwordEnv,
	})
	if err != nil {
		return nil, errors.Join(ErrCredentialStore, err)
	}

	return &fileKeyringStore{
		keyringCredentialStore: &keyringCredentialStore{kr: kr, storeType: types.CredentialStoreTypeFile},
		path:                   dir,
	}, nil
}

// parseFileKeyringConfig extracts the keyring directory and password-env name from the auth
// config, with environment overrides. Priority: env var > config > default.
func parseFileKeyringConfig(authConfig *schema.AuthConfig) (path, passwordEnv string) {
	path = resolveKeyringSetting("ATMOS_KEYRING_FILE_PATH", authConfig, "path")
	passwordEnv = resolveKeyringSetting("ATMOS_KEYRING_PASSWORD_ENV", authConfig, "password_env")
	return path, passwordEnv
}

// resolveKeyringSetting returns the value of envVar, falling back to authConfig.Keyring.Spec[specKey].
func resolveKeyringSetting(envVar string, authConfig *schema.AuthConfig, specKey string) string {
	// Use os.Getenv directly to avoid viper singleton caching in tests.
	//nolint:forbidigo // os.Getenv required here to avoid viper singleton caching in tests.
	if v := os.Getenv(envVar); v != "" {
		return v
	}
	if authConfig != nil && authConfig.Keyring.Spec != nil {
		if s, ok := authConfig.Keyring.Spec[specKey].(string); ok && s != "" {
			return s
		}
	}
	return ""
}

// getDefaultKeyringPath returns the default keyring directory under the XDG data dir. It respects
// ATMOS_XDG_DATA_HOME and XDG_DATA_HOME.
func getDefaultKeyringPath() (string, error) {
	return xdg.GetXDGDataDir("keyring", KeyringDirPermissions)
}
