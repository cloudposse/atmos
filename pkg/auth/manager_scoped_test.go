package auth

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/schema"
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

func TestScopedCleanupIsReverseOrderedAndPreservesProvider(t *testing.T) {
	ctrl := gomock.NewController(t)
	provider := types.NewMockProvider(ctrl)
	first, second := types.NewMockIdentity(ctrl), types.NewMockIdentity(ctrl)
	credentialStore := types.NewMockCredentialStore(ctrl)
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
		credentialStore: credentialStore,
		chain:           []string{"root", "first", "second"},
	}
	credentialStore.EXPECT().Delete(gomock.Any(), "").Return(nil).Times(2)
	gomock.InOrder(
		second.EXPECT().Logout(gomock.Any()).Return(assert.AnError),
		first.EXPECT().Logout(gomock.Any()).Return(nil),
	)
	err := m.releaseIsolatedIdentities(t.Context(), m.chain)
	require.ErrorIs(t, err, assert.AnError)
	// No provider.Logout expectation: an accidental provider logout fails here.
}
