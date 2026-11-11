package auth

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/auth/credentials"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/xdg"
)

func TestScopedAuthenticationPreservesParentAndRemovesOwnedCredentials(t *testing.T) {
	for _, fail := range []bool{false, true} {
		t.Run(map[bool]string{false: "success", true: "canceled operation"}[fail], func(t *testing.T) {
			parent := isolatedTestManager(t)
			_, err := parent.Authenticate(t.Context(), "deployer")
			require.NoError(t, err)
			before := snapshotParent(parent)
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			called := false
			err = parent.(types.ScopedAuthManager).WithIsolatedIdentity(ctx, "automation", func(_ context.Context, auth *schema.AuthContext) error {
				called = true
				require.NotNil(t, auth.AWS)
				assert.Equal(t, "automation", auth.AWS.Profile)
				if fail {
					cancel()
					return context.Canceled
				}
				return nil
			})
			if fail {
				require.ErrorIs(t, err, context.Canceled)
			} else {
				require.NoError(t, err)
			}
			require.True(t, called)
			assertParentUnchanged(t, parent, before)
			m := parent.(*manager)
			_, err = m.credentialStore.Retrieve("automation", m.realm.Value)
			require.Error(t, err)
		})
	}
}

func TestScopedCleanupPreservesAncestorsAndProvider(t *testing.T) {
	ctrl := gomock.NewController(t)
	provider := types.NewMockProvider(ctrl)
	first, second := types.NewMockIdentity(ctrl), types.NewMockIdentity(ctrl)
	credentialStore := types.NewMockCredentialStore(ctrl)
	scope := &identityOperationScope{target: "second", realm: "operation-test"}
	m := &manager{
		config: &schema.AuthConfig{
			Providers: map[string]schema.Provider{"root": {Kind: "mock/aws"}},
			Identities: map[string]schema.Identity{
				"first":  {Kind: "mock/aws", Via: &schema.IdentityVia{Provider: "root"}},
				"second": {Kind: "mock/aws", Via: &schema.IdentityVia{Identity: "first"}},
			},
		},
		providers:       map[string]types.Provider{"root": provider},
		identities:      map[string]types.Identity{"first": first, "second": second},
		credentialStore: &operationCredentialStore{CredentialStore: credentialStore, scope: scope},
		operationScope:  scope,
		chain:           []string{"root", "first", "second"},
	}
	credentialStore.EXPECT().Delete("second", "operation-test").Return(nil)
	second.EXPECT().Logout(gomock.Any()).Return(assert.AnError)
	err := m.releaseOperationIdentity(t.Context())
	require.ErrorIs(t, err, assert.AnError)
	// No ancestor/provider logout expectation: either would fail this test.
}

func TestScopedCleanupToleratesAbsentKeyringAndPreservesFailures(t *testing.T) {
	notFound := errors.Join(credentials.ErrCredentialStore, credentials.ErrCredentialsNotFound)
	for _, tt := range []struct {
		name                    string
		deletion, logout, wantErr error
	}{
		{name: "no keyring entry", deletion: notFound},
		{name: "logout failure", deletion: notFound, logout: assert.AnError, wantErr: assert.AnError},
		{name: "deletion failure", deletion: assert.AnError, wantErr: assert.AnError},
		{name: "existing keyring entry"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			identity, storage := types.NewMockIdentity(ctrl), types.NewMockCredentialStore(ctrl)
			scope := &identityOperationScope{target: "owned", realm: "operation-test"}
			m := &manager{
				identities: map[string]types.Identity{"owned": identity},
				credentialStore: &operationCredentialStore{CredentialStore: storage, scope: scope},
				operationScope: scope,
				chain: []string{"root", "owned"},
			}
			key := m.chainCacheKey()
			processCredentialCache.Store(key, &processCachedCreds{})
			t.Cleanup(func() { processCredentialCache.Delete(key) })
			storage.EXPECT().Delete("owned", scope.realm).Return(tt.deletion)
			identity.EXPECT().Logout(gomock.Any()).Return(tt.logout)
			err := m.releaseOperationIdentity(t.Context())
			if tt.wantErr == nil {
				require.NoError(t, err)
			} else {
				require.ErrorIs(t, err, tt.wantErr)
			}
			require.NotErrorIs(t, err, credentials.ErrCredentialsNotFound)
			_, cached := processCredentialCache.Load(key)
			require.False(t, cached, "cleanup must discard its operation cache even on failure")
		})
	}
}

func TestScopedAuthenticationKeepsExistingTargetFilesAndKeyring(t *testing.T) {
	resetProcessCredentialCache()
	t.Cleanup(resetProcessCredentialCache)
	parent := isolatedTestManager(t).(*manager)
	_, err := parent.Authenticate(t.Context(), "automation")
	require.NoError(t, err)
	before := snapshotParent(parent)
	// Mock AWS session credentials normally use files rather than the keyring.
	// Seed an existing keyring session too, so both storage boundaries are checked.
	previous := &types.AWSCredentials{AccessKeyID: "synthetic-previous-session", SecretAccessKey: "synthetic-previous-secret", Expiration: time.Now().Add(time.Hour).Format(time.RFC3339)}
	require.NoError(t, parent.credentialStore.Store("automation", previous, parent.realm.Value))
	require.NoError(t, parent.credentialStore.Store("mock", previous, ""))
	legacyPath := filepath.Join(xdg.LookupXDGConfigDir(""), "aws", "legacy-provider", "credentials")
	require.NoError(t, os.MkdirAll(filepath.Dir(legacyPath), 0o700))
	require.NoError(t, os.WriteFile(legacyPath, []byte("synthetic-legacy-session"), 0o600))
	cached, err := parent.credentialStore.Retrieve("automation", parent.realm.Value)
	require.NoError(t, err)
	originalPath := parent.stackInfo.AuthContext.AWS.CredentialsFile
	original, err := os.ReadFile(originalPath)
	require.NoError(t, err)
	err = parent.WithIsolatedIdentity(t.Context(), "automation", func(_ context.Context, auth *schema.AuthContext) error {
		require.NotEqual(t, originalPath, auth.AWS.CredentialsFile)
		_, err := os.Stat(auth.AWS.CredentialsFile)
		require.NoError(t, err)
		return nil
	})
	require.NoError(t, err)
	preserved, err := parent.credentialStore.Retrieve("automation", parent.realm.Value)
	require.NoError(t, err)
	assert.Equal(t, cached, preserved)
	after, err := os.ReadFile(originalPath)
	require.NoError(t, err)
	assert.Equal(t, original, after)
	_, err = parent.credentialStore.Retrieve("mock", "")
	require.NoError(t, err, "legacy ancestor sessions must survive")
	legacy, err := os.ReadFile(legacyPath)
	require.NoError(t, err)
	assert.Equal(t, "synthetic-legacy-session", string(legacy))
	assertParentUnchanged(t, parent, before)
}

func TestScopedTargetBypassesProcessCredentialReuse(t *testing.T) {
	resetProcessCredentialCache()
	t.Cleanup(resetProcessCredentialCache)
	parent := isolatedTestManager(t).(*manager)
	_, err := parent.Authenticate(t.Context(), "automation")
	require.NoError(t, err)
	previous := &types.AWSCredentials{AccessKeyID: "synthetic-previous-session", SecretAccessKey: "synthetic-previous-secret", SessionToken: "synthetic-previous-token", Expiration: time.Now().Add(time.Hour).Format(time.RFC3339)}
	oldKey := parent.chainCacheKey()
	processCredentialCache.Store(oldKey, &processCachedCreds{credentials: previous})
	t.Cleanup(func() { processCredentialCache.Delete(oldKey) })
	isolated, _, err := parent.newOperationManager("automation")
	require.NoError(t, err)
	chain, err := isolated.buildAuthenticationChain("automation")
	require.NoError(t, err)
	isolated.chain = chain
	require.NotEqual(t, oldKey, isolated.chainCacheKey())
	acquired, err := isolated.authenticateChain(t.Context(), "automation")
	require.NoError(t, err)
	assert.NotSame(t, previous, acquired)
	require.NoError(t, isolated.releaseOperationIdentity(t.Context()))
	preserved, ok := processCredentialCache.Load(oldKey)
	require.True(t, ok)
	assert.Same(t, previous, preserved.(*processCachedCreds).credentials)
}
