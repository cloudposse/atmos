package registryauth

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/google/go-containerregistry/pkg/name"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth/integrations"
	"github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/schema"
)

func testResolver(t *testing.T) (*Resolver, *MockManager, name.Repository) {
	t.Helper()
	host := "registry.example.com:5000"
	config := &schema.AtmosConfiguration{Auth: schema.AuthConfig{Identities: map[string]schema.Identity{"corporate": {Kind: "oci/user", Spec: map[string]any{"registry": host, "username": "bot"}}}}}
	resolver := New(config, map[string]string{host: "corporate"})
	manager := NewMockManager(gomock.NewController(t))
	manager.EXPECT().GetCachedCredentials(gomock.Any(), "corporate").Return(nil, errUtils.ErrIdentityCredentialsNone).AnyTimes()
	resolver.newManager = func(*schema.AuthConfig, string) (Manager, error) { return manager, nil }
	repository, err := name.NewRepository(host + "/plugins/oidc")
	require.NoError(t, err)
	return resolver, manager, repository
}

func TestResolverScopesCredentialsAndCoalescesDownloads(t *testing.T) {
	resolver, manager, repo := testResolver(t)
	manager.EXPECT().Authenticate(gomock.Any(), "corporate").Return(&types.WhoamiInfo{Credentials: &types.OCICredentials{Registry: repo.RegistryStr(), Username: "bot", Password: "secret"}}, nil).Times(2)
	for range 2 {
		auth, matched, err := resolver.Resolve(t.Context(), repo)
		require.NoError(t, err)
		require.True(t, matched)
		config, err := auth.Authorization()
		require.NoError(t, err)
		require.Equal(t, "bot", config.Username)
		require.Equal(t, "secret", config.Password)
	}
	other, err := name.NewRepository("other.example.com/plugins/oidc")
	require.NoError(t, err)
	auth, matched, err := resolver.Resolve(t.Context(), other)
	require.NoError(t, err)
	require.False(t, matched)
	require.Nil(t, auth)
}

func TestResolverFailsClosedAndNeverCachesFailures(t *testing.T) {
	resolver, manager, repo := testResolver(t)
	manager.EXPECT().Authenticate(gomock.Any(), "corporate").Return(nil, os.ErrPermission).Times(2)
	for range 2 {
		auth, matched, err := resolver.Resolve(t.Context(), repo)
		require.ErrorIs(t, err, os.ErrPermission)
		require.True(t, matched)
		require.Nil(t, auth)
	}
}

func TestResolverRejectsWrongScopeUsernameAndExpiredCredentials(t *testing.T) {
	expired := time.Now().Add(-time.Minute)
	for _, tc := range []struct {
		name, host, username string
		expiry               *time.Time
	}{
		{name: "host", host: "other.example.com", username: "bot"},
		{name: "username", host: "registry.example.com:5000", username: "other"},
		{name: "expired", host: "registry.example.com:5000", username: "bot", expiry: &expired},
	} {
		t.Run(tc.name, func(t *testing.T) {
			resolver, manager, repo := testResolver(t)
			manager.EXPECT().Authenticate(gomock.Any(), "corporate").Return(&types.WhoamiInfo{Credentials: &types.OCICredentials{Registry: tc.host, Username: tc.username, Password: "secret", Expiration: tc.expiry}}, nil)
			auth, matched, err := resolver.Resolve(t.Context(), repo)
			require.Error(t, err)
			require.True(t, matched)
			require.Nil(t, auth)
			require.Empty(t, resolver.cached)
		})
	}
}

func TestResolverDoesNotCacheDirectIdentityCredentials(t *testing.T) {
	resolver, manager, repo := testResolver(t)
	expiry := time.Now().Add(10 * time.Second)
	manager.EXPECT().Authenticate(gomock.Any(), "corporate").Return(&types.WhoamiInfo{Credentials: &types.OCICredentials{Registry: repo.RegistryStr(), Username: "bot", Password: "secret", Expiration: &expiry}}, nil)
	_, matched, err := resolver.Resolve(t.Context(), repo)
	require.NoError(t, err)
	require.True(t, matched)
	require.Empty(t, resolver.cached, "direct credentials must be rechecked against the canonical auth session")
	manager.EXPECT().Authenticate(gomock.Any(), "corporate").Return(nil, os.ErrPermission)
	_, _, err = resolver.Resolve(t.Context(), repo)
	require.ErrorIs(t, err, os.ErrPermission)
}

func TestResolverDetectsRecursiveBootstrapBeforeLocking(t *testing.T) {
	resolver, manager, repo := testResolver(t)
	manager.EXPECT().Authenticate(gomock.Any(), "corporate").DoAndReturn(func(ctx context.Context, _ string) (*types.WhoamiInfo, error) {
		_, matched, err := resolver.Resolve(ctx, repo)
		require.True(t, matched)
		require.ErrorContains(t, err, "own auth plugin")
		return nil, err
	})
	_, _, err := resolver.Resolve(t.Context(), repo)
	require.ErrorIs(t, err, errUtils.ErrIdentityAuthFailed)
}

func TestResolverUsesReturnedExchangeAndCachesAgainstLiveSession(t *testing.T) {
	resolver, manager, _ := testResolver(t)
	host := "123456789012.dkr.ecr.us-east-1.amazonaws.com"
	repo, err := name.NewRepository(host + "/plugins/oidc")
	require.NoError(t, err)
	resolver.bindings = map[string]string{host: "corporate"}
	resolver.config.Auth.Identities["corporate"] = schema.Identity{Kind: "aws/user"}
	resolver.config.Auth.Integrations = map[string]schema.Integration{"ecr": {Kind: "aws/ecr", Via: &schema.IntegrationVia{Identity: "corporate"}, Spec: &schema.IntegrationSpec{Registry: &schema.Registry{AccountID: "123456789012", Region: "us-east-1"}}}}
	original := &types.AWSCredentials{AccessKeyID: "original", SecretAccessKey: "session-secret"}
	rotated := &types.AWSCredentials{AccessKeyID: "rotated", SecretAccessKey: "session-secret"}
	manager.EXPECT().Authenticate(gomock.Any(), "corporate").Return(&types.WhoamiInfo{Credentials: original}, nil).Times(2)
	mintedExpiry := time.Now().Add(10 * time.Second)
	exchanges := 0
	resolver.exchange = func(_ context.Context, config *integrations.IntegrationConfig, creds types.ICredentials) (*types.OCICredentials, error) {
		exchanges++
		require.Equal(t, "ecr", config.Name)
		return &types.OCICredentials{Registry: host, Username: "AWS", Password: creds.(*types.AWSCredentials).AccessKeyID, Expiration: &mintedExpiry}, nil
	}
	for range 2 {
		credentials, matched, err := resolver.Resolve(t.Context(), repo)
		require.NoError(t, err)
		require.True(t, matched)
		actual, err := credentials.Authorization()
		require.NoError(t, err)
		require.Equal(t, "original", actual.Password)
	}
	require.Equal(t, 1, exchanges, "same live auth session should coalesce cloud exchange calls")
	require.Equal(t, mintedExpiry, resolver.cached[host].expires)
	manager.EXPECT().Authenticate(gomock.Any(), "corporate").Return(&types.WhoamiInfo{Credentials: rotated}, nil)
	result, _, err := resolver.Resolve(t.Context(), repo)
	require.NoError(t, err)
	actual, err := result.Authorization()
	require.NoError(t, err)
	require.Equal(t, "rotated", actual.Password)
	require.Equal(t, 2, exchanges, "rotated upstream credentials must invalidate an exchanged token")
	manager.EXPECT().Authenticate(gomock.Any(), "corporate").Return(nil, os.ErrPermission)
	_, _, err = resolver.Resolve(t.Context(), repo)
	require.ErrorIs(t, err, os.ErrPermission)
	require.Empty(t, resolver.cached, "logout or failed identity auth must discard exchanged tokens")
	manager.EXPECT().Authenticate(gomock.Any(), "corporate").Return(&types.WhoamiInfo{Credentials: original}, nil)
	resolver.exchange = func(context.Context, *integrations.IntegrationConfig, types.ICredentials) (*types.OCICredentials, error) {
		return nil, os.ErrPermission
	}
	_, _, err = resolver.Resolve(t.Context(), repo)
	require.ErrorIs(t, err, os.ErrPermission)
	require.Empty(t, resolver.cached, "exchange failure must never fall back to earlier registry tokens")
}

func TestResolverNormalizesHostCaseWithoutAmbientFallback(t *testing.T) {
	resolver, manager, repo := testResolver(t)
	manager.EXPECT().Authenticate(gomock.Any(), "corporate").Return(nil, os.ErrPermission)
	upper, err := name.NewRepository("REGISTRY.EXAMPLE.COM:5000/plugins/oidc")
	require.NoError(t, err)
	require.NotEqual(t, repo.RegistryStr(), upper.RegistryStr())
	_, matched, err := resolver.Resolve(t.Context(), upper)
	require.True(t, matched)
	require.ErrorIs(t, err, os.ErrPermission)
}

func TestExplicitAuthenticateRefreshesAndLogoutUsesLifecycle(t *testing.T) {
	resolver, _, repo := testResolver(t)
	manager := NewMockManager(gomock.NewController(t))
	option := func(target *Resolver) {
		target.newManager = func(*schema.AuthConfig, string) (Manager, error) { return manager, nil }
	}
	manager.EXPECT().Authenticate(gomock.Any(), "corporate").Return(&types.WhoamiInfo{Credentials: &types.OCICredentials{Registry: repo.RegistryStr(), Username: "bot", Password: "fresh"}}, nil).Times(1)
	result, err := Authenticate(t.Context(), resolver.config, "corporate", repo.RegistryStr(), option)
	require.NoError(t, err)
	actual, err := result.Authorization()
	require.NoError(t, err)
	require.Equal(t, "fresh", actual.Password)
	manager.EXPECT().Logout(gomock.Any(), "corporate", true).Return(os.ErrPermission)
	require.ErrorIs(t, Logout(t.Context(), resolver.config, "corporate", option), os.ErrPermission)
	_, err = Authenticate(t.Context(), resolver.config, "corporate", "invalid registry", option)
	require.Error(t, err)
	_, err = Authenticate(t.Context(), resolver.config, "missing", repo.RegistryStr(), option)
	require.ErrorIs(t, err, errUtils.ErrIdentityNotFound)
	require.ErrorIs(t, Logout(t.Context(), resolver.config, "missing", option), errUtils.ErrIdentityNotFound)
	active, err := guardAuthentication(t.Context(), "corporate")
	require.NoError(t, err)
	_, err = Authenticate(active, resolver.config, "corporate", repo.RegistryStr(), option)
	require.ErrorIs(t, err, errUtils.ErrIdentityAuthFailed)
}

func TestExplicitAuthenticationPreservesManagerFailure(t *testing.T) {
	resolver, _, repo := testResolver(t)
	option := func(target *Resolver) {
		target.newManager = func(*schema.AuthConfig, string) (Manager, error) { return nil, os.ErrPermission }
	}
	_, err := Authenticate(t.Context(), resolver.config, "corporate", repo.RegistryStr(), option)
	require.ErrorIs(t, err, os.ErrPermission)
	require.ErrorIs(t, Logout(t.Context(), resolver.config, "corporate", option), os.ErrPermission)
	_, err = Authenticate(t.Context(), resolver.config, "corporate", repo.RegistryStr(), WithPreparation(func(context.Context) (func(), error) {
		return nil, os.ErrPermission
	}), func(target *Resolver) {
		target.newManager = func(*schema.AuthConfig, string) (Manager, error) { return nil, errUtils.ErrInvalidIdentityKind }
	})
	require.ErrorIs(t, err, os.ErrPermission)
}

func TestResolverUsesLiveCacheAndReauthenticatesMissingMaterial(t *testing.T) {
	t.Setenv("ATMOS_KEYRING_TYPE", "memory")
	for _, materialized := range []bool{false, true} {
		t.Run(fmt.Sprint(materialized), func(t *testing.T) {
			original, _, repo := testResolver(t)
			manager := NewMockManager(gomock.NewController(t))
			resolver := New(original.config, original.bindings, func(target *Resolver) {
				target.newManager = func(*schema.AuthConfig, string) (Manager, error) { return manager, nil }
			})
			credentials := &types.OCICredentials{Registry: repo.RegistryStr(), Username: "bot", Password: "cached"}
			cached := &types.WhoamiInfo{Identity: "corporate"}
			if materialized {
				cached.Credentials = credentials
			} else {
				manager.EXPECT().Authenticate(gomock.Any(), "corporate").Return(&types.WhoamiInfo{Credentials: credentials}, nil)
			}
			manager.EXPECT().GetCachedCredentials(gomock.Any(), "corporate").Return(cached, nil)
			result, matched, err := resolver.Resolve(t.Context(), repo)
			require.NoError(t, err)
			require.True(t, matched)
			actual, err := result.Authorization()
			require.NoError(t, err)
			require.Equal(t, "cached", actual.Password)
		})
	}
}

func TestResolverRejectsCanceledCallAndWrongDirectHostBeforeAuthentication(t *testing.T) {
	resolver, _, repo := testResolver(t)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	result, matched, err := resolver.Resolve(ctx, repo)
	require.ErrorIs(t, err, context.Canceled)
	require.True(t, matched)
	require.Nil(t, result)
	resolver.config.Auth.Identities["corporate"] = schema.Identity{Kind: "oci/user", Spec: map[string]any{"registry": "other.example.com"}}
	result, matched, err = resolver.Resolve(t.Context(), repo)
	require.ErrorIs(t, err, errUtils.ErrIdentityAuthFailed)
	require.True(t, matched)
	require.Nil(t, result)
}

func TestResolverRejectsAbsentAndUnsupportedCredentialMaterial(t *testing.T) {
	for name, whoami := range map[string]*types.WhoamiInfo{
		"absent identity":      nil,
		"absent credentials":   {},
		"unsupported contract": {Credentials: &types.AWSCredentials{}},
	} {
		t.Run(name, func(t *testing.T) {
			resolver, manager, repo := testResolver(t)
			manager.EXPECT().Authenticate(gomock.Any(), "corporate").Return(whoami, nil)
			result, matched, err := resolver.Resolve(t.Context(), repo)
			require.Error(t, err)
			require.True(t, matched)
			require.Nil(t, result)
			require.Empty(t, resolver.cached)
		})
	}
}

func TestExchangeRejectsMissingWrongHostAndExpiredCredentials(t *testing.T) {
	past := time.Now().Add(-time.Hour)
	host := "registry.example.com"
	for name, minted := range map[string]*types.OCICredentials{
		"absent":     nil,
		"wrong host": {Registry: "other.example.com", Password: "private"},
		"expired":    {Registry: host, Password: "private", Expiration: &past},
	} {
		t.Run(name, func(t *testing.T) {
			resolver := New(&schema.AtmosConfiguration{}, nil)
			resolver.exchange = func(context.Context, *integrations.IntegrationConfig, types.ICredentials) (*types.OCICredentials, error) {
				return minted, nil
			}
			result, err := resolver.exchangedCredentials(t.Context(), &integrations.IntegrationConfig{Name: "registry", Realm: "test"}, &types.AWSCredentials{}, host)
			require.Error(t, err)
			require.Nil(t, result)
			require.Empty(t, resolver.cached, "failed exchange results must not enter the cache")
		})
	}
}

func TestResolverRejectsDifferentIdentityBeforeLocking(t *testing.T) {
	resolver, _, repo := testResolver(t)
	ctx, err := guardAuthentication(t.Context(), "different-identity")
	require.NoError(t, err)
	// Authentication already owns the lock when a callback resolves another host.
	resolver.mu.Lock()
	done := make(chan error, 1)
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	go func() {
		_, matched, err := resolver.Resolve(ctx, repo)
		if !matched {
			err = fmt.Errorf("%w: bound registry was not matched", errUtils.ErrIdentityAuthFailed)
		}
		done <- err
	}()
	select {
	case err := <-done:
		resolver.mu.Unlock()
		require.ErrorIs(t, err, errUtils.ErrIdentityAuthFailed)
		require.ErrorContains(t, err, "another bound registry")
	case <-time.After(time.Second):
		cancel()
		resolver.mu.Unlock()
		<-done
		t.Fatal("nested authentication waited on the already-held resolver mutex")
	}
	// Unbound public registries must still bypass authentication during bootstrap.
	unbound, err := name.NewRepository("public.example.com/package")
	require.NoError(t, err)
	auth, matched, err := resolver.Resolve(ctx, unbound)
	require.NoError(t, err)
	require.False(t, matched)
	require.Nil(t, auth)
}
