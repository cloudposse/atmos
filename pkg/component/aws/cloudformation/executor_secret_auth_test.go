package cloudformation

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth"
	authdeferred "github.com/cloudposse/atmos/pkg/auth/deferred"
	authtypes "github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/component"
	"github.com/cloudposse/atmos/pkg/schema"
	secretdeferred "github.com/cloudposse/atmos/pkg/secrets/deferred"
	"github.com/cloudposse/atmos/pkg/store"
)

// Secret evaluation precedes CloudFormation authentication. Exercise the real
// deferred secret resolver at that boundary, including successive identities.
func TestExecuteSingleSecretAuthIsScoped(t *testing.T) {
	ctrl := gomock.NewController(t)
	backend := store.NewMockIdentityAwareStore(ctrl)
	original := &schema.AtmosConfiguration{
		Stores:       store.StoreRegistry{"vault": backend},
		StoresConfig: store.StoresConfig{"vault": {Secret: true, Identity: "store-role"}},
		Auth: schema.AuthConfig{Identities: map[string]schema.Identity{
			"store-role": {Kind: "aws/user"}, "cli-role": {Kind: "aws/user", Default: true},
		}},
	}
	installExecutorSeamStubs(t, executorSeamStubs{
		processStacks: func(ac *schema.AtmosConfiguration, info schema.ConfigAndStacksInfo, _, _, _ bool, _ []string, _ auth.AuthManager) (schema.ConfigAndStacksInfo, error) {
			require.True(t, authdeferred.IsDeferred(ac.AuthManager), "secrets need deferred auth before stack evaluation")
			require.NotSame(t, original, ac)
			factory := authdeferred.NewMockAuthFactory(ctrl)
			ac.AuthManager = authdeferred.NewManager(authdeferred.AuthOptions{Factory: factory})
			manager := authtypes.NewMockAuthManager(ctrl)
			manager.EXPECT().GetChain().Return([]string{"store-role"}).AnyTimes()
			manager.EXPECT().GetStackInfo().Return(&schema.ConfigAndStacksInfo{AuthContext: &schema.AuthContext{AWS: &schema.AWSAuthContext{Profile: info.Stack}}}).AnyTimes()
			factory.EXPECT().Create(gomock.Any(), gomock.Cond(func(config *schema.AuthConfig) bool {
				return config.Identities["store-role"].Default && !config.Identities["cli-role"].Default
			}), info.Stack).Return(manager, nil)
			backend.EXPECT().ResetAuthContext()
			backend.EXPECT().SetAuthContext(gomock.Any(), "store-role").Do(func(resolver store.AuthContextResolver, identity string) {
				credentials, err := resolver.ResolveAWSAuthContext(t.Context(), identity)
				require.NoError(t, err)
				require.Equal(t, info.Stack, credentials.Profile)
			})
			backend.EXPECT().Get(info.Stack, "consumer", "TOKEN").Return("synthetic-canary", nil)
			info.ComponentSection = map[string]any{"secrets": map[string]any{"vars": map[string]any{"TOKEN": map[string]any{"store": "vault"}}}}
			value, err := secretdeferred.NewValue(ac, "!secret TOKEN", info.Stack, &info).Resolve()
			require.NoError(t, err)
			require.Equal(t, "synthetic-canary", value)
			info.ComponentIsEnabled = false // The stack evaluation is the boundary under test.
			return info, nil
		},
	})
	for _, stack := range []string{"first-account", "second-account"} {
		info := &schema.ConfigAndStacksInfo{Stack: stack, ComponentFromArg: "consumer", Identity: "cli-role"}
		require.NoError(t, executeSingle(&component.ExecutionContext{}, original, info, OperationApply))
		require.Nil(t, original.AuthManager)
		require.True(t, original.Auth.Identities["cli-role"].Default)
		require.False(t, original.Auth.Identities["store-role"].Default)
	}
}

// TestSourceAWSAuthResolvesOnlyWhenRequested checks lazy source credentials, disabled authentication and preservation of existing contexts.
func TestSourceAWSAuthResolvesOnlyWhenRequested(t *testing.T) {
	for _, scenario := range []string{"default", "explicit", "dry-run", "disabled", "existing", "failure"} {
		t.Run(scenario, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			factory := authdeferred.NewMockAuthFactory(ctrl)
			ac := &schema.AtmosConfiguration{
				AuthManager: authdeferred.NewManager(authdeferred.AuthOptions{Factory: factory}),
				Auth: schema.AuthConfig{Identities: map[string]schema.Identity{
					"default": {Kind: "aws/user", Default: true}, "explicit": {Kind: "aws/user"},
				}},
			}
			info := &schema.ConfigAndStacksInfo{Stack: "dev", DryRun: scenario == "dry-run", AuthDisabled: scenario == "disabled"}
			expected := "default"
			if scenario == "explicit" {
				expected, info.Identity = "explicit", "explicit"
			}
			credentials := &schema.AWSAuthContext{Profile: expected}
			if scenario == "existing" {
				info.AuthContext = &schema.AuthContext{AWS: credentials}
			} else if scenario != "dry-run" && scenario != "disabled" {
				if scenario == "failure" {
					factory.EXPECT().Create(gomock.Any(), gomock.Any(), "dev").Return(nil, errUtils.ErrAuthenticationFailed)
				} else {
					manager := authtypes.NewMockAuthManager(ctrl)
					manager.EXPECT().GetStackInfo().Return(&schema.ConfigAndStacksInfo{AuthContext: &schema.AuthContext{AWS: credentials}}).AnyTimes()
					factory.EXPECT().Create(gomock.Any(), gomock.Cond(func(config *schema.AuthConfig) bool { return config.Identities[expected].Default }), "dev").Return(manager, nil)
				}
			}
			got, err := resolveSourceAWSAuth(ac, info)
			if scenario == "failure" {
				require.ErrorIs(t, err, errUtils.ErrAuthenticationFailed)
				require.Nil(t, info.AuthContext)
				return
			}
			require.NoError(t, err)
			if scenario == "dry-run" || scenario == "disabled" {
				require.Nil(t, got)
			} else {
				require.Same(t, credentials, got)
				require.Same(t, credentials, info.AuthContext.AWS)
				again, err := resolveSourceAWSAuth(ac, info)
				require.NoError(t, err)
				require.Same(t, got, again)
			}
			require.True(t, ac.Auth.Identities["default"].Default)
		})
	}
}
