package terraform

import (
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	auth "github.com/cloudposse/atmos/pkg/auth"
	"github.com/cloudposse/atmos/pkg/auth/credentials"
	authtypes "github.com/cloudposse/atmos/pkg/auth/types"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
	storepkg "github.com/cloudposse/atmos/pkg/store"
)

// Compile-time sentinels for the schema and store fields referenced by these tests.
var (
	_ = schema.Provider{Kind: "mock/aws"}
	_ = schema.Identity{Kind: "mock/aws", Default: true, Via: &schema.IdentityVia{Provider: "mock"}}
	_ = schema.AWSAuthContext{Profile: ""}
	_ = storepkg.StoreConfig{Type: "aws-ssm-parameter-store", Identity: "automation"}
)

const (
	isolationStack     = "test"
	isolationComponent = "app"
	isolationDeployer  = "deployer"
	stepState          = "state"
	stepSecretPrefix   = "secret:"
)

// isolationStore is an identity-aware store backed by a gomock store. It records the resolver and
// identity it was bound to (like the real SSM store does in SetAuthContext) and, on Get, resolves AWS
// credentials through that resolver exactly as a real store does in initIdentityClient.
type isolationStore struct {
	*storepkg.MockIdentityAwareStore

	pinned   string
	bound    string
	resolver storepkg.AuthContextResolver
	bindings int
}

func (s *isolationStore) IdentityName() string { return s.pinned }

func (s *isolationStore) SetAuthContext(resolver storepkg.AuthContextResolver, identityName string) {
	s.resolver, s.bound = resolver, identityName
	s.bindings++
}

// effectiveIdentity is the identity a real store would authenticate as: its own pinned identity,
// otherwise the identity inherited from the component.
func (s *isolationStore) effectiveIdentity() string {
	if s.pinned != "" {
		return s.pinned
	}
	return s.bound
}

// isolationHarness wires a real auth manager (mock/aws provider), authenticated as the component
// identity, to identity-aware stores through injectTerraformStoreAuthResolver.
type isolationHarness struct {
	t           *testing.T
	atmosConfig *schema.AtmosConfiguration
	info        *schema.ConfigAndStacksInfo
	manager     authtypes.AuthManager
	stores      map[string]*isolationStore
	chainBefore []string
}

func isolationAuthConfig() *schema.AuthConfig {
	return &schema.AuthConfig{
		Realm:     "isolation-test",
		Keyring:   schema.KeyringConfig{Type: authtypes.CredentialStoreTypeMemory},
		Providers: map[string]schema.Provider{"mock": {Kind: "mock/aws"}},
		Identities: map[string]schema.Identity{
			isolationDeployer: {Kind: "mock/aws", Default: true, Via: &schema.IdentityVia{Provider: "mock"}},
			"automation":      {Kind: "mock/aws", Via: &schema.IdentityVia{Provider: "mock"}},
			"auditor":         {Kind: "mock/aws", Via: &schema.IdentityVia{Provider: "mock"}},
		},
	}
}

// newIsolationHarness builds the harness.
// The storePins argument maps each store name to the identity the store is pinned to; an empty
// value means the store inherits the component identity.
func newIsolationHarness(t *testing.T, identityFlag string, storePins map[string]string) *isolationHarness {
	t.Helper()

	root := t.TempDir()
	t.Setenv("HOME", root)
	t.Setenv("XDG_CONFIG_HOME", root)
	t.Setenv("XDG_CACHE_HOME", root)
	t.Setenv("XDG_DATA_HOME", root)

	authCfg := isolationAuthConfig()
	ctrl := gomock.NewController(t)
	validator := authtypes.NewMockValidator(ctrl)
	validator.EXPECT().ValidateAuthConfig(gomock.Any()).Return(nil).AnyTimes()
	validator.EXPECT().ValidateProvider(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
	validator.EXPECT().ValidateIdentity(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
	validator.EXPECT().ValidateChains(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()

	manager, err := auth.NewAuthManager(authCfg, credentials.NewCredentialStoreWithConfig(authCfg), validator,
		&schema.ConfigAndStacksInfo{Stack: isolationStack}, "")
	require.NoError(t, err)

	// What setupTerraformAuth does for the component: authenticate the component identity first.
	_, err = manager.Authenticate(t.Context(), isolationDeployer)
	require.NoError(t, err)

	h := &isolationHarness{
		t:       t,
		manager: manager,
		stores:  map[string]*isolationStore{},
		atmosConfig: &schema.AtmosConfiguration{
			Auth:         *authCfg,
			Stores:       storepkg.StoreRegistry{},
			StoresConfig: storepkg.StoresConfig{},
		},
		info: &schema.ConfigAndStacksInfo{
			Stack:       isolationStack,
			Identity:    identityFlag,
			AuthManager: manager,
			AuthContext: manager.GetStackInfo().AuthContext,
		},
		chainBefore: slices.Clone(manager.GetChain()),
	}

	require.NotEmpty(t, h.chainBefore)
	require.Equal(t, isolationDeployer, h.chainBefore[len(h.chainBefore)-1], "precondition: component identity authenticated")
	require.Equal(t, isolationDeployer, h.info.AuthContext.AWS.Profile, "precondition: deployer auth context")

	for name, pinned := range storePins {
		s := &isolationStore{MockIdentityAwareStore: storepkg.NewMockIdentityAwareStore(ctrl), pinned: pinned}
		h.stores[name] = s
		h.atmosConfig.Stores[name] = s
		h.atmosConfig.StoresConfig[name] = storepkg.StoreConfig{Type: "aws-ssm-parameter-store", Secret: true, Identity: pinned}
	}

	return h
}

// readStore simulates `!secret` resolving against the named store: the store resolves its effective
// identity through the injected resolver and the profile it receives must be that identity's profile.
func (h *isolationHarness) readStore(name string) {
	h.t.Helper()

	s := h.stores[name]
	require.NotNil(h.t, s, "unknown store %q", name)
	require.NotNil(h.t, s.resolver, "store %q must have been handed a resolver", name)

	want := s.effectiveIdentity()
	require.NotEmpty(h.t, want, "store %q must resolve to an identity", name)

	var gotProfile string
	s.EXPECT().Get(isolationStack, isolationComponent, name).DoAndReturn(func(string, string, string) (any, error) {
		awsCfg, err := s.resolver.ResolveAWSAuthContext(context.Background(), want)
		require.NoError(h.t, err)
		require.NotNil(h.t, awsCfg)
		gotProfile = awsCfg.Profile
		return "value-from-" + name, nil
	}).Times(1)

	value, err := s.Get(isolationStack, isolationComponent, name)
	require.NoError(h.t, err)
	assert.Equal(h.t, "value-from-"+name, value)
	assert.Equal(h.t, want, gotProfile, "store %q must read as its own identity", name)
}

// assertComponentIdentityIntact simulates what nested !terraform.state reads consume and asserts each
// still sees the component identity, not whichever identity a store authenticated as.
func (h *isolationHarness) assertComponentIdentityIntact() {
	h.t.Helper()

	// Auth chain on the shared manager.
	chain := h.manager.GetChain()
	assert.Equal(h.t, h.chainBefore, chain, "shared manager chain must not change after store reads")
	require.NotEmpty(h.t, chain)
	assert.Equal(h.t, isolationDeployer, chain[len(chain)-1])

	// Auth context on the shared manager and on the info that GetTerraformState receives.
	managerCtx := h.manager.GetStackInfo().AuthContext
	require.NotNil(h.t, managerCtx)
	require.NotNil(h.t, managerCtx.AWS)
	assert.Equal(h.t, isolationDeployer, managerCtx.AWS.Profile)
	require.NotNil(h.t, h.info.AuthContext)
	require.NotNil(h.t, h.info.AuthContext.AWS)
	assert.Equal(h.t, isolationDeployer, h.info.AuthContext.AWS.Profile)

	// terraform_state_utils derives the backend-read context through resolvedTargetAuthContext.
	backendCtx := testExecutor(h.t).services.ResolvedTargetAuthContext(h.atmosConfig, h.manager, h.info.AuthContext, false)
	require.NotNil(h.t, backendCtx)
	require.NotNil(h.t, backendCtx.AWS)
	assert.Equal(h.t, isolationDeployer, backendCtx.AWS.Profile)

	// Identity auto-detection used by hooks and nested operations.
	detected := &schema.ConfigAndStacksInfo{Stack: isolationStack}
	testExecutor(h.t).services.StoreAutoDetectedIdentity(h.manager, detected)
	assert.Equal(h.t, isolationDeployer, detected.Identity)
	assert.Equal(h.t, isolationDeployer, testExecutor(h.t).HookStoreDefaultIdentity(h.manager, &schema.ConfigAndStacksInfo{Stack: isolationStack}))

	// Nested target inheritance: createComponentAuthManager inherits the last element of the parent chain.
	nested, err := CreateComponentAuthManager(h.atmosConfig, map[string]any{}, "nested", isolationStack, h.manager)
	require.NoError(h.t, err)
	require.NotNil(h.t, nested)
	nestedChain := nested.GetChain()
	require.NotEmpty(h.t, nestedChain)
	assert.Equal(h.t, isolationDeployer, nestedChain[len(nestedChain)-1], "nested target must inherit the component identity")
	require.NotNil(h.t, nested.GetStackInfo().AuthContext)
	require.NotNil(h.t, nested.GetStackInfo().AuthContext.AWS)
	assert.Equal(h.t, isolationDeployer, nested.GetStackInfo().AuthContext.AWS.Profile)
}

func (h *isolationHarness) run(steps []string) {
	h.t.Helper()
	for _, step := range steps {
		if step == stepState {
			h.assertComponentIdentityIntact()
			continue
		}
		name, ok := strings.CutPrefix(step, stepSecretPrefix)
		require.True(h.t, ok, "unknown step %q", step)
		h.readStore(name)
	}
}

func TestStoreIdentityIsolation(t *testing.T) {
	const ssm = "secrets/ssm"
	const other = "secrets/other"

	tests := []struct {
		name         string
		identityFlag string
		pins         map[string]string
		steps        []string
		wantBound    map[string]string
		wantInfoID   string
		wantRequest  string
	}{
		{
			name:         "state then secret then state with default identity",
			identityFlag: "",
			pins:         map[string]string{ssm: "automation"},
			steps:        []string{stepState, stepSecretPrefix + ssm, stepState},
			wantBound:    map[string]string{ssm: ""},
			wantInfoID:   isolationDeployer,
			wantRequest:  "",
		},
		{
			name:         "secret then state reversed ordering",
			identityFlag: "",
			pins:         map[string]string{ssm: "automation"},
			steps:        []string{stepSecretPrefix + ssm, stepState},
			wantBound:    map[string]string{ssm: ""},
			wantInfoID:   isolationDeployer,
			wantRequest:  "",
		},
		{
			name:         "explicit identity flag",
			identityFlag: isolationDeployer,
			pins:         map[string]string{ssm: "automation"},
			steps:        []string{stepState, stepSecretPrefix + ssm, stepState, stepSecretPrefix + ssm, stepState},
			wantBound:    map[string]string{ssm: ""},
			wantInfoID:   isolationDeployer,
			wantRequest:  isolationDeployer,
		},
		{
			name:         "two stores pinned to different identities",
			identityFlag: "",
			pins:         map[string]string{ssm: "automation", other: "auditor"},
			steps:        []string{stepSecretPrefix + ssm, stepState, stepSecretPrefix + other, stepState, stepSecretPrefix + ssm, stepState},
			wantBound:    map[string]string{ssm: "", other: ""},
			wantInfoID:   isolationDeployer,
			wantRequest:  "",
		},
		{
			name:         "store inheriting the component identity",
			identityFlag: "",
			pins:         map[string]string{ssm: "", other: "automation"},
			steps:        []string{stepSecretPrefix + ssm, stepState, stepSecretPrefix + other, stepState},
			wantBound:    map[string]string{ssm: isolationDeployer, other: ""},
			wantInfoID:   isolationDeployer,
			wantRequest:  "",
		},
		{
			name:         "explicit identity inherited by identity-less store",
			identityFlag: isolationDeployer,
			pins:         map[string]string{ssm: ""},
			steps:        []string{stepSecretPrefix + ssm, stepState},
			wantBound:    map[string]string{ssm: isolationDeployer},
			wantInfoID:   isolationDeployer,
			wantRequest:  isolationDeployer,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newIsolationHarness(t, tt.identityFlag, tt.pins)

			testExecutor(t).injectTerraformStoreAuthResolver(h.atmosConfig, h.info, h.manager)

			assert.Equal(t, tt.wantInfoID, h.info.Identity)
			require.NotNil(t, h.info.RequestedIdentity)
			assert.Equal(t, tt.wantRequest, *h.info.RequestedIdentity)
			require.NotNil(t, h.atmosConfig.SecretsAuth)
			assert.NotNil(t, h.atmosConfig.SecretsAuth.Resolver)
			for name, wantBound := range tt.wantBound {
				s := h.stores[name]
				assert.NotNil(t, s.resolver, "store %q receives the resolver", name)
				assert.Equal(t, wantBound, s.bound, "identity bound to store %q", name)
				assert.Equal(t, 1, s.bindings, "store %q is bound exactly once", name)
			}

			h.run(tt.steps)

			// Each secret step registers Get with Times(1), so the resolver is exercised exactly once per read.
		})
	}
}

// TestStoreIdentityIsolation_AuthDisabled asserts what injectTerraformStoreAuthResolver actually does
// when auth is disabled (--identity=false): setupTerraformAuth returns a nil manager, and no resolver
// reaches any store.
func TestStoreIdentityIsolation_AuthDisabled(t *testing.T) {
	const ssm = "secrets/ssm"

	t.Run("nil manager binds no resolver", func(t *testing.T) {
		h := newIsolationHarness(t, cfg.IdentityFlagDisabledValue, map[string]string{ssm: "automation", "secrets/inherit": ""})

		testExecutor(t).injectTerraformStoreAuthResolver(h.atmosConfig, h.info, nil)

		for name, s := range h.stores {
			assert.Nil(t, s.resolver, "store %q must not receive a resolver", name)
			assert.Equal(t, 0, s.bindings, "SetAuthContext must not be called for store %q", name)
			assert.Empty(t, s.bound)
		}
		assert.Nil(t, h.atmosConfig.SecretsAuth)
		assert.Empty(t, resolveStoreDefaultIdentity(h.info.Identity, nil))
		assert.Empty(t, testExecutor(t).HookStoreDefaultIdentity(nil, h.info))
		assert.Equal(t, cfg.IdentityFlagDisabledValue, h.info.Identity, "identity stays disabled")
	})

	t.Run("disabled identity with a manager keeps identity-less stores on the SDK chain", func(t *testing.T) {
		h := newIsolationHarness(t, cfg.IdentityFlagDisabledValue, map[string]string{ssm: "automation", "secrets/inherit": ""})

		testExecutor(t).injectTerraformStoreAuthResolver(h.atmosConfig, h.info, h.manager)

		assert.Empty(t, resolveStoreDefaultIdentity(cfg.IdentityFlagDisabledValue, h.manager))
		for name, s := range h.stores {
			require.NotNil(t, s.resolver, "store %q receives the resolver", name)
			assert.Empty(t, s.bound, "no component identity is inherited when auth is disabled (store %q)", name)
		}

		// A pinned store still reads as its pinned identity without touching the shared manager.
		h.readStore(ssm)
		chain := h.manager.GetChain()
		assert.Equal(t, h.chainBefore, chain)
		assert.Equal(t, isolationDeployer, h.manager.GetStackInfo().AuthContext.AWS.Profile)
	})
}

// legacyResolver reproduces the pre-fix resolver behavior (issue #3544): it authenticates the store
// identity on the shared manager and reads the result back from the manager's own stack info.
type legacyResolver struct {
	storepkg.AuthContextResolver
	manager authtypes.AuthManager
}

func (r *legacyResolver) ResolveAWSAuthContext(ctx context.Context, identityName string) (*storepkg.AWSAuthConfig, error) {
	if _, err := r.manager.Authenticate(ctx, identityName); err != nil {
		return nil, err
	}
	awsCtx := r.manager.GetStackInfo().AuthContext.AWS
	return &storepkg.AWSAuthConfig{Profile: awsCtx.Profile}, nil
}

// TestStoreIdentityIsolation_HarnessDetectsSharedManagerLeak proves the harness is sensitive to the
// bug: when stores authenticate on the shared manager (the pre-fix behavior), the component identity
// observed by nested reads flips to the store identity.
func TestStoreIdentityIsolation_HarnessDetectsSharedManagerLeak(t *testing.T) {
	const ssm = "secrets/ssm"
	h := newIsolationHarness(t, "", map[string]string{ssm: "automation"})
	testExecutor(t).injectTerraformStoreAuthResolver(h.atmosConfig, h.info, h.manager)

	// Swap in the legacy behavior while keeping the identity binding the real injection produced.
	s := h.stores[ssm]
	s.SetAuthContext(&legacyResolver{manager: h.manager}, s.bound)

	h.readStore(ssm)

	chain := h.manager.GetChain()
	require.NotEmpty(t, chain)
	assert.Equal(t, "automation", chain[len(chain)-1], "legacy resolver overwrites the shared chain")
	assert.Equal(t, "automation", h.manager.GetStackInfo().AuthContext.AWS.Profile, "legacy resolver overwrites the shared auth context")

	detected := &schema.ConfigAndStacksInfo{Stack: isolationStack}
	testExecutor(h.t).services.StoreAutoDetectedIdentity(h.manager, detected)
	assert.Equal(t, "automation", detected.Identity)

	nested, err := CreateComponentAuthManager(h.atmosConfig, map[string]any{}, "nested", isolationStack, h.manager)
	require.NoError(t, err)
	require.NotNil(t, nested)
	nestedChain := nested.GetChain()
	require.NotEmpty(t, nestedChain)
	assert.Equal(t, "automation", nestedChain[len(nestedChain)-1], "nested target inherits the leaked identity")
}
