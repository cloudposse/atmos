package exec

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	authtypes "github.com/cloudposse/atmos/pkg/auth/types"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
)

// producerAuthSandbox is a producer whose direct target authenticates as sandbox.
const producerAuthSandbox = `            auth:
              identity: sandbox
`

// outputIdentityReader appends a terraform component that reads the producer with both function forms.
const outputIdentityReader = `
  terraform:
    reader:
      vars:
        yaml_value: '!aws.cloudformation.output vpc test VpcId'
        template_value: '{{ (atmos.Component "vpc" "test").outputs.VpcId }}'
`

// setupOutputIdentityFixture creates a producer with the given target auth and a reader component.
func setupOutputIdentityFixture(t *testing.T, targetAuth string) schema.AtmosConfiguration {
	t.Helper()
	clearComponentFuncSyncMap(t)
	ResetNestedAuthManagerCache()
	t.Cleanup(ResetNestedAuthManagerCache)
	ac := setupCloudFormationTargetOutputFixtureWithAuth(t, targetAuth+outputIdentityReader)
	ac.Templates.Settings.Enabled = true
	stubCloudFormationDeliveryAuth(t, new(bool))
	getter := NewMockCloudFormationOutputsGetter(gomock.NewController(t))
	getter.EXPECT().GetOutputs(gomock.Any(), gomock.Any(), "test-vpc", gomock.Any()).DoAndReturn(
		func(_ context.Context, _, _ string, credentials *schema.AWSAuthContext) (map[string]any, error) {
			require.NotNil(t, credentials)
			// A same-named stack exists in every identity's account; the value names the account read.
			return map[string]any{"VpcId": credentials.Profile}, nil
		},
	).AnyTimes()
	stubCloudFormationOutputsGetter(t, getter)
	return ac
}

// outputIdentityManager is an authenticated caller manager for the given explicit identity.
func outputIdentityManager(t *testing.T, requested string) *authtypes.MockAuthManager {
	t.Helper()
	manager := authtypes.NewMockAuthManager(gomock.NewController(t))
	manager.EXPECT().GetStackInfo().Return(&schema.ConfigAndStacksInfo{
		RequestedIdentity: &requested,
		AuthContext:       &schema.AuthContext{AWS: &schema.AWSAuthContext{Profile: requested}},
	}).AnyTimes()
	manager.EXPECT().GetChain().Return([]string{requested}).AnyTimes()
	return manager
}

// TestCloudFormationOutputsFollowProducerIdentity reproduces the cross-account redirect: the consumer's own
// default (or any explicit identity) must never move an output read away from the account the producer's
// target declares, in describe component, describe stacks and both function forms.
func TestCloudFormationOutputsFollowProducerIdentity(t *testing.T) {
	for _, caller := range []string{"dev", "sandbox", "other"} {
		t.Run("caller="+caller, func(t *testing.T) {
			ac := setupOutputIdentityFixture(t, producerAuthSandbox)

			t.Run("describe component", func(t *testing.T) {
				manager := outputIdentityManager(t, caller)
				result, err := ExecuteDescribeComponent(&ExecuteDescribeComponentParams{
					AtmosConfig: &ac, Component: "reader", Stack: "test",
					ProcessTemplates: true, ProcessYamlFunctions: true, AuthManager: manager,
				})
				require.NoError(t, err)
				vars := result[cfg.VarsSectionName].(map[string]any)
				assert.Equal(t, "sandbox", vars["yaml_value"])
				assert.Equal(t, "sandbox", vars["template_value"])
			})

			t.Run("describe stacks", func(t *testing.T) {
				manager := outputIdentityManager(t, caller)
				stacks, err := ExecuteDescribeStacks(&ac, "test", []string{"reader"}, nil, nil, false, true, true, false, nil, manager)
				require.NoError(t, err)
				vars := stackComponentVars(t, stacks, "test", "reader")
				assert.Equal(t, "sandbox", vars["yaml_value"])
				assert.Equal(t, "sandbox", vars["template_value"])
			})
		})
	}
}

// TestCloudFormationOutputsWithoutProducerAuthUseCaller keeps the existing behavior when the producer's
// target declares no auth: the lookup uses the caller's credentials.
func TestCloudFormationOutputsWithoutProducerAuthUseCaller(t *testing.T) {
	ac := setupOutputIdentityFixture(t, "")
	for _, caller := range []string{"dev", "sandbox"} {
		manager := outputIdentityManager(t, caller)
		result, err := ExecuteDescribeComponent(&ExecuteDescribeComponentParams{
			AtmosConfig: &ac, Component: "reader", Stack: "test",
			ProcessTemplates: true, ProcessYamlFunctions: true, AuthManager: manager,
		})
		require.NoError(t, err)
		vars := result[cfg.VarsSectionName].(map[string]any)
		assert.Equal(t, caller, vars["yaml_value"])
		assert.Equal(t, caller, vars["template_value"])
	}
}

// stackComponentVars returns a terraform component's vars from a describe-stacks result.
func stackComponentVars(t *testing.T, stacks map[string]any, stack, component string) map[string]any {
	t.Helper()
	stackSection, ok := stacks[stack].(map[string]any)
	require.True(t, ok, "stack %q missing", stack)
	components, _ := stackSection[cfg.ComponentsSectionName].(map[string]any)
	terraform, _ := components[cfg.TerraformSectionName].(map[string]any)
	section, ok := terraform[component].(map[string]any)
	require.True(t, ok, "component %q missing", component)
	vars, _ := section[cfg.VarsSectionName].(map[string]any)
	return vars
}
