package registryauth

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestResolverPreparesUnavailableAuthKindsOnlyWhileAuthenticating(t *testing.T) {
	resolver, manager, repo := testResolver(t)
	constructions, preparations, cleanups := 0, 0, 0
	resolver.newManager = func(*schema.AuthConfig, string) (Manager, error) {
		constructions++
		if constructions == 1 {
			return nil, errUtils.ErrInvalidProviderKind
		}
		return manager, nil
	}
	resolver.prepare = func(context.Context) (func(), error) {
		preparations++
		return func() { cleanups++ }, nil
	}
	manager.EXPECT().Authenticate(gomock.Any(), "corporate").DoAndReturn(func(context.Context, string) (*types.WhoamiInfo, error) {
		require.Equal(t, 1, preparations)
		require.Zero(t, cleanups, "temporary constructors must remain available until authentication completes")
		return &types.WhoamiInfo{Credentials: &types.OCICredentials{Registry: repo.RegistryStr(), Username: "bot", Password: "secret"}}, nil
	})
	_, matched, err := resolver.Resolve(t.Context(), repo)
	require.NoError(t, err)
	require.True(t, matched)
	require.Equal(t, 2, constructions)
	require.Equal(t, 1, cleanups)
}

func TestPreparationFailureAndUnresolvedCycleDoNotLeaveRegistrations(t *testing.T) {
	resolver, _, repo := testResolver(t)
	resolver.newManager = func(*schema.AuthConfig, string) (Manager, error) { return nil, errUtils.ErrInvalidProviderKind }
	cleanup := false
	resolver.prepare = func(context.Context) (func(), error) { return func() { cleanup = true }, nil }
	_, _, err := resolver.Resolve(t.Context(), repo)
	require.ErrorIs(t, err, errUtils.ErrIdentityAuthFailed)
	require.ErrorContains(t, err, "before plugins are downloaded")
	require.True(t, cleanup)
	resolver.prepare = func(context.Context) (func(), error) { return nil, os.ErrPermission }
	_, _, err = resolver.Resolve(t.Context(), repo)
	require.ErrorIs(t, err, os.ErrPermission)
}
