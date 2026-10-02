package exec

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth"
	authtypes "github.com/cloudposse/atmos/pkg/auth/types"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
)

// setupCloudFormationTargetOutputFixture creates a direct-target fixture with distinct dev and sandbox identities.
func setupCloudFormationTargetOutputFixture(t *testing.T) schema.AtmosConfiguration {
	t.Helper()
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "stacks"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "atmos.yaml"), []byte(`base_path: "."
components:
  aws/cloudformation:
    base_path: components/cloudformation
stacks:
  base_path: stacks
  included_paths: ["*"]
  name_template: "{{ .vars.stage }}"
auth:
  identities:
    dev:
      kind: aws/user
    sandbox:
      kind: aws/user
`), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "stacks", "test.yaml"), []byte(`vars:
  stage: test
components:
  aws/cloudformation:
    vpc:
      stack_name: test-vpc
      template:
        Resources: {}
      provision:
        default: deployment
        targets:
          deployment:
            kind: aws/cloudformation
            auth:
              identities:
                dev:
                  default: true
`), 0o600))
	t.Chdir(dir)
	t.Setenv("ATMOS_CLI_CONFIG_PATH", dir)
	ac, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, true)
	require.NoError(t, err)
	return ac
}

// TestCloudFormationOutputTargetAuthSameProcess checks YAML and template output caches across identity changes and authentication failures.
func TestCloudFormationOutputTargetAuthSameProcess(t *testing.T) {
	for _, mode := range []string{"yaml", "template"} {
		t.Run(mode, func(t *testing.T) {
			clearComponentFuncSyncMap(t)
			ResetNestedAuthManagerCache()
			t.Cleanup(ResetNestedAuthManagerCache)
			ac := setupCloudFormationTargetOutputFixture(t)
			failAuth := false
			stubCloudFormationDeliveryAuth(t, &failAuth)
			getter := NewMockCloudFormationOutputsGetter(gomock.NewController(t))
			getter.EXPECT().GetOutputs(gomock.Any(), gomock.Any(), "test-vpc", gomock.Any()).DoAndReturn(
				func(_ context.Context, _, _ string, credentials *schema.AWSAuthContext) (map[string]any, error) {
					require.NotNil(t, credentials)
					return map[string]any{"VpcId": credentials.Profile}, nil
				},
			).AnyTimes()
			stubCloudFormationOutputsGetter(t, getter)
			requested := ""
			parent := &schema.ConfigAndStacksInfo{
				Identity: "sandbox", RequestedIdentity: &requested,
				AuthContext: &schema.AuthContext{AWS: &schema.AWSAuthContext{Profile: "sandbox"}},
			}
			read := func() (any, error) {
				if mode == "yaml" {
					return processTagAwsCloudFormationOutputWithContext(&ac, "!aws.cloudformation.output vpc test VpcId", "test", nil, parent)
				}
				value, err := componentFunc(&ac, parent, "vpc", "test")
				if err != nil {
					return nil, err
				}
				return value.(map[string]any)[cfg.OutputsSectionName].(map[string]any)["VpcId"], nil
			}
			for _, tc := range []struct{ requested, expected string }{{"", "dev"}, {"sandbox", "sandbox"}, {"", "dev"}} {
				requested = tc.requested
				value, err := read()
				require.NoError(t, err)
				assert.Equal(t, tc.expected, value)
				assert.Equal(t, "sandbox", parent.Identity)
				assert.Equal(t, "sandbox", parent.AuthContext.AWS.Profile)
				assert.False(t, ac.Auth.Identities["dev"].Default)
			}
			failAuth = true
			for _, requested = range []string{"", "sandbox"} {
				_, err := read()
				require.ErrorIs(t, err, errUtils.ErrAuthenticationFailed, "cached output must not bypass failed target authentication, including matching caller credentials")
			}
		})
	}
}

// stubCloudFormationDeliveryAuth provides observable target identities and an injectable authentication failure.
func stubCloudFormationDeliveryAuth(t *testing.T, fail *bool) {
	t.Helper()
	original := createCloudFormationTargetAuthManager
	t.Cleanup(func() { createCloudFormationTargetAuthManager = original })
	createCloudFormationTargetAuthManager = func(identity string, merged *schema.AuthConfig, _ string, _ *schema.AtmosConfiguration, stack string) (auth.AuthManager, error) {
		if *fail {
			return nil, errUtils.ErrAuthenticationFailed
		}
		require.Equal(t, "test", stack)
		if identity == "" {
			require.True(t, merged.Identities["dev"].Default)
			identity = "dev"
		}
		manager := authtypes.NewMockAuthManager(gomock.NewController(t))
		manager.EXPECT().GetChain().Return([]string{identity}).AnyTimes()
		manager.EXPECT().GetStackInfo().Return(&schema.ConfigAndStacksInfo{AuthContext: &schema.AuthContext{AWS: &schema.AWSAuthContext{Profile: identity}}}).AnyTimes()
		return manager, nil
	}
}

// TestCloudFormationOutputAuthIgnoresNonDirectTargets keeps component credentials for outputs when the delivery target does not deploy a stack.
func TestCloudFormationOutputAuthIgnoresNonDirectTargets(t *testing.T) {
	for _, kind := range []string{"aws/s3", "git", "aws/stackset"} {
		section := map[string]any{cfg.ProvisionSectionName: map[string]any{
			"default": "publish", "targets": map[string]any{"publish": map[string]any{"kind": kind, "auth": "invalid-but-unselected"}},
		}}
		parent := &schema.AuthContext{AWS: &schema.AWSAuthContext{Profile: "parent"}}
		resolved, err := cloudFormationOutputAuthForSections(&schema.AtmosConfiguration{}, section, &schema.ConfigAndStacksInfo{}, parent)
		require.NoError(t, err)
		assert.Same(t, parent, resolved)
	}
}

// Exercise the describe entry point: the command passes an authenticated manager,
// not its original ConfigAndStacksInfo, to the executor.
func TestDescribeCloudFormationOutputTargetIdentity(t *testing.T) {
	clearComponentFuncSyncMap(t)
	ResetNestedAuthManagerCache()
	t.Cleanup(ResetNestedAuthManagerCache)
	ac := setupCloudFormationTargetOutputFixture(t)
	path := filepath.Join(ac.BasePath, "stacks", "test.yaml")
	contents, err := os.ReadFile(path)
	require.NoError(t, err)
	contents = append(contents, []byte(`
  terraform:
    reader:
      vars:
        yaml_value: '!aws.cloudformation.output vpc test VpcId'
        template_value: '{{ (atmos.Component "vpc" "test").outputs.VpcId }}'
`)...)
	require.NoError(t, os.WriteFile(path, contents, 0o600))
	ac, err = cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, true)
	require.NoError(t, err)
	ac.Templates.Settings.Enabled = true
	failAuth := false
	stubCloudFormationDeliveryAuth(t, &failAuth)
	getter := NewMockCloudFormationOutputsGetter(gomock.NewController(t))
	getter.EXPECT().GetOutputs(gomock.Any(), gomock.Any(), "test-vpc", gomock.Any()).DoAndReturn(
		func(_ context.Context, _, _ string, credentials *schema.AWSAuthContext) (map[string]any, error) {
			require.NotNil(t, credentials)
			return map[string]any{"VpcId": credentials.Profile}, nil
		},
	).AnyTimes()
	stubCloudFormationOutputsGetter(t, getter)
	for _, requested := range []string{"sandbox", "", "sandbox"} {
		manager := authtypes.NewMockAuthManager(gomock.NewController(t))
		manager.EXPECT().GetStackInfo().Return(&schema.ConfigAndStacksInfo{
			RequestedIdentity: &requested,
			AuthContext:       &schema.AuthContext{AWS: &schema.AWSAuthContext{Profile: "sandbox"}},
		}).AnyTimes()
		ac.AuthManager = manager
		result, err := ExecuteDescribeComponent(&ExecuteDescribeComponentParams{
			AtmosConfig: &ac, Component: "reader", Stack: "test",
			ProcessTemplates: true, ProcessYamlFunctions: true, AuthManager: manager,
		})
		require.NoError(t, err)
		expected := requested
		if expected == "" {
			expected = "dev"
		}
		vars := result[cfg.VarsSectionName].(map[string]any)
		assert.Equal(t, expected, vars["yaml_value"])
		assert.Equal(t, expected, vars["template_value"])
	}
}
