package providers

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/store"
	"github.com/cloudposse/atmos/pkg/store/authbridge"
)

func TestVaultStoreAuthenticatesEveryOperationBeforeAccess(t *testing.T) {
	operations := map[string]func(*VaultStore) error{
		"set":       func(s *VaultStore) error { return s.Set("dev", "app", "key", "value") },
		"get":       func(s *VaultStore) error { _, err := s.Get("dev", "app", "key"); return err },
		"get-key":   func(s *VaultStore) error { _, err := s.GetKey("key"); return err },
		"delete":    func(s *VaultStore) error { return s.Delete("dev", "app", "key") },
		"has":       func(s *VaultStore) error { _, err := s.Has("dev", "app", "key"); return err },
		"keys":      func(s *VaultStore) error { _, err := s.Keys("dev", "app"); return err },
		"field":     func(s *VaultStore) error { _, err := s.GetField("dev", "app", "key", "token"); return err },
		"key-field": func(s *VaultStore) error { _, err := s.GetKeyField("key", "token"); return err },
	}
	for name, operation := range operations {
		t.Run(name, func(t *testing.T) {
			fake := newFakeVaultKV()
			fake.data["/dev/app/key"] = map[string]any{"token": "scoped", vaultValueKey: "legacy"}
			fake.data["key"] = map[string]any{"token": "direct", vaultValueKey: "legacy"}
			s := newTestVaultStore(fake)
			s.SetAuthContext(nil, "openbao")
			require.ErrorIs(t, operation(s), store.ErrIdentityNotConfigured)
			assert.Zero(t, fake.getCalls)
			assert.Zero(t, fake.metaCalls)
			s.SetAuthContext(authbridge.NewResolvedContext(&schema.AuthContext{
				Environment: map[string]string{"VAULT_TOKEN": "identity-token"},
			}), "openbao")
			require.NoError(t, operation(s))
			assert.Equal(t, "identity-token", fake.token)
		})
	}
}

func TestVaultStoreRejectsUnavailableEnvironmentResolver(t *testing.T) {
	t.Run("resolver without environment capability", func(t *testing.T) {
		s := newTestVaultStore(newFakeVaultKV())
		s.SetAuthContext(store.NewMockAuthContextResolver(gomock.NewController(t)), "openbao")
		_, err := s.GetKey("key")
		require.ErrorIs(t, err, store.ErrAuthContextNotAvailable)
	})
	t.Run("environment authentication failure", func(t *testing.T) {
		s := newTestVaultStore(newFakeVaultKV())
		resolver := store.NewMockEnvironmentAuthContextResolver(gomock.NewController(t))
		resolver.EXPECT().ResolveEnvironmentAuthContext(gomock.Any(), "openbao").Return(nil, assert.AnError)
		s.SetAuthContext(resolver, "openbao")
		_, err := s.GetKey("key")
		require.ErrorIs(t, err, store.ErrAuthContextNotAvailable)
		require.ErrorIs(t, err, assert.AnError)
	})
}

func TestVaultStoreExactFieldValidationAndLookupErrors(t *testing.T) {
	s := newTestVaultStore(newFakeVaultKV())
	for _, lookup := range []func() (any, error){
		func() (any, error) { return s.GetField("dev", "app", "", "token") },
		func() (any, error) { return s.GetKeyField("", "token") },
	} {
		_, err := lookup()
		require.ErrorIs(t, err, store.ErrEmptyKey)
	}
	_, err := s.GetField("dev", "app", "key", "")
	require.ErrorIs(t, err, store.ErrEmptyField)
	s.stackDelimiter = nil
	_, err = s.GetField("dev", "app", "key", "token")
	require.ErrorIs(t, err, store.ErrGetKey)
	require.ErrorContains(t, err, store.ErrStackDelimiterNotSet.Error())
	fake := newFakeVaultKV()
	s = newTestVaultStore(fake)
	s.prefix = "platform/"
	fake.data["platform/service/credentials"] = map[string]any{"token": "selected"}
	value, err := s.GetKeyField("/service/credentials", "token")
	require.NoError(t, err)
	assert.Equal(t, "selected", value)
	fake.getErr = assert.AnError
	_, err = s.GetKeyField("service/credentials", "token")
	require.ErrorIs(t, err, store.ErrVaultRead)
	require.ErrorIs(t, err, assert.AnError)
}
