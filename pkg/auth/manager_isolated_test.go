package auth

import (
	"context"
	"errors"
	"maps"
	"slices"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth/credentials"
	"github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/schema"
)

// Compile-time sentinels for the schema fields referenced by these tests.
var (
	_ = schema.Provider{Kind: "mock/aws"}
	_ = schema.Identity{Kind: "mock/aws", Default: true, Via: &schema.IdentityVia{Provider: "mock"}}
	_ = schema.AWSAuthContext{Profile: ""}
)

// isolatedTestManager builds a real manager backed by the mock/aws provider and two
// identities: "deployer" (default) and "automation".
func isolatedTestManager(t testing.TB) types.AuthManager {
	t.Helper()

	root := t.TempDir()
	t.Setenv("HOME", root)
	t.Setenv("XDG_CONFIG_HOME", root)
	t.Setenv("XDG_CACHE_HOME", root)
	t.Setenv("XDG_DATA_HOME", root)

	cfg := &schema.AuthConfig{
		Realm:     "isolated-test",
		Keyring:   schema.KeyringConfig{Type: types.CredentialStoreTypeMemory},
		Providers: map[string]schema.Provider{"mock": {Kind: "mock/aws"}},
		Identities: map[string]schema.Identity{
			"deployer":   {Kind: "mock/aws", Default: true, Via: &schema.IdentityVia{Provider: "mock"}},
			"automation": {Kind: "mock/aws", Via: &schema.IdentityVia{Provider: "mock"}},
		},
	}

	ctrl := gomock.NewController(t)
	validator := types.NewMockValidator(ctrl)
	validator.EXPECT().ValidateAuthConfig(gomock.Any()).Return(nil).AnyTimes()
	validator.EXPECT().ValidateProvider(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
	validator.EXPECT().ValidateIdentity(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
	validator.EXPECT().ValidateChains(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()

	manager, err := NewAuthManager(cfg, credentials.NewCredentialStoreWithConfig(cfg), validator, &schema.ConfigAndStacksInfo{
		Stack:           "dev",
		ProfilesFromArg: []string{"one"},
	}, "")
	require.NoError(t, err)
	return manager
}

// parentSnapshot captures the observable state of a manager that AuthenticateIsolated must not modify.
type parentSnapshot struct {
	chain      []string
	aws        *schema.AWSAuthContext
	identities map[string]schema.Identity
	providers  map[string]schema.Provider
	stackInfo  *schema.ConfigAndStacksInfo
	authCtxPtr *schema.AuthContext
}

func snapshotParent(m types.AuthManager) parentSnapshot {
	snap := parentSnapshot{
		chain:      slices.Clone(m.GetChain()),
		identities: maps.Clone(m.GetIdentities()),
		providers:  maps.Clone(m.GetProviders()),
		stackInfo:  m.GetStackInfo(),
	}
	if ctx := m.GetStackInfo().AuthContext; ctx != nil {
		snap.authCtxPtr = ctx
		if ctx.AWS != nil {
			awsCopy := *ctx.AWS
			snap.aws = &awsCopy
		}
	}
	return snap
}

func assertParentUnchanged(t *testing.T, m types.AuthManager, want parentSnapshot) {
	t.Helper()
	got := snapshotParent(m)
	assert.Equal(t, want.chain, got.chain, "parent chain must not change")
	assert.Equal(t, want.aws, got.aws, "parent AWS auth context must not change")
	assert.Equal(t, want.identities, got.identities, "parent identities must not change")
	assert.Equal(t, want.providers, got.providers, "parent providers must not change")
	assert.Same(t, want.stackInfo, got.stackInfo, "parent stack info pointer must not change")
	assert.Same(t, want.authCtxPtr, got.authCtxPtr, "parent auth context pointer must not change")
}

func TestManager_AuthenticateIsolated_DoesNotMutateParent(t *testing.T) {
	manager := isolatedTestManager(t)
	_, err := manager.Authenticate(t.Context(), "deployer")
	require.NoError(t, err)

	before := snapshotParent(manager)
	require.NotNil(t, before.aws)
	require.Equal(t, "deployer", before.aws.Profile)
	require.NotEmpty(t, before.chain)
	require.Equal(t, "deployer", before.chain[len(before.chain)-1])

	first, err := manager.AuthenticateIsolated(t.Context(), "automation")
	require.NoError(t, err)
	require.NotNil(t, first)
	require.NotNil(t, first.AWS)
	assert.Equal(t, "automation", first.AWS.Profile)

	assertParentUnchanged(t, manager, before)
	assert.NotSame(t, manager.GetStackInfo().AuthContext, first, "result must not alias the parent's auth context")
	assert.Equal(t, "deployer", manager.GetStackInfo().AuthContext.AWS.Profile)

	second, err := manager.AuthenticateIsolated(t.Context(), "automation")
	require.NoError(t, err)
	assert.NotSame(t, first, second, "each call returns its own auth context")
	assert.Equal(t, "automation", second.AWS.Profile)

	// Mutating the returned context must not reach the parent or later results.
	first.AWS.Profile = "mutated"
	assert.Equal(t, "automation", second.AWS.Profile)
	assertParentUnchanged(t, manager, before)
}

func TestManager_AuthenticateIsolated_ParentStackMetadataUntouched(t *testing.T) {
	manager := isolatedTestManager(t)
	info := manager.GetStackInfo()

	_, err := manager.AuthenticateIsolated(t.Context(), "automation")
	require.NoError(t, err)

	assert.Equal(t, "dev", info.Stack)
	assert.Equal(t, []string{"one"}, info.ProfilesFromArg)
}

func TestManager_AuthenticateIsolated_NeverAuthenticatedParent(t *testing.T) {
	manager := isolatedTestManager(t)
	require.Empty(t, manager.GetChain())
	require.Nil(t, manager.GetStackInfo().AuthContext)

	got, err := manager.AuthenticateIsolated(t.Context(), "automation")
	require.NoError(t, err)
	require.NotNil(t, got)
	require.NotNil(t, got.AWS)
	assert.Equal(t, "automation", got.AWS.Profile)

	assert.Empty(t, manager.GetChain(), "parent chain must stay empty")
	if ctx := manager.GetStackInfo().AuthContext; ctx != nil {
		assert.Nil(t, ctx.AWS, "parent AWS context must stay nil")
	}
}

func TestManager_AuthenticateIsolated_Failures(t *testing.T) {
	canceled, cancel := context.WithCancel(context.Background())
	cancel()

	tests := []struct {
		name     string
		ctx      func(t *testing.T) context.Context
		identity string
		wantErr  error
	}{
		{name: "unknown identity", ctx: func(t *testing.T) context.Context { return t.Context() }, identity: "missing", wantErr: errUtils.ErrIdentityNotFound},
		{name: "empty identity", ctx: func(t *testing.T) context.Context { return t.Context() }, identity: "", wantErr: errUtils.ErrNilParam},
		{name: "canceled context", ctx: func(*testing.T) context.Context { return canceled }, identity: "automation"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			manager := isolatedTestManager(t)
			_, err := manager.Authenticate(t.Context(), "deployer")
			require.NoError(t, err)
			before := snapshotParent(manager)

			got, err := manager.AuthenticateIsolated(tt.ctx(t), tt.identity)
			if tt.wantErr != nil {
				require.Error(t, err)
				assert.True(t, errors.Is(err, tt.wantErr), "got %v", err)
				assert.Nil(t, got)
			}
			// The mock/aws provider may ignore ctx cancellation; either outcome must leave the parent untouched.
			assertParentUnchanged(t, manager, before)
		})
	}
}

func TestManager_AuthenticateIsolated_Concurrent(t *testing.T) {
	manager := isolatedTestManager(t)

	const workers = 6
	var wg sync.WaitGroup
	results := make([]*schema.AuthContext, workers)
	errs := make([]error, workers)
	for i := range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results[i], errs[i] = manager.AuthenticateIsolated(t.Context(), "automation")
		}()
	}
	wg.Add(1)
	var parentErr error
	go func() {
		defer wg.Done()
		_, parentErr = manager.Authenticate(t.Context(), "deployer")
	}()
	wg.Wait()

	require.NoError(t, parentErr)
	for i := range workers {
		require.NoError(t, errs[i])
		require.NotNil(t, results[i])
		assert.Equal(t, "automation", results[i].AWS.Profile)
	}
	assert.Equal(t, "deployer", manager.GetStackInfo().AuthContext.AWS.Profile)
	chain := manager.GetChain()
	require.NotEmpty(t, chain)
	assert.Equal(t, "deployer", chain[len(chain)-1])
}
