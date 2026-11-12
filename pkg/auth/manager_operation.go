package auth

import (
	"context"
	"errors"

	"github.com/google/uuid"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth/credentials"
	"github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

type identityOperationScope struct{ target, realm string }

// newOperationManager isolates the target's keyring, files, native private
// session and process cache without changing its name, principal or policy.
// Ancestors keep their existing realms and credential store entries.
func (m *manager) newOperationManager(name string) (*manager, *schema.ConfigAndStacksInfo, error) {
	isolated, info, err := m.newIsolatedManager()
	if err != nil {
		return nil, nil, err
	}
	identity, exists := isolated.identities[name]
	if !exists {
		return nil, nil, errUtils.ErrIdentityNotFound
	}
	scope := &identityOperationScope{target: name, realm: "operation-" + uuid.NewString()}
	storage := &operationCredentialStore{CredentialStore: m.credentialStore, scope: scope}
	isolated.operationScope, isolated.credentialStore = scope, storage
	identity.SetRealm(scope.realm)
	if receiver, ok := identity.(credentialStoreReceiver); ok {
		receiver.SetCredentialStore(storage)
	}
	return isolated, info, nil
}

// identityCredentialRealm selects the private realm only for the operation's target.
func (m *manager) identityCredentialRealm(name string) string {
	if m.operationScope != nil && m.operationScope.target == name {
		return m.operationScope.realm
	}
	return m.realm.Value
}

// releaseOperationIdentity attempts target logout and local credential removal,
// clears the chain cache, and combines cleanup failures without logging out ancestors.
func (m *manager) releaseOperationIdentity(ctx context.Context) error {
	defer perf.Track(nil, "auth.Manager.releaseOperationIdentity")()

	name := m.operationScope.target
	logout := m.identities[name].Logout(ctx)
	removed := m.credentialStore.Delete(name, m.realm.Value)
	if errors.Is(removed, credentials.ErrCredentialsNotFound) {
		removed = nil
	}
	processCredentialCache.Delete(m.chainCacheKey())
	return errors.Join(logout, removed)
}

// operationCredentialStore redirects only the owned target, including legacy
// keyring lookups/deletions, to its unique operation realm.
type operationCredentialStore struct {
	types.CredentialStore
	scope *identityOperationScope
}

// realmFor redirects the target alias to its operation realm, retaining ancestor realms.
func (s *operationCredentialStore) realmFor(alias, realm string) string {
	if alias == s.scope.target {
		return s.scope.realm
	}
	return realm
}

func (s *operationCredentialStore) Store(alias string, value types.ICredentials, realm string) error {
	defer perf.Track(nil, "auth.operationCredentialStore.Store")()

	return s.CredentialStore.Store(alias, value, s.realmFor(alias, realm))
}

func (s *operationCredentialStore) Retrieve(alias, realm string) (types.ICredentials, error) {
	defer perf.Track(nil, "auth.operationCredentialStore.Retrieve")()

	return s.CredentialStore.Retrieve(alias, s.realmFor(alias, realm))
}

func (s *operationCredentialStore) Delete(alias, realm string) error {
	defer perf.Track(nil, "auth.operationCredentialStore.Delete")()

	return s.CredentialStore.Delete(alias, s.realmFor(alias, realm))
}

func (s *operationCredentialStore) IsExpired(alias, realm string) (bool, error) {
	defer perf.Track(nil, "auth.operationCredentialStore.IsExpired")()

	return s.CredentialStore.IsExpired(alias, s.realmFor(alias, realm))
}
