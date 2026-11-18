package auth

import (
	"context"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
	authregistry "github.com/cloudposse/atmos/pkg/auth/registry"
	"github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/schema"
)

// Compile-time sentinels for the schema fields referenced by these tests.
var (
	_ = schema.AuthContext{Environment: map[string]string{}}
	_ = schema.AuthConfig{IdentityCaseMap: map[string]string{}}
)

// scopedOperationKind is a test-only identity kind registered per test so the
// isolated operation manager constructs a controllable gomock identity.
const scopedOperationKind = "scopedtest/operation"

// registerScopedOperationIdentity registers scopedOperationKind for this test and
// returns a manager whose "store" identity uses it via the mock/aws provider.
func registerScopedOperationIdentity(t *testing.T, identity types.Identity) *manager {
	t.Helper()

	unregister, err := authregistry.RegisterBatch(authregistry.Registration{
		Identities: map[string]authregistry.IdentityConfigConstructor{
			scopedOperationKind: func(string, *schema.Identity) (types.Identity, error) { return identity, nil },
		},
	})
	require.NoError(t, err)
	t.Cleanup(unregister)

	parent := isolatedTestManager(t).(*manager)
	parent.config.Identities["store"] = schema.Identity{Kind: scopedOperationKind, Via: &schema.IdentityVia{Provider: "mock"}}
	return parent
}

// expectScopedIdentityBasics allows the lifecycle calls the manager makes on every identity.
func expectScopedIdentityBasics(identity *types.MockIdentity) {
	identity.EXPECT().Kind().Return(scopedOperationKind).AnyTimes()
	identity.EXPECT().Validate().Return(nil).AnyTimes()
	identity.EXPECT().SetRealm(gomock.Any()).AnyTimes()
	expectNoStoredScopedCredentials(identity)
}

// expectNoStoredScopedCredentials reports that the operation realm starts empty.
func expectNoStoredScopedCredentials(identity *types.MockIdentity) {
	identity.EXPECT().LoadCredentials(gomock.Any()).Return(nil, assert.AnError).AnyTimes()
	identity.EXPECT().GetProviderName().Return("mock", nil).AnyTimes()
	identity.EXPECT().Environment().Return(map[string]string{}, nil).AnyTimes()
}

func TestOperationCredentialStoreRedirectsOnlyTheOwnedTarget(t *testing.T) {
	const parentRealm, operationRealm = "parent-realm", "operation-test"
	tests := []struct {
		name      string
		alias     string
		realm     string
		wantRealm string
	}{
		{name: "target in parent realm", alias: "owned", realm: parentRealm, wantRealm: operationRealm},
		{name: "target legacy realm", alias: "owned", realm: "", wantRealm: operationRealm},
		{name: "ancestor in parent realm", alias: "ancestor", realm: parentRealm, wantRealm: parentRealm},
		{name: "ancestor legacy realm", alias: "ancestor", realm: "", wantRealm: ""},
		{name: "provider", alias: "root", realm: parentRealm, wantRealm: parentRealm},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			backend := types.NewMockCredentialStore(gomock.NewController(t))
			scoped := &operationCredentialStore{CredentialStore: backend, scope: &identityOperationScope{target: "owned", realm: operationRealm}}
			creds := &types.AWSCredentials{AccessKeyID: "synthetic"}

			backend.EXPECT().Store(tt.alias, creds, tt.wantRealm).Return(nil)
			backend.EXPECT().Retrieve(tt.alias, tt.wantRealm).Return(creds, nil)
			backend.EXPECT().Delete(tt.alias, tt.wantRealm).Return(nil)

			require.NoError(t, scoped.Store(tt.alias, creds, tt.realm))
			got, err := scoped.Retrieve(tt.alias, tt.realm)
			require.NoError(t, err)
			assert.Same(t, creds, got)
			require.NoError(t, scoped.Delete(tt.alias, tt.realm))
		})
	}
}

func TestIdentityCredentialRealmSelectsOperationRealmOnlyForTarget(t *testing.T) {
	m := &manager{}
	m.realm.Value = "parent-realm"
	assert.Equal(t, "parent-realm", m.identityCredentialRealm("owned"), "an unscoped manager always uses its realm")

	m.operationScope = &identityOperationScope{target: "owned", realm: "operation-test"}
	assert.Equal(t, "operation-test", m.identityCredentialRealm("owned"))
	assert.Equal(t, "parent-realm", m.identityCredentialRealm("ancestor"))
}

func TestWithIsolatedIdentityConcurrentOperationsStayIsolated(t *testing.T) {
	resetProcessCredentialCache()
	t.Cleanup(resetProcessCredentialCache)
	parent := isolatedTestManager(t).(*manager)
	_, err := parent.Authenticate(t.Context(), "deployer")
	require.NoError(t, err)
	before := snapshotParent(parent)

	const workers = 8
	paths := make(chan string, workers)
	errs := make(chan error, workers)
	var wg sync.WaitGroup
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs <- parent.WithIsolatedIdentity(t.Context(), "automation", func(_ context.Context, auth *schema.AuthContext) error {
				if auth == nil || auth.AWS == nil {
					return errUtils.ErrNilParam
				}
				if _, err := os.Stat(auth.AWS.CredentialsFile); err != nil {
					return err
				}
				paths <- auth.AWS.CredentialsFile
				return nil
			})
		}()
	}
	wg.Wait()
	close(errs)
	close(paths)
	for err := range errs {
		require.NoError(t, err)
	}

	seen := map[string]bool{}
	for path := range paths {
		require.False(t, seen[path], "each operation must own a private credentials file")
		seen[path] = true
	}
	require.Len(t, seen, workers)

	processCredentialCache.Range(func(key, _ any) bool {
		assert.NotContains(t, key.(string), "operation-", "operation cache entries must not outlive the operation")
		return true
	})
	assertParentUnchanged(t, parent, before)
}

func TestWithIsolatedIdentityReleasesAfterOperationPanic(t *testing.T) {
	ctrl := gomock.NewController(t)
	identity := types.NewMockIdentity(ctrl)
	expectScopedIdentityBasics(identity)
	parent := registerScopedOperationIdentity(t, identity)

	identity.EXPECT().Authenticate(gomock.Any(), gomock.Any()).Return(&types.AWSCredentials{AccessKeyID: "synthetic"}, nil)
	identity.EXPECT().PostAuthenticate(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, params *types.PostAuthenticateParams) error {
		params.AuthContext.Environment = map[string]string{"VAULT_TOKEN": "synthetic"}
		return nil
	})
	identity.EXPECT().Logout(gomock.Any()).Return(nil).Times(1)

	require.PanicsWithValue(t, "operation failed", func() {
		_ = parent.WithIsolatedIdentity(t.Context(), "store", func(context.Context, *schema.AuthContext) error {
			panic("operation failed")
		})
	})
}

func TestWithIsolatedIdentityReleasesAfterFailedAcquisition(t *testing.T) {
	ctrl := gomock.NewController(t)
	identity := types.NewMockIdentity(ctrl)
	expectScopedIdentityBasics(identity)
	parent := registerScopedOperationIdentity(t, identity)

	identity.EXPECT().Authenticate(gomock.Any(), gomock.Any()).Return(nil, assert.AnError)
	// A server may already have minted a credential before reporting failure.
	identity.EXPECT().Logout(gomock.Any()).Return(nil).Times(1)

	called := false
	err := parent.WithIsolatedIdentity(t.Context(), "store", func(context.Context, *schema.AuthContext) error {
		called = true
		return nil
	})
	require.ErrorIs(t, err, assert.AnError)
	assert.False(t, called, "a failed acquisition must never dispatch the operation")
}

func TestWithIsolatedIdentityCleanupOutlivesCallerCancellation(t *testing.T) {
	ctrl := gomock.NewController(t)
	identity := types.NewMockIdentity(ctrl)
	expectScopedIdentityBasics(identity)
	parent := registerScopedOperationIdentity(t, identity)

	identity.EXPECT().Authenticate(gomock.Any(), gomock.Any()).Return(&types.AWSCredentials{AccessKeyID: "synthetic"}, nil)
	identity.EXPECT().PostAuthenticate(gomock.Any(), gomock.Any()).Return(nil)
	identity.EXPECT().Logout(gomock.Any()).DoAndReturn(func(ctx context.Context) error {
		require.NoError(t, ctx.Err(), "logout must run on a live cleanup context after caller cancellation")
		deadline, ok := ctx.Deadline()
		require.True(t, ok, "cleanup must stay bounded")
		assert.LessOrEqual(t, time.Until(deadline), 30*time.Second)
		return nil
	})

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	err := parent.WithIsolatedIdentity(ctx, "store", func(opCtx context.Context, _ *schema.AuthContext) error {
		cancel()
		<-opCtx.Done()
		return opCtx.Err()
	})
	require.ErrorIs(t, err, context.Canceled)
}

func TestWithIsolatedIdentityRealmScopesOnlyTarget(t *testing.T) {
	ctrl := gomock.NewController(t)
	identity := types.NewMockIdentity(ctrl)
	identity.EXPECT().Kind().Return(scopedOperationKind).AnyTimes()
	identity.EXPECT().Validate().Return(nil).AnyTimes()
	var realms []string
	identity.EXPECT().SetRealm(gomock.Any()).Do(func(realm string) { realms = append(realms, realm) }).AnyTimes()
	expectNoStoredScopedCredentials(identity)
	parent := registerScopedOperationIdentity(t, identity)

	identity.EXPECT().Authenticate(gomock.Any(), gomock.Any()).Return(&types.AWSCredentials{AccessKeyID: "synthetic"}, nil)
	identity.EXPECT().PostAuthenticate(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, params *types.PostAuthenticateParams) error {
		require.True(t, strings.HasPrefix(params.Realm, "operation-"), "target materializes into its operation realm, got %q", params.Realm)
		require.NotEqual(t, parent.realm.Value, params.Realm)
		return nil
	})
	identity.EXPECT().Logout(gomock.Any()).Return(nil)

	require.NoError(t, parent.WithIsolatedIdentity(t.Context(), "store", func(context.Context, *schema.AuthContext) error { return nil }))
	require.NotEmpty(t, realms)
	assert.True(t, strings.HasPrefix(realms[len(realms)-1], "operation-"), "the last realm assigned to the target must be its operation realm")
}
