package credentials

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"

	"github.com/spf13/viper"

	"github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// ErrCredentialStore is the static sentinel for credential-store failures.
var ErrCredentialStore = errors.New("credential store")

// ErrNotSupported indicates an unsupported operation for this backend.
var ErrNotSupported = errors.New("not supported")

// ErrListNotSupported indicates that listing credentials is not supported with keyring backend.
var ErrListNotSupported = errors.New("listing credentials is not supported with keyring backend")

const (
	// KeyringUser is the "account" used to store credentials in the keyring. Here we use atmos-auth to provide a consistent way to search for atmos credentials.
	KeyringUser = "atmos-auth"
)

// credentialEnvelope is used to persist interface credentials with type information.
type credentialEnvelope struct {
	Type string          `json:"type"`
	Data json.RawMessage `json:"data"`
}

// NewCredentialStore creates a new credential store instance based on configuration.
// It selects the appropriate backend (system, file, or memory) based on:
// 1. ATMOS_KEYRING_TYPE environment variable (highest priority).
// 2. AuthConfig.Keyring.Type configuration.
// 3. Default to "system" for backward compatibility.
func NewCredentialStore() types.CredentialStore {
	defer perf.Track(nil, "credentials.NewCredentialStore")()

	return NewCredentialStoreWithConfig(nil)
}

// NewCredentialStoreWithConfig creates a credential store with explicit configuration.
func NewCredentialStoreWithConfig(authConfig *schema.AuthConfig) types.CredentialStore {
	defer perf.Track(nil, "credentials.NewCredentialStoreWithConfig")()

	keyringType := "system" // Default for backward compatibility.

	// Bind environment variable.
	_ = viper.BindEnv("atmos_keyring_type", "ATMOS_KEYRING_TYPE")

	// Check environment variable first (for testing and CI).
	if envType := viper.GetString("atmos_keyring_type"); envType != "" {
		keyringType = envType
	} else if authConfig != nil && authConfig.Keyring.Type != "" {
		// Use configuration if provided.
		keyringType = authConfig.Keyring.Type
	}

	var store types.CredentialStore
	var err error

	switch keyringType {
	case "memory":
		store = newMemoryKeyringStore()
	case "file":
		store, err = newFileKeyringStore(authConfig)
	case "system":
		store, err = newSystemKeyringStore()
	default:
		// Log warning about unknown type and fall back to system
		fmt.Fprintf(os.Stderr, "Warning: unknown keyring type %q, using system keyring\n", keyringType)
		store, err = newSystemKeyringStore()
	}

	if err != nil {
		// Fall back to system keyring on error
		fmt.Fprintf(os.Stderr, "Warning: failed to create %s keyring (%v), using system keyring\n", keyringType, err)
		store, _ = newSystemKeyringStore()
	}

	return store
}

// NewKeyringAuthStore creates a new system keyring-based auth store (for backward compatibility).
// Deprecated: Use NewCredentialStore() instead.
func NewKeyringAuthStore() types.CredentialStore {
	store, _ := newSystemKeyringStore()
	return store
}
