package exec

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/secrets"
)

func TestSecretSelectorPreservesSkippedFunctions(t *testing.T) {
	for _, tc := range []struct{ name, selector string }{
		{"env", "!env ATMOS_TEST_SECRET_SELECTOR"},
		{"aws.cloudformation.output", "!aws.cloudformation.output vpc test VpcId"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			config := setupAwsCloudFormationOutputFixture(t)
			t.Setenv("ATMOS_TEST_SECRET_SELECTOR", "selected-store")
			stubCloudFormationOutputsGetter(t, NewMockCloudFormationOutputsGetter(gomock.NewController(t)))
			info := secretSelectorStackInfo(tc.selector)
			_, err := processCustomTagsWithContext(&config, "!secret API_KEY", "test", []string{tc.name}, NewResolutionContext(), info)
			require.ErrorIs(t, err, secrets.ErrSelectorUnresolved)
			assert.NotErrorIs(t, err, errUtils.ErrCircularDependency)
		})
	}
}

func TestSecretSelectorPreservesResolutionContext(t *testing.T) {
	config := setupAwsCloudFormationOutputFixture(t)
	stubCloudFormationOutputsGetter(t, NewMockCloudFormationOutputsGetter(gomock.NewController(t)))
	resolutionCtx := NewResolutionContext()
	const selector = "!aws.cloudformation.output vpc test VpcId"
	require.NoError(t, resolutionCtx.Push(&config, DependencyNode{
		Component: "vpc", Stack: "test", FunctionType: "aws.cloudformation.output", FunctionCall: selector,
	}))
	t.Cleanup(func() { resolutionCtx.Pop(&config) })
	_, err := processCustomTagsWithContext(&config, "!secret API_KEY", "test", nil, resolutionCtx, secretSelectorStackInfo(selector))
	require.ErrorIs(t, err, errUtils.ErrCircularDependency)
	require.Len(t, resolutionCtx.CallStack, 1)
}

func secretSelectorStackInfo(selector string) *schema.ConfigAndStacksInfo {
	return &schema.ConfigAndStacksInfo{
		ComponentFromArg: "consumer", Stack: "test",
		ComponentSection: map[string]any{
			"secrets": map[string]any{
				"vars": map[string]any{"API_KEY": map[string]any{"store": selector}},
			},
		},
	}
}

// Callers without an explicit context still share the goroutine's dependency chain.
func TestSecretSelectorFallsBackToCurrentResolutionContext(t *testing.T) {
	config := setupAwsCloudFormationOutputFixture(t)
	stubCloudFormationOutputsGetter(t, NewMockCloudFormationOutputsGetter(gomock.NewController(t)))
	ClearResolutionContext()
	t.Cleanup(ClearResolutionContext)
	resolutionCtx := GetOrCreateResolutionContext()
	const selector = "!aws.cloudformation.output vpc test VpcId"
	require.NoError(t, resolutionCtx.Push(&config, DependencyNode{
		Component: "vpc", Stack: "test", FunctionType: "aws.cloudformation.output", FunctionCall: selector,
	}))
	_, handled, err := processSimpleTags(&config, "!secret API_KEY", "test", nil, nil, secretSelectorStackInfo(selector))
	require.True(t, handled)
	require.ErrorIs(t, err, errUtils.ErrCircularDependency)
	require.Len(t, resolutionCtx.CallStack, 1)
}
