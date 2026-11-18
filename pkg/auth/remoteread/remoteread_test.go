package remoteread

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

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

// Compile-time sentinel: the scoping tests tell configurations apart by this field.
var _ = schema.AuthConfig{Realm: "a"}

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

func TestResolve_FailedResolutionIsRetriedAndNotCached(t *testing.T) {
	ctrl := gomock.NewController(t)
	manager := types.NewMockAuthManager(ctrl)
	transient := errors.Join(errUtils.ErrIdentityAuthFailed, errors.New("token endpoint timed out"))
	gomock.InOrder(
		manager.EXPECT().EnsureIdentityEnvironment(gomock.Any(), "ops").Return(nil, transient),
		// The second fetch tries again; the third is served from the successful resolution.
		manager.EXPECT().EnsureIdentityEnvironment(gomock.Any(), "ops").Return(map[string]string{"A": "1"}, nil),
	)
	resolver := newTestResolver(t, manager, true)
	atmosConfig := &schema.AtmosConfiguration{}

	_, err := resolver.Resolve(context.Background(), atmosConfig, "ops")
	require.ErrorIs(t, err, errUtils.ErrRemoteIdentityLoginRequired)

	second, err := resolver.Resolve(context.Background(), atmosConfig, "ops")
	require.NoError(t, err, "a failure must not be kept for the life of the process")
	assert.Equal(t, []string{"A=1"}, second)

	third, err := resolver.Resolve(context.Background(), atmosConfig, "ops")
	require.NoError(t, err)
	assert.Equal(t, []string{"A=1"}, third)
}

func TestResolve_ManagerFailureIsRetried(t *testing.T) {
	ctrl := gomock.NewController(t)
	manager := types.NewMockAuthManager(ctrl)
	manager.EXPECT().EnsureIdentityEnvironment(gomock.Any(), "ops").Return(map[string]string{"A": "1"}, nil).Times(1)
	boom := errors.New("keyring locked")
	calls := 0
	resolver := NewResolver(
		WithManagerFactory(func(*schema.AtmosConfiguration) (identityManager, error) {
			calls++
			if calls == 1 {
				return nil, boom
			}
			return manager, nil
		}),
		WithCIDetector(func() bool { return true }),
	)
	atmosConfig := &schema.AtmosConfiguration{}

	_, err := resolver.Resolve(context.Background(), atmosConfig, "ops")
	require.ErrorIs(t, err, boom)

	env, err := resolver.Resolve(context.Background(), atmosConfig, "ops")
	require.NoError(t, err)
	assert.Equal(t, []string{"A=1"}, env)
	assert.Equal(t, 2, calls, "the manager is built again after a failure, then kept")
}

// The same identity name under two auth configurations must not share an environment or a manager:
// that would let one configuration fetch with the other's credentials.
func TestResolve_ScopedToAuthConfiguration(t *testing.T) {
	ctrl := gomock.NewController(t)
	managers := map[string]*types.MockAuthManager{"a": types.NewMockAuthManager(ctrl), "b": types.NewMockAuthManager(ctrl)}
	managers["a"].EXPECT().EnsureIdentityEnvironment(gomock.Any(), "ops").Return(map[string]string{"REALM": "a"}, nil).Times(1)
	managers["b"].EXPECT().EnsureIdentityEnvironment(gomock.Any(), "ops").Return(map[string]string{"REALM": "b"}, nil).Times(1)
	built := map[string]int{}
	resolver := NewResolver(
		WithManagerFactory(func(atmosConfig *schema.AtmosConfiguration) (identityManager, error) {
			built[atmosConfig.Auth.Realm]++
			return managers[atmosConfig.Auth.Realm], nil
		}),
		WithCIDetector(func() bool { return true }),
	)
	configA := func() *schema.AtmosConfiguration {
		return &schema.AtmosConfiguration{Auth: schema.AuthConfig{Realm: "a"}}
	}
	configB := &schema.AtmosConfiguration{Auth: schema.AuthConfig{Realm: "b"}}

	fromA, err := resolver.Resolve(context.Background(), configA(), "ops")
	require.NoError(t, err)
	fromB, err := resolver.Resolve(context.Background(), configB, "ops")
	require.NoError(t, err)
	// A second copy of configuration A (a different pointer, the same content) reuses A's scope.
	againA, err := resolver.Resolve(context.Background(), configA(), "ops")
	require.NoError(t, err)

	assert.Equal(t, []string{"REALM=a"}, fromA)
	assert.Equal(t, []string{"REALM=b"}, fromB, "configuration B must not receive configuration A's environment")
	assert.Equal(t, []string{"REALM=a"}, againA)
	assert.Equal(t, map[string]int{"a": 1, "b": 1}, built, "one manager per configuration, reused for equal configurations")
}

// The same auth section loaded from another directory is another configuration: the default realm
// and the profile fallback both derive from that directory.
func TestResolve_ScopedToConfigurationDirectory(t *testing.T) {
	first, ok := scopeKey(&schema.AtmosConfiguration{CliConfigPath: "repo-one"})
	require.True(t, ok)
	second, ok := scopeKey(&schema.AtmosConfiguration{CliConfigPath: "repo-two"})
	require.True(t, ok)
	same, ok := scopeKey(&schema.AtmosConfiguration{CliConfigPath: "repo-one"})
	require.True(t, ok)

	assert.NotEqual(t, first, second)
	assert.Equal(t, first, same)

	_, ok = scopeKey(nil)
	assert.False(t, ok, "a nil configuration has no scope to share")
}

// The auth manager keeps the chain it is authenticating in a field, so the resolver must never run
// two identities through one manager at the same time.
func TestResolve_ManagerIsNeverUsedConcurrently(t *testing.T) {
	ctrl := gomock.NewController(t)
	manager := types.NewMockAuthManager(ctrl)
	var inFlight, maxInFlight atomic.Int32
	ensure := func(_ context.Context, identity string) (map[string]string, error) {
		current := inFlight.Add(1)
		defer inFlight.Add(-1)
		for {
			seen := maxInFlight.Load()
			if current <= seen || maxInFlight.CompareAndSwap(seen, current) {
				break
			}
		}
		time.Sleep(5 * time.Millisecond)
		return map[string]string{"ID": identity}, nil
	}
	identities := []string{"one", "two", "three", "four"}
	for _, identity := range identities {
		manager.EXPECT().EnsureIdentityEnvironment(gomock.Any(), identity).DoAndReturn(ensure).Times(1)
	}
	resolver := newTestResolver(t, manager, true)
	atmosConfig := &schema.AtmosConfiguration{}

	var wg sync.WaitGroup
	results := make([][]string, len(identities))
	for i, identity := range identities {
		wg.Add(1)
		go func() {
			defer wg.Done()
			env, err := resolver.Resolve(context.Background(), atmosConfig, identity)
			if err == nil {
				results[i] = env
			}
		}()
	}
	wg.Wait()

	assert.Equal(t, int32(1), maxInFlight.Load(), "identities must be resolved one at a time")
	assert.Equal(t, []string{"ID=one"}, results[0])
	assert.Equal(t, []string{"ID=four"}, results[len(results)-1])
}

// Importing this package registers the resolver the downloader uses, so a named identity reaches the
// auth layer instead of failing with "no resolver registered".
func TestInit_RegistersResolverWithDownloader(t *testing.T) {
	_, err := downloader.WithIdentity(context.Background(), &schema.AtmosConfiguration{}, "not-configured", "")

	require.Error(t, err, "an identity that is not configured cannot resolve")
	assert.NotErrorIs(t, err, errUtils.ErrRemoteIdentityResolverUnavailable)
}
