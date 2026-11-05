package authbridge_test

import (
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/cloudposse/atmos/pkg/auth"
	"github.com/cloudposse/atmos/pkg/auth/credentials"
	"github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/store/authbridge"
)

// Compile-time sentinels for the schema fields referenced by these tests.
var (
	_ = schema.Provider{Kind: "mock/aws"}
	_ = schema.Identity{Kind: "mock/aws", Default: true, Via: &schema.IdentityVia{Provider: "mock"}}
	_ = schema.AWSAuthContext{Profile: ""}
)

// isolationTestManager builds a real manager backed by the mock/aws provider and two
// identities: "deployer" (default) and "automation".
func isolationTestManager(t *testing.T) types.AuthManager {
	t.Helper()

	root := t.TempDir()
	t.Setenv("HOME", root)
	t.Setenv("XDG_CONFIG_HOME", root)
	t.Setenv("XDG_CACHE_HOME", root)
	t.Setenv("XDG_DATA_HOME", root)

	cfg := &schema.AuthConfig{
		Realm:     "resolver-isolation-test",
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

	manager, err := auth.NewAuthManager(cfg, credentials.NewCredentialStoreWithConfig(cfg), validator, &schema.ConfigAndStacksInfo{
		Stack:           "dev",
		ProfilesFromArg: []string{"one"},
	}, "")
	require.NoError(t, err)

	return manager
}

// TestResolver_DoesNotMutateParentManager is a regression test for issue #3544: resolving a store
// identity must not change the component manager's chain or AWS auth context.
func TestResolver_DoesNotMutateParentManager(t *testing.T) {
	manager := isolationTestManager(t)

	_, err := manager.Authenticate(t.Context(), "deployer")
	require.NoError(t, err)

	chainBefore := slices.Clone(manager.GetChain())
	require.NotEmpty(t, chainBefore)
	require.Equal(t, "deployer", chainBefore[len(chainBefore)-1])

	stackInfo := manager.GetStackInfo()
	require.NotNil(t, stackInfo.AuthContext)
	require.NotNil(t, stackInfo.AuthContext.AWS)
	awsBefore := *stackInfo.AuthContext.AWS
	require.Equal(t, "deployer", awsBefore.Profile)

	resolver := authbridge.NewResolver(manager, stackInfo)

	cfg, err := resolver.ResolveAWSAuthContext(t.Context(), "automation")
	require.NoError(t, err)
	require.NotNil(t, cfg)
	assert.Equal(t, "automation", cfg.Profile)

	chainAfter := manager.GetChain()
	assert.Equal(t, chainBefore, chainAfter, "manager chain must not change")
	assert.Equal(t, "deployer", chainAfter[len(chainAfter)-1])

	afterInfo := manager.GetStackInfo()
	require.NotNil(t, afterInfo.AuthContext)
	require.NotNil(t, afterInfo.AuthContext.AWS)
	assert.Equal(t, awsBefore, *afterInfo.AuthContext.AWS, "manager AWS auth context must not change")
}
