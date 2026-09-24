package exec

import (
	"errors"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/cloudposse/atmos/pkg/deferred"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth"
	"github.com/cloudposse/atmos/pkg/auth/types"
	awsIdentity "github.com/cloudposse/atmos/pkg/aws/identity"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/degradation"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestDeferredAuthPipeline(t *testing.T) {
	t.Chdir(filepath.Join("..", "..", "tests", "fixtures", "scenarios", "list-deferred-auth"))
	for _, mode := range []string{"warn", "silent", "strict"} {
		for _, field := range []string{"literal", "account", "state", "template"} {
			t.Run(mode+"/"+field, func(t *testing.T) {
				ClearFindStacksMapCache()
				ac, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, true)
				require.NoError(t, err)
				deferred.ConfigureAuth(&ac, "")
				ctrl := gomock.NewController(t)
				factory := deferred.NewMockAuthFactory(ctrl)
				setDeferredAuthFactory(&ac, factory)
				oldOutputs := componentFuncOutputsExecutor
				t.Cleanup(func() { componentFuncOutputsExecutor = oldOutputs })
				componentFuncOutputsExecutor = NewMockComponentFuncOutputsExecutor(ctrl)
				awsIdentity.ClearIdentityCache()
				t.Cleanup(awsIdentity.SetGetter(awsIdentity.NewMockGetter(ctrl)))
				if field != "literal" {
					factory.EXPECT().Create(gomock.Any(), gomock.Any(), "dev").Return(nil, unavailableAuth(errUtils.ErrEmulatorNotRunning)).Times(1)
				}
				opts, collector := ErrorOptionsFromMode(mode)
				opts.EvaluationPaths = [][]string{{"vars", field}}
				result, err := ExecuteDescribeStacksWithEvalSections(&ac, "dev", nil, nil, nil, false, true, true, false, nil, nil, false, nil, nil, opts, []string{"vars"})
				if field != "literal" && mode == "strict" {
					require.ErrorIs(t, err, errUtils.ErrEmulatorNotRunning)
					return
				}
				require.NoError(t, err)
				vars := result["dev"].(map[string]any)["components"].(map[string]any)["terraform"].(map[string]any)["example"].(map[string]any)["vars"].(map[string]any)
				if field == "literal" {
					assert.Equal(t, "preserved", vars[field])
					if collector != nil {
						assert.Zero(t, collector.Count())
					}
				} else {
					assert.Equal(t, degradation.AtmosComputedValue{}, vars[field])
					assert.Equal(t, 1, collector.Count())
				}
			})
		}
	}
}

func TestDeferredAuthInheritedAndOverridden(t *testing.T) {
	ctrl := gomock.NewController(t)
	parent := types.NewMockAuthManager(ctrl)
	parentSection := map[string]any{"auth": map[string]any{"identities": map[string]any{"parent": map[string]any{"kind": "aws/emulator", "default": true, "emulator": "aws"}}}}
	parent.EXPECT().GetStackInfo().Return(&schema.ConfigAndStacksInfo{ComponentSection: parentSection}).AnyTimes()
	ac := &schema.AtmosConfiguration{}
	deferred.ConfigureAuth(ac, "")
	factory := deferred.NewMockAuthFactory(ctrl)
	setDeferredAuthFactory(ac, factory)
	for _, name := range []string{"parent", "target"} {
		section := map[string]any{}
		if name == "target" {
			section["auth"] = map[string]any{"identities": map[string]any{"target": map[string]any{"kind": "aws/emulator", "default": true, "emulator": "aws"}}}
		}
		section = inheritDeferredAuth(section, []auth.AuthManager{parent})
		factory.EXPECT().Create(ac, gomock.Any(), "dev").DoAndReturn(func(_ *schema.AtmosConfiguration, config *schema.AuthConfig, _ string) (auth.AuthManager, error) {
			require.True(t, config.Identities[name].Default)
			return nil, unavailableAuth(errUtils.ErrExpiredCredentials)
		}).Times(1)
		for range 2 {
			err := deferred.ResolveAuth(ac, &schema.ConfigAndStacksInfo{Stack: "dev", ComponentSection: section})
			require.ErrorIs(t, err, errUtils.ErrAuthenticationUnavailable)
		}
	}
}

func TestDeferredAuthExplicitAndFatalErrors(t *testing.T) {
	ac := templatingEnabledConfig()
	assert.False(t, deferred.ConfigureAuth(ac, "local"))
	assert.Nil(t, ac.DeferredAuth)
	deferred.ConfigureAuth(ac, "")
	for _, value := range []string{`{{ fail "invalid expression" }}`, `{{ .vars.malformed `} {
		info := &schema.ConfigAndStacksInfo{Stack: "dev"}
		opts, _ := ErrorOptionsFromMode("warn")
		_, err := processComponentSectionTemplates(ac, info, map[string]any{"vars": map[string]any{"value": value}}, nil, nil, opts.OnWarning)
		require.Error(t, err)
	}
	ClearResolutionContext()
	t.Cleanup(ClearResolutionContext)
	ctx := GetOrCreateResolutionContext()
	require.NoError(t, ctx.Push(ac, DependencyNode{Component: "example", Stack: "dev", FunctionType: "atmos.Component"}))
	_, err := componentFunc(ac, &schema.ConfigAndStacksInfo{}, "example", "dev")
	require.ErrorIs(t, err, errUtils.ErrCircularDependency)
}

func TestDeferredAuthBackendFailureAndSuccess(t *testing.T) {
	for _, failure := range []bool{false, true} {
		t.Run(fmt.Sprint(failure), func(t *testing.T) {
			ctrl := gomock.NewController(t)
			ac := templatingEnabledConfig()
			deferred.ConfigureAuth(ac, "")
			factory := deferred.NewMockAuthFactory(ctrl)
			setDeferredAuthFactory(ac, factory)
			manager := types.NewMockAuthManager(ctrl)
			authContext := &schema.AuthContext{AWS: &schema.AWSAuthContext{Profile: "configured"}}
			manager.EXPECT().GetStackInfo().Return(&schema.ConfigAndStacksInfo{AuthContext: authContext}).AnyTimes()
			factory.EXPECT().Create(ac, gomock.Any(), "dev").Return(manager, nil).Times(1)
			awsIdentity.ClearIdentityCache()
			getter := awsIdentity.NewMockGetter(ctrl)
			t.Cleanup(awsIdentity.SetGetter(getter))
			call := getter.EXPECT().GetCallerIdentity(gomock.Any(), ac, authContext.AWS)
			if failure {
				call.Return(nil, unavailableAuth(errUtils.ErrExpiredCredentials)).AnyTimes()
			} else {
				call.Return(&awsIdentity.CallerIdentity{Account: "123456789012"}, nil).Times(1)
			}
			info := &schema.ConfigAndStacksInfo{Stack: "dev"}
			opts, collector := ErrorOptionsFromMode("warn")
			result, err := processComponentSectionYAMLFunctions(ac, info, map[string]any{"account": "!aws.account_id"}, nil, opts.OnWarning, true, nil)
			require.NoError(t, err)
			if failure {
				assert.Equal(t, degradation.AtmosComputedValue{}, result["account"])
				assert.Equal(t, 1, collector.Count())
			} else {
				assert.Equal(t, "123456789012", result["account"])
				assert.Zero(t, collector.Count())
			}
		})
	}
}

func TestDeferredAuthCacheAndFailure(t *testing.T) {
	for _, failure := range []bool{false, true} {
		t.Run(fmt.Sprint(failure), func(t *testing.T) {
			ctrl := gomock.NewController(t)
			factory := deferred.NewMockAuthFactory(ctrl)
			ac := &schema.AtmosConfiguration{}
			require.True(t, deferred.ConfigureAuth(ac, ""))
			setDeferredAuthFactory(ac, factory)
			manager := types.NewMockAuthManager(ctrl)
			context := &schema.AuthContext{AWS: &schema.AWSAuthContext{Profile: "configured"}}
			manager.EXPECT().GetStackInfo().Return(&schema.ConfigAndStacksInfo{AuthContext: context}).AnyTimes()
			for _, stack := range []string{"dev", "prod"} {
				call := factory.EXPECT().Create(ac, gomock.Any(), stack).Times(1)
				if failure {
					call.Return(nil, unavailableAuth(errUtils.ErrEmulatorNotRunning))
				} else {
					call.Return(manager, nil)
				}
				for range 2 {
					info := &schema.ConfigAndStacksInfo{Stack: stack}
					err := deferred.ResolveAuth(ac, info)
					if failure {
						require.ErrorIs(t, err, errUtils.ErrAuthenticationUnavailable)
						require.ErrorIs(t, err, errUtils.ErrEmulatorNotRunning)
						assert.Nil(t, info.AuthManager)
						assert.Nil(t, info.AuthContext)
					} else {
						require.NoError(t, err)
						assert.Same(t, context, info.AuthContext)
					}
				}
			}
		})
	}
}

func TestDeferredAuthDoesNotEnableTerraformDefaults(t *testing.T) {
	assert.False(t, isRecoverableTerraformError(errUtils.ErrAuthenticationUnavailable), "YQ defaults must not hide failed auth")
}

func setDeferredAuthFactory(ac *schema.AtmosConfiguration, factory deferred.AuthFactory) {
	ac.DeferredAuth = deferred.NewAuthResolver(deferred.AuthOptions{Disabled: deferred.AuthDisabled(ac), Factory: factory})
}

func unavailableAuth(err error) error {
	return errors.Join(errUtils.ErrAuthenticationUnavailable, err)
}

func TestDeferredAuthYAMLDegradation(t *testing.T) {
	for _, mode := range []string{"warn", "silent", "strict"} {
		t.Run(mode, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			factory := deferred.NewMockAuthFactory(ctrl)
			ac := &schema.AtmosConfiguration{}
			deferred.ConfigureAuth(ac, "")
			setDeferredAuthFactory(ac, factory)
			factory.EXPECT().Create(ac, gomock.Any(), "dev").Return(nil, unavailableAuth(errUtils.ErrEmulatorNotRunning)).Times(1)
			info := &schema.ConfigAndStacksInfo{Stack: "dev", Component: "example"}
			input := map[string]any{"vars": map[string]any{"literal": "preserved", "account": "!aws.account_id", "arn": "!aws.caller_identity_arn"}}
			errOpts, collector := ErrorOptionsFromMode(mode)
			got, err := processComponentSectionYAMLFunctions(ac, info, input, nil, errOpts.OnWarning, true, nil)
			if mode == "strict" {
				require.ErrorIs(t, err, errUtils.ErrEmulatorNotRunning)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, map[string]any{"literal": "preserved", "account": degradation.AtmosComputedValue{}, "arn": degradation.AtmosComputedValue{}}, got["vars"])
			assert.Equal(t, 2, collector.Count())
		})
	}
}

func TestDeferredAuthUnusedAndDisabled(t *testing.T) {
	ctrl := gomock.NewController(t)
	factory := deferred.NewMockAuthFactory(ctrl)
	ac := &schema.AtmosConfiguration{}
	deferred.ConfigureAuth(ac, "")
	setDeferredAuthFactory(ac, factory)
	input := map[string]any{"vars": map[string]any{"account": "!aws.account_id"}}
	got, err := processComponentSectionYAMLFunctions(ac, &schema.ConfigAndStacksInfo{}, input, nil, nil, true, []string{})
	require.NoError(t, err)
	assert.Equal(t, input, got)
	deferred.ConfigureAuth(ac, "false")
	info := &schema.ConfigAndStacksInfo{}
	require.NoError(t, deferred.ResolveAuth(ac, info))
	assert.True(t, info.AuthDisabled)
}

func TestDeferredAuthTemplateDegradation(t *testing.T) {
	ctrl := gomock.NewController(t)
	factory := deferred.NewMockAuthFactory(ctrl)
	ac := templatingEnabledConfig()
	deferred.ConfigureAuth(ac, "")
	setDeferredAuthFactory(ac, factory)
	factory.EXPECT().Create(ac, gomock.Any(), "dev").Return(nil, unavailableAuth(errUtils.ErrExpiredCredentials)).Times(1)
	info := &schema.ConfigAndStacksInfo{Stack: "dev", Component: "example"}
	input := map[string]any{"vars": map[string]any{
		"literal": "preserved",
		"account": `{{ atmos.Resolve "!aws.account_id" }}`,
		"nested":  []any{`{{ atmos.Resolve "!aws.caller_identity_arn" }}`, "unchanged"},
	}}
	errOpts, collector := ErrorOptionsFromMode("warn")
	got, err := processComponentSectionTemplates(ac, info, input, nil, nil, errOpts.OnWarning)
	require.NoError(t, err)
	assert.Equal(t, map[string]any{
		"literal": "preserved", "account": degradation.AtmosComputedValue{},
		"nested": []any{degradation.AtmosComputedValue{}, "unchanged"},
	}, got["vars"])
	assert.Equal(t, 2, collector.Count())
}
