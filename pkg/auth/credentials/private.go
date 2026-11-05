package credentials

import (
	"fmt"

	"github.com/cloudposse/atmos/pkg/keyring"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// OpenKeyring opens the configured auth backend for private provider sessions.
// Unlike optional credential caching, durable session writes must fail visibly
// when the requested backend cannot be opened; no fallback to noop is allowed.
func OpenKeyring(config *schema.AuthConfig) (keyring.Keyring, error) {
	defer perf.Track(nil, "credentials.OpenKeyring")()

	switch resolveKeyringType(config) {
	case "memory":
		return newMemoryKeyringStore().kr, nil
	case "file":
		store, err := newFileKeyringStore(config)
		if err != nil {
			return nil, err
		}
		return store.kr, nil
	case "system":
		store, err := newSystemKeyringStore()
		if err != nil {
			return nil, err
		}
		return store.kr, nil
	default:
		return nil, fmt.Errorf("%w: private sessions require a configured keyring", ErrCredentialStore)
	}
}

// KeyringType reports the selected backend including the existing environment override.
func KeyringType(config *schema.AuthConfig) string { return resolveKeyringType(config) }
