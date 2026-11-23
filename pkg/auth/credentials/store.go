package credentials

import (
	"encoding/json"
	"errors"
	"os"
	"strings"

	"github.com/cloudposse/atmos/pkg/auth/types"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui"
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

	// KeyringRealmPrefix is the prefix used for realm-scoped keyring keys.
	KeyringRealmPrefix = "atmos"

	// KeyringSeparator is the separator used between key components.
	// Uses underscore instead of colon for Windows compatibility (colons not allowed in filenames).
	KeyringSeparator = "_"

	// KeyringRealmAliasSeparator separates the realm from the alias in realm-scoped keys.
	// Realms cannot contain "__" or end with "_" (see realm.Validate), so the first "__"
	// after the prefix always ends the realm, whatever the alias contains.
	KeyringRealmAliasSeparator = "__"

	// logKeyAlias is the structured-log key for a credential alias.
	logKeyAlias = "alias"
)

// buildKeyringKey creates a realm-scoped keyring key.
// Format: "atmos_realm__alias" when realm is provided.
// Falls back to "atmos_alias" for backward compatibility when realm is empty.
func buildKeyringKey(alias, realm string) string {
	log.Debug("Building keyring key", logKeyAlias, alias, "realm", realm)
	if realm != "" {
		return KeyringRealmPrefix + KeyringSeparator + realm + KeyringRealmAliasSeparator + alias
	}
	return KeyringRealmPrefix + KeyringSeparator + alias
}

// legacyRealmKeyringKey returns the previous realm-scoped key "atmos_<realm>_<alias>". Both
// parts may contain "_", so realm "acme" + alias "prod_admin" and realm "acme_prod" + alias
// "admin" share one key; it is read only when the store's legacyKeyIsUnambiguous holds.
func legacyRealmKeyringKey(alias, realm string) string {
	return KeyringRealmPrefix + KeyringSeparator + realm + KeyringSeparator + alias
}

// legacyRealmKeyIsUnambiguous reports whether the previous-layout key for alias and realm
// can only have been written for this exact realm and alias: neither contains "_", so no
// other realm and alias split produces the same key, and the key contains no "__" of the
// current layout.
func legacyRealmKeyIsUnambiguous(alias, realm string) bool {
	return realm != "" && alias != "" && !strings.Contains(realm, KeyringSeparator) && !strings.Contains(alias, KeyringSeparator)
}

// EmptyRealmKeyIsUnambiguous reports whether the empty-realm (pre-realm) key "atmos_<alias>"
// can only belong to alias. An alias containing "_" shares its key with a realm-scoped entry
// of another realm ("prod_admin" is realm "prod" + "admin" in the previous layout, and
// "prod__admin" is realm "prod" + "admin" in the current one), so realm-scoped callers
// must not delete that empty-realm key as their own legacy entry.
func EmptyRealmKeyIsUnambiguous(alias string) bool {
	return alias != "" && !strings.Contains(alias, KeyringSeparator)
}

// credentialEnvelope is used to persist interface credentials with type information.
//
// ConfigDigest is the SHA-256 of the non-secret effective configuration of the chain step that
// produced the credentials (see ConfigDigestStore). It is omitted when empty, so envelopes
// without one are byte-identical to those of older versions, and older versions decode
// envelopes with one unchanged because encoding/json ignores unknown fields.
type credentialEnvelope struct {
	Type         string          `json:"type"`
	Data         json.RawMessage `json:"data"`
	ConfigDigest string          `json:"config_digest,omitempty"`
}

// NewCredentialStore creates a new credential store instance based on configuration.
// It selects the appropriate backend (system, file, or memory) based on:
// 1. ATMOS_KEYRING_TYPE environment variable (highest priority).
// 2. AuthConfig.Keyring.Type configuration.
// 3. Default to "system" for backward compatibility.
//
// Deprecated: this no-argument form cannot honor auth.keyring.type from
// atmos.yaml because it passes a nil config to NewCredentialStoreWithConfig.
// Production code MUST use NewCredentialStoreWithConfig(authConfig) so the
// configured keyring backend is selected (see issue #2544). This form remains
// only for tests and other call sites that genuinely have no auth config.
func NewCredentialStore() types.CredentialStore {
	defer perf.Track(nil, "credentials.NewCredentialStore")()

	return NewCredentialStoreWithConfig(nil)
}

// NewCredentialStoreWithConfig creates a credential store with explicit configuration.
func NewCredentialStoreWithConfig(authConfig *schema.AuthConfig) types.CredentialStore {
	defer perf.Track(nil, "credentials.NewCredentialStoreWithConfig")()

	keyringType := resolveKeyringType(authConfig)

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
		ui.Warningf("unknown keyring type %q, using system keyring", keyringType)
		store, err = newSystemKeyringStore()
	}

	if err != nil {
		// Handle keyring initialization failure.
		if keyringType == "system" {
			// System keyring unavailable (e.g., no dbus in containers).
			// Use no-op keyring to gracefully handle missing keyring.
			// Auth will continue using credentials from files/environment.
			log.Debug("System keyring not available, using no-op keyring (will use credentials from files/environment)", "error", err.Error())
			store = newNoopKeyringStore()
		} else {
			// Non-system keyring failed - try system keyring.
			log.Debug("Keyring type failed, attempting system keyring fallback", "keyringType", keyringType, "error", err.Error())
			store, err = newSystemKeyringStore()
			if err != nil {
				// System keyring also failed - use no-op keyring.
				log.Debug("System keyring not available, using no-op keyring (will use credentials from files/environment)", "error", err.Error())
				store = newNoopKeyringStore()
			}
		}
	}

	// Keep an explicitly configured realm from being mistaken for the auto realm.
	setExplicitRealm(store, authConfig)
	return store
}

func resolveKeyringType(authConfig *schema.AuthConfig) string {
	// Avoid viper.BindEnv here: credential stores are created per auth manager,
	// and Terraform bulk execution can initialize auth managers concurrently.
	//nolint:forbidigo // os.Getenv avoids mutating global Viper state during concurrent auth setup.
	if envType := os.Getenv("ATMOS_KEYRING_TYPE"); envType != "" {
		return envType
	}
	if authConfig != nil && authConfig.Keyring.Type != "" {
		return authConfig.Keyring.Type
	}
	return "system"
}

// NewKeyringAuthStore creates a new system keyring-based auth store (for backward compatibility).
// Deprecated: Use NewCredentialStore() instead.
func NewKeyringAuthStore() types.CredentialStore {
	store, _ := newSystemKeyringStore()
	return store
}
