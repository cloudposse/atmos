package terraform

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth"
	authtypes "github.com/cloudposse/atmos/pkg/auth/types"
	cfg "github.com/cloudposse/atmos/pkg/config"
	atmosio "github.com/cloudposse/atmos/pkg/io"
	"github.com/cloudposse/atmos/pkg/schema"
	tfcache "github.com/cloudposse/atmos/pkg/terraform/cache"
	"github.com/cloudposse/atmos/pkg/terraform/rc"
)

func TestSetLastAuthContext_RoundTrips(t *testing.T) {
	t.Cleanup(ClearLastAuthContext)

	authCtx := &schema.AuthContext{
		AWS: &schema.AWSAuthContext{
			Profile:         "test-profile",
			CredentialsFile: "/tmp/creds",
			Region:          "us-east-1",
		},
	}
	authMgr := "mock-manager"

	SetLastAuthContext(authCtx, authMgr)

	gotCtx, gotMgr := GetLastAuthContext()
	require.NotNil(t, gotCtx)
	assert.Equal(t, "test-profile", gotCtx.AWS.Profile)
	assert.Equal(t, "us-east-1", gotCtx.AWS.Region)
	assert.Equal(t, "mock-manager", gotMgr)
}

func TestGetLastAuthContext_ReturnsNilWhenUnset(t *testing.T) {
	t.Cleanup(ClearLastAuthContext)
	ClearLastAuthContext()

	gotCtx, gotMgr := GetLastAuthContext()
	assert.Nil(t, gotCtx)
	assert.Nil(t, gotMgr)
}

func TestClearLastAuthContext_ResetsState(t *testing.T) {
	authCtx := &schema.AuthContext{
		AWS: &schema.AWSAuthContext{Profile: "before-clear"},
	}
	SetLastAuthContext(authCtx, "mgr")

	ClearLastAuthContext()

	gotCtx, gotMgr := GetLastAuthContext()
	assert.Nil(t, gotCtx)
	assert.Nil(t, gotMgr)
}

func TestSetLastAuthContext_OverwritesPrevious(t *testing.T) {
	t.Cleanup(ClearLastAuthContext)

	first := &schema.AuthContext{
		AWS: &schema.AWSAuthContext{Profile: "first"},
	}
	second := &schema.AuthContext{
		AWS: &schema.AWSAuthContext{Profile: "second"},
	}

	SetLastAuthContext(first, "mgr1")
	SetLastAuthContext(second, "mgr2")

	gotCtx, gotMgr := GetLastAuthContext()
	require.NotNil(t, gotCtx)
	assert.Equal(t, "second", gotCtx.AWS.Profile)
	assert.Equal(t, "mgr2", gotMgr)
}

// TestResolveStoreDefaultIdentityFallback verifies explicit identities win and missing defaults preserve ambient credentials.
func TestResolveStoreDefaultIdentityFallback(t *testing.T) {
	for _, tc := range []struct {
		name, identity, defaultIdentity, want string
		lookupErr                             error
		lookup                                bool
	}{
		{name: "disabled does not authenticate", identity: cfg.IdentityFlagDisabledValue},
		{name: "explicit bypasses defaults", identity: "deployment", want: "deployment"},
		{name: "empty selects stack default", defaultIdentity: "stack-default", want: "stack-default", lookup: true},
		{name: "selection uses stack default", identity: cfg.IdentityFlagSelectValue, defaultIdentity: "stack-default", want: "stack-default", lookup: true},
		{name: "missing default keeps ambient credentials", lookupErr: errUtils.ErrInvalidAuthConfig, lookup: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			manager := authtypes.NewMockAuthManager(gomock.NewController(t))
			if tc.lookup {
				manager.EXPECT().GetDefaultIdentity(false).Return(tc.defaultIdentity, tc.lookupErr)
			}
			assert.Equal(t, tc.want, resolveStoreDefaultIdentity(tc.identity, manager))
		})
	}
	assert.Empty(t, resolveStoreDefaultIdentity("", nil))
}

// TestSetupTerraformAuthResolvesConfigurationFailures verifies auth failures retain recovery context and error sentinels.
func TestSetupTerraformAuthResolvesConfigurationFailures(t *testing.T) {
	for _, tc := range []struct {
		name         string
		creatorFails bool
		wantSentinel error
	}{
		{name: "merged config", wantSentinel: errUtils.ErrInvalidAuthConfig},
		{name: "manager creation", creatorFails: true, wantSentinel: errUtils.ErrFailedToInitializeAuthManager},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cause := errors.New("identity configuration unavailable")
			config := &schema.AtmosConfiguration{}
			info := &schema.ConfigAndStacksInfo{Identity: "deploy", Stack: "dev", ComponentFromArg: "vpc", ComponentPrompted: true, StackPrompted: true}
			merged := &schema.AuthConfig{}
			resolved := false
			executor := &Executor{services: Services{
				DefaultMergedAuthConfigGetter: func(gotConfig *schema.AtmosConfiguration, gotInfo *schema.ConfigAndStacksInfo) (*schema.AuthConfig, error) {
					require.Same(t, config, gotConfig)
					require.Same(t, info, gotInfo)
					if !tc.creatorFails {
						return nil, cause
					}
					return merged, nil
				},
				DefaultAuthManagerCreator: func(identity string, gotMerged *schema.AuthConfig, selectValue string, gotConfig *schema.AtmosConfiguration, reexec auth.ReExecContext) (auth.AuthManager, error) {
					require.True(t, tc.creatorFails, "invalid merged configuration must stop before auth creation")
					assert.Equal(t, "deploy", identity)
					assert.Same(t, merged, gotMerged)
					assert.Same(t, config, gotConfig)
					assert.Equal(t, cfg.IdentityFlagSelectValue, selectValue)
					assert.Equal(t, auth.ReExecContext{Component: "vpc", Stack: "dev", ComponentPrompted: true, StackPrompted: true}, reexec)
					return nil, cause
				},
				ResolveIdentityConfigError: func(gotConfig *schema.AtmosConfiguration, gotInfo *schema.ConfigAndStacksInfo, err, sentinel error) error {
					resolved = true
					assert.Same(t, config, gotConfig)
					assert.Same(t, info, gotInfo)
					assert.ErrorIs(t, err, cause)
					assert.ErrorIs(t, sentinel, tc.wantSentinel)
					return errors.Join(sentinel, err)
				},
			}}
			manager, err := executor.setupTerraformAuth(config, info)
			require.ErrorIs(t, err, cause)
			require.ErrorIs(t, err, tc.wantSentinel)
			assert.Nil(t, manager)
			assert.Nil(t, info.AuthManager)
			assert.True(t, resolved, "auth errors must retain the identity-selection recovery path")
		})
	}
}

// TestSetupTerraformAuthThreadsTargetStack verifies nested store authentication inherits the selected stack and identity.
func TestSetupTerraformAuthThreadsTargetStack(t *testing.T) {
	manager := authtypes.NewMockAuthManager(gomock.NewController(t))
	managerInfo := &schema.ConfigAndStacksInfo{}
	manager.EXPECT().GetStackInfo().Return(managerInfo)
	info := &schema.ConfigAndStacksInfo{Stack: "dev"}
	config := &schema.AtmosConfiguration{}
	executor := &Executor{services: Services{
		DefaultMergedAuthConfigGetter: func(*schema.AtmosConfiguration, *schema.ConfigAndStacksInfo) (*schema.AuthConfig, error) {
			return &schema.AuthConfig{}, nil
		},
		DefaultAuthManagerCreator: func(string, *schema.AuthConfig, string, *schema.AtmosConfiguration, auth.ReExecContext) (auth.AuthManager, error) {
			return manager, nil
		},
		StoreAutoDetectedIdentity: func(got auth.AuthManager, target *schema.ConfigAndStacksInfo) {
			require.Same(t, manager, got)
			require.Same(t, info, target)
			target.Identity = "detected-deploy"
		},
	}}
	got, err := executor.setupTerraformAuth(config, info)
	require.NoError(t, err)
	assert.Same(t, manager, got)
	assert.Same(t, manager, info.AuthManager)
	assert.Equal(t, "dev", managerInfo.Stack, "nested store auth needs the target stack to resolve emulator endpoints")
	require.NotNil(t, config.SecretsAuth)
	assert.Equal(t, "detected-deploy", config.SecretsAuth.DefaultIdentity)
	assert.NotNil(t, config.SecretsAuth.Resolver)
}

// TestAppendTerraformSecretEnv verifies secret encoding failures propagate and test secrets take precedence over component values.
func TestAppendTerraformSecretEnv(t *testing.T) {
	secret := "environment-fixture-sensitive-value"
	atmosio.RegisterSecret(secret)
	for _, tc := range []struct {
		name    string
		info    schema.ConfigAndStacksInfo
		want    []string
		invalid bool
	}{
		{name: "component encoding error", info: schema.ConfigAndStacksInfo{
			TerraformSecretVarKeys: map[string]bool{"token": true}, ComponentVarsSection: map[string]any{"token": make(chan int)},
		}, invalid: true},
		{name: "test encoding error", info: schema.ConfigAndStacksInfo{
			SubCommand: "test", ComponentSection: map[string]any{"test": map[string]any{"vars": map[string]any{"credentials": map[string]any{"token": secret, "invalid": make(chan int)}}}},
		}, invalid: true},
		{name: "test secrets override component variables", info: schema.ConfigAndStacksInfo{
			SubCommand: "test", ComponentEnvList: []string{"TF_VAR_token=component-value"},
			ComponentSection: map[string]any{"test": map[string]any{"vars": map[string]any{"token": secret, "public": "unclassified"}}},
		}, want: []string{"TF_VAR_token=component-value", "TF_VAR_token=" + secret}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := appendTerraformSecretEnv(&tc.info)
			if tc.invalid {
				var encodingErr *json.UnsupportedTypeError
				require.ErrorAs(t, err, &encodingErr)
				assert.Empty(t, tc.info.ComponentEnvList, "invalid secrets must not yield partial environment output")
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.want, tc.info.ComponentEnvList)
		})
	}
}

// TestAssembleComponentEnvVarsRejectsInvalidRC verifies malformed CLI configuration stops environment preparation.
func TestAssembleComponentEnvVarsRejectsInvalidRC(t *testing.T) {
	clearUserCLIConfigEnv(t)
	config := &schema.AtmosConfiguration{BasePath: t.TempDir()}
	config.Components.Terraform.RC = &schema.TerraformRCConfig{
		Enabled: true,
		Config:  map[string]any{"provider_installation": "invalid-method-list"},
	}
	info := &schema.ConfigAndStacksInfo{}
	err := assembleComponentEnvVars(config, info, nil)
	require.ErrorIs(t, err, errUtils.ErrInvalidConfig)
	assert.Nil(t, info.RCCleanup)
	for _, env := range info.ComponentEnvList {
		assert.NotContains(t, env, rc.EnvKeyTFCLIConfigFile+"=", "invalid configuration must not be exported to Terraform")
	}
}

// TestConfigureTerraformRCIncludesRegistryCache verifies the generated CLI configuration routes providers and modules through the registry proxy.
func TestConfigureTerraformRCIncludesRegistryCache(t *testing.T) {
	clearUserCLIConfigEnv(t)
	config := &schema.AtmosConfiguration{}
	config.Components.Terraform.Cache = &schema.TerraformCacheConfig{Enabled: true, Location: t.TempDir()}
	setup, err := tfcache.Start(t.Context(), config)
	require.NoError(t, err)
	require.NotNil(t, setup)
	t.Cleanup(func() { require.NoError(t, setup.Close(context.Background())) })
	info := &schema.ConfigAndStacksInfo{TerraformCache: setup}
	require.NoError(t, configureTerraformRC(config, info))
	require.NotNil(t, info.RCCleanup)
	t.Cleanup(func() { require.NoError(t, info.RCCleanup()) })
	var configPath string
	for _, env := range info.ComponentEnvList {
		if path, ok := strings.CutPrefix(env, rc.EnvKeyTFCLIConfigFile+"="); ok {
			configPath = path
		}
	}
	require.NotEmpty(t, configPath)
	contents, err := os.ReadFile(configPath)
	require.NoError(t, err)
	assert.Contains(t, string(contents), "network_mirror")
	assert.Contains(t, string(contents), "https://127.0.0.1:")
	assert.Contains(t, string(contents), `host "registry.terraform.io"`)
	assert.Contains(t, string(contents), `host "registry.opentofu.org"`)
	assert.Contains(t, info.ComponentEnvList, rc.EnvKeyTofuCLIConfigFile+"="+configPath)
}
