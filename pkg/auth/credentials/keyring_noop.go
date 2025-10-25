package credentials

import (
	"github.com/cloudposse/atmos/pkg/auth/types"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
)

// noopKeyringStore implements a credential store for containerized environments.
// When system keyring is unavailable (e.g., no dbus in containers),
// this store signals that credentials are managed externally via files and environment variables.
// It doesn't store, validate, or cache credentials - those operations happen via the auth system.
type noopKeyringStore struct{}

// newNoopKeyringStore creates a new no-op keyring store.
func newNoopKeyringStore() *noopKeyringStore {
	return &noopKeyringStore{}
}

// Store always succeeds but doesn't persist anything.
// This allows the auth flow to continue even when keyring is unavailable.
func (s *noopKeyringStore) Store(alias string, creds types.ICredentials) error {
	defer perf.Track(nil, "credentials.noopKeyringStore.Store")()

	// No-op: credentials won't be persisted to keyring.
	// They will still be written to credentials files by the auth system.
	return nil
}

// Retrieve returns ErrCredentialsNotFound without validation.
// In containerized environments, credentials are managed externally (via mounted files
// and environment variables set by the auth system). The noop keyring doesn't store or
// validate credentials - it simply signals that credentials should come from the environment.
func (s *noopKeyringStore) Retrieve(alias string) (types.ICredentials, error) {
	defer perf.Track(nil, "credentials.noopKeyringStore.Retrieve")()

	log.Debug("Noop keyring: credentials managed externally", "alias", alias)

	// Return "not found" - signals that credentials should come from AWS SDK/environment.
	// No validation is performed because:
	// 1. We don't have the context (provider name, file paths) needed to validate
	// 2. Validation happens later when environment variables are properly set
	// 3. The noop keyring is for environments where credentials are externally managed
	return nil, ErrCredentialsNotFound
}

// Delete always succeeds (no-op).
func (s *noopKeyringStore) Delete(alias string) error {
	defer perf.Track(nil, "credentials.noopKeyringStore.Delete")()

	return nil
}

// List returns empty list (no credentials stored).
func (s *noopKeyringStore) List() ([]string, error) {
	defer perf.Track(nil, "credentials.noopKeyringStore.List")()

	return []string{}, nil
}

// IsExpired always returns true (no credentials available).
func (s *noopKeyringStore) IsExpired(alias string) (bool, error) {
	defer perf.Track(nil, "credentials.noopKeyringStore.IsExpired")()

	return true, ErrCredentialsNotFound
}

// Type returns the type of this credential store.
func (s *noopKeyringStore) Type() string {
	return "noop"
}

// GetAny always returns "not found" error.
func (s *noopKeyringStore) GetAny(key string, dest interface{}) error {
	defer perf.Track(nil, "credentials.noopKeyringStore.GetAny")()

	return ErrCredentialsNotFound
}

// SetAny always succeeds (no-op).
func (s *noopKeyringStore) SetAny(key string, value interface{}) error {
	defer perf.Track(nil, "credentials.noopKeyringStore.SetAny")()

	return nil
}
