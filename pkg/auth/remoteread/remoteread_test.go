package remoteread

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"

	cockroachErrors "github.com/cockroachdb/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/downloader"
	"github.com/cloudposse/atmos/pkg/schema"
)

// Compile-time sentinel: the tests build the auth configuration through this struct.
var _ = schema.AtmosConfiguration{}

func newTestResolver(t *testing.T, manager identityManager, inCI bool) *Resolver {
	t.Helper()
	return NewResolver(
		WithManagerFactory(func(*schema.AtmosConfiguration) (identityManager, error) { return manager, nil }),
		WithCIDetector(func() bool { return inCI }),
	)
}

func TestResolve_LoggedInIdentityComposesEnvironment(t *testing.T) {
	ctrl := gomock.NewController(t)
	manager := types.NewMockAuthManager(ctrl)
	manager.EXPECT().GetCachedCredentials(gomock.Any(), "ops").Return(&types.WhoamiInfo{}, nil)
	manager.EXPECT().EnsureIdentityEnvironment(gomock.Any(), "ops").DoAndReturn(
		func(ctx context.Context, _ string) (map[string]string, error) {
			assert.False(t, types.AllowPrompts(ctx), "a remote read must never allow prompts")
			return map[string]string{"SSH_AUTH_SOCK": "/tmp/ops.sock", "GIT_SSH_COMMAND": "ssh -o IdentitiesOnly=yes"}, nil
		},
	)

	env, err := newTestResolver(t, manager, false).Resolve(context.Background(), &schema.AtmosConfiguration{}, "ops")

	require.NoError(t, err)
	assert.Equal(t, []string{"GIT_SSH_COMMAND=ssh -o IdentitiesOnly=yes", "SSH_AUTH_SOCK=/tmp/ops.sock"}, env,
		"only the identity's variables are returned, not the process environment")
}

func TestResolve_NotLoggedInFailsWithSentinelAndLoginHint(t *testing.T) {
	tests := []struct {
		name string
		err  error
	}{
		{name: "no cached credentials", err: fmt.Errorf("%w: identity=ops", errUtils.ErrNoCredentialsFound)},
		{name: "expired credentials", err: fmt.Errorf("%w: `ops`", errUtils.ErrExpiredCredentials)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			manager := types.NewMockAuthManager(ctrl)
			// No EnsureIdentityEnvironment expectation: calling it would authenticate (and could
			// prompt), and gomock fails the test if it happens.
			manager.EXPECT().GetCachedCredentials(gomock.Any(), "ops").Return(nil, tt.err)

			env, err := newTestResolver(t, manager, false).Resolve(context.Background(), &schema.AtmosConfiguration{}, "ops")

			require.ErrorIs(t, err, errUtils.ErrRemoteIdentityLoginRequired)
			assert.Nil(t, env)
			hints := fmt.Sprint(cockroachErrors.GetAllHints(err))
			assert.Contains(t, hints, "atmos auth login --identity ops")
		})
	}
}

func TestResolve_UnknownIdentityIsNotReportedAsLoginRequired(t *testing.T) {
	ctrl := gomock.NewController(t)
	manager := types.NewMockAuthManager(ctrl)
	manager.EXPECT().GetCachedCredentials(gomock.Any(), "typo").Return(nil, fmt.Errorf("%w: `typo`", errUtils.ErrIdentityNotFound))

	_, err := newTestResolver(t, manager, false).Resolve(context.Background(), &schema.AtmosConfiguration{}, "typo")

	require.ErrorIs(t, err, errUtils.ErrIdentityNotFound)
	assert.NotErrorIs(t, err, errUtils.ErrRemoteIdentityLoginRequired, "logging in would not fix a misspelled identity")
}

func TestResolve_InCIAuthenticatesWithoutRequiringACachedSession(t *testing.T) {
	ctrl := gomock.NewController(t)
	manager := types.NewMockAuthManager(ctrl)
	// In CI there is nobody to prompt; the cached-session pre-check is skipped, so
	// GetCachedCredentials is deliberately not expected.
	manager.EXPECT().EnsureIdentityEnvironment(gomock.Any(), "source-reader").
		Return(map[string]string{"GIT_CONFIG_COUNT": "1"}, nil)

	env, err := newTestResolver(t, manager, true).Resolve(context.Background(), &schema.AtmosConfiguration{}, "source-reader")

	require.NoError(t, err)
	assert.Equal(t, []string{"GIT_CONFIG_COUNT=1"}, env)
}

func TestResolve_AuthenticationFailureMapsToLoginRequired(t *testing.T) {
	ctrl := gomock.NewController(t)
	manager := types.NewMockAuthManager(ctrl)
	manager.EXPECT().EnsureIdentityEnvironment(gomock.Any(), "ops").Return(nil, errors.Join(errUtils.ErrIdentityAuthFailed, errors.New("device flow unavailable")))

	_, err := newTestResolver(t, manager, true).Resolve(context.Background(), &schema.AtmosConfiguration{}, "ops")

	require.ErrorIs(t, err, errUtils.ErrRemoteIdentityLoginRequired)
	require.ErrorIs(t, err, errUtils.ErrIdentityAuthFailed, "the cause stays in the chain")
}

func TestResolve_ManagerFailure(t *testing.T) {
	boom := errors.New("bad auth config")
	resolver := NewResolver(WithManagerFactory(func(*schema.AtmosConfiguration) (identityManager, error) { return nil, boom }))

	_, err := resolver.Resolve(context.Background(), &schema.AtmosConfiguration{}, "ops")

	require.ErrorIs(t, err, errUtils.ErrAuthManager)
	require.ErrorIs(t, err, boom)
}

func TestResolve_SharedIdentityResolvedOnceAcrossConcurrentSources(t *testing.T) {
	ctrl := gomock.NewController(t)
	manager := types.NewMockAuthManager(ctrl)
	manager.EXPECT().GetCachedCredentials(gomock.Any(), "ops").Return(&types.WhoamiInfo{}, nil).Times(1)
	manager.EXPECT().EnsureIdentityEnvironment(gomock.Any(), "ops").Return(map[string]string{"A": "1"}, nil).Times(1)
	manager.EXPECT().GetCachedCredentials(gomock.Any(), "deploy").Return(&types.WhoamiInfo{}, nil).Times(1)
	manager.EXPECT().EnsureIdentityEnvironment(gomock.Any(), "deploy").Return(map[string]string{"B": "2"}, nil).Times(1)
	resolver := newTestResolver(t, manager, false)

	var wg sync.WaitGroup
	results := make(chan string, 16)
	for i := 0; i < 8; i++ {
		for _, identity := range []string{"ops", "deploy"} {
			wg.Add(1)
			go func() {
				defer wg.Done()
				env, err := resolver.Resolve(context.Background(), &schema.AtmosConfiguration{}, identity)
				if err != nil {
					results <- "error: " + err.Error()
					return
				}
				results <- identity + ":" + env[0]
			}()
		}
	}
	wg.Wait()
	close(results)

	counts := map[string]int{}
	for result := range results {
		counts[result]++
	}
	assert.Equal(t, map[string]int{"ops:A=1": 8, "deploy:B=2": 8}, counts, "each identity keeps its own environment")
}

func TestResolve_ReturnedEnvironmentIsACopy(t *testing.T) {
	ctrl := gomock.NewController(t)
	manager := types.NewMockAuthManager(ctrl)
	manager.EXPECT().EnsureIdentityEnvironment(gomock.Any(), "ops").Return(map[string]string{"A": "1"}, nil).Times(1)
	resolver := newTestResolver(t, manager, true)

	first, err := resolver.Resolve(context.Background(), &schema.AtmosConfiguration{}, "ops")
	require.NoError(t, err)
	first[0] = "A=mutated"
	second, err := resolver.Resolve(context.Background(), &schema.AtmosConfiguration{}, "ops")

	require.NoError(t, err)
	assert.Equal(t, []string{"A=1"}, second)
}

// Importing this package registers the resolver the downloader uses, so a named identity reaches the
// auth layer instead of failing with "no resolver registered".
func TestInit_RegistersResolverWithDownloader(t *testing.T) {
	_, err := downloader.WithIdentity(context.Background(), &schema.AtmosConfiguration{}, "not-configured", "")

	require.Error(t, err, "an identity that is not configured cannot resolve")
	assert.NotErrorIs(t, err, errUtils.ErrRemoteIdentityResolverUnavailable)
}
