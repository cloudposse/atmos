package composition

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/auth"
	"github.com/cloudposse/atmos/pkg/schema"
)

// withValidateStubs replaces the seams ExecuteValidate depends on so it runs
// without a real Atmos project: initCliConfig returns a config with the given
// compositions, and describeStacks returns the given stacks map/error.
func withValidateStubs(t *testing.T, comps map[string]schema.Composition, stacksMap map[string]any, initErr, describeErr error) {
	t.Helper()

	origInit, origDescribe := initCliConfig, describeStacks
	t.Cleanup(func() {
		initCliConfig, describeStacks = origInit, origDescribe
	})

	initCliConfig = func(_ schema.ConfigAndStacksInfo, _ bool) (schema.AtmosConfiguration, error) {
		return schema.AtmosConfiguration{Compositions: comps}, initErr
	}
	describeStacks = func(_ *schema.AtmosConfiguration, _ string, _, _, _ []string, _, _, _, _ bool, _ []string, _ auth.AuthManager) (map[string]any, error) {
		return stacksMap, describeErr
	}
}

func TestExecuteValidate_Success(t *testing.T) {
	stacksMap := map[string]any{
		"dev": stackWithComponents(map[string]string{"api": "storefront", "worker": "storefront"}),
	}
	withValidateStubs(t, compositions(), stacksMap, nil, nil)
	require.NoError(t, ExecuteValidate(context.Background(), &schema.ConfigAndStacksInfo{}, "storefront"))
}

func TestExecuteValidate_InitConfigError(t *testing.T) {
	withValidateStubs(t, nil, nil, assert.AnError, nil)
	require.ErrorIs(t, ExecuteValidate(context.Background(), &schema.ConfigAndStacksInfo{}, "storefront"), assert.AnError)
}

func TestExecuteValidate_DescribeStacksError(t *testing.T) {
	withValidateStubs(t, compositions(), nil, nil, assert.AnError)
	require.ErrorIs(t, ExecuteValidate(context.Background(), &schema.ConfigAndStacksInfo{}, "storefront"), assert.AnError)
}

func TestExecuteValidate_UnknownComposition(t *testing.T) {
	withValidateStubs(t, compositions(), map[string]any{}, nil, nil)
	require.Error(t, ExecuteValidate(context.Background(), &schema.ConfigAndStacksInfo{}, "missing"))
}
