package auth

import (
	"errors"

	"github.com/cloudposse/atmos/pkg/auth/credentials"
	"github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/perf"
)

// The operation store forwards configuration digests with the same realm redirection as the
// credentials they describe, so an operation-scoped target and its digests stay together.
var _ credentials.ConfigDigestStore = (*operationCredentialStore)(nil)

// digestStore returns the wrapped store as a ConfigDigestStore, if it is one.
func (s *operationCredentialStore) digestStore() (credentials.ConfigDigestStore, error) {
	store, ok := s.CredentialStore.(credentials.ConfigDigestStore)
	if !ok {
		return nil, errors.Join(credentials.ErrCredentialStore, credentials.ErrNotSupported)
	}
	return store, nil
}

// SupportsConfigDigests reports whether the wrapped store persists configuration digests.
func (s *operationCredentialStore) SupportsConfigDigests() bool {
	store, err := s.digestStore()
	return err == nil && store.SupportsConfigDigests()
}

// StoreWithConfigDigest forwards to the wrapped store in the alias's operation realm.
func (s *operationCredentialStore) StoreWithConfigDigest(alias string, creds types.ICredentials, realm, digest string) error {
	defer perf.Track(nil, "auth.operationCredentialStore.StoreWithConfigDigest")()

	store, err := s.digestStore()
	if err != nil {
		return err
	}
	return store.StoreWithConfigDigest(alias, creds, s.realmFor(alias, realm), digest)
}

// RetrieveWithConfigDigest forwards to the wrapped store in the alias's operation realm.
func (s *operationCredentialStore) RetrieveWithConfigDigest(alias, realm string) (types.ICredentials, string, error) {
	defer perf.Track(nil, "auth.operationCredentialStore.RetrieveWithConfigDigest")()

	store, err := s.digestStore()
	if err != nil {
		return nil, "", err
	}
	return store.RetrieveWithConfigDigest(alias, s.realmFor(alias, realm))
}

// StoreExternalConfigDigest forwards to the wrapped store in the alias's operation realm.
func (s *operationCredentialStore) StoreExternalConfigDigest(alias, realm string, record credentials.ExternalConfigDigest) error {
	defer perf.Track(nil, "auth.operationCredentialStore.StoreExternalConfigDigest")()

	store, err := s.digestStore()
	if err != nil {
		return err
	}
	return store.StoreExternalConfigDigest(alias, s.realmFor(alias, realm), record)
}

// RetrieveExternalConfigDigest forwards to the wrapped store in the alias's operation realm.
func (s *operationCredentialStore) RetrieveExternalConfigDigest(alias, realm string) (credentials.ExternalConfigDigest, error) {
	defer perf.Track(nil, "auth.operationCredentialStore.RetrieveExternalConfigDigest")()

	store, err := s.digestStore()
	if err != nil {
		return credentials.ExternalConfigDigest{}, err
	}
	return store.RetrieveExternalConfigDigest(alias, s.realmFor(alias, realm))
}
