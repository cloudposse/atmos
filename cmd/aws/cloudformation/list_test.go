package cloudformation

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/auth"
	"github.com/cloudposse/atmos/pkg/schema"
)

// cloudFormationComponentStackName must extract
// components.aws/cloudformation.<component>.stack_name, returning ("", false)
// gracefully at every level where the expected shape is absent instead of
// panicking on a bad type assertion.
func TestCloudFormationComponentStackName(t *testing.T) {
	validStacksMap := map[string]any{
		"dev": map[string]any{
			"components": map[string]any{
				"aws/cloudformation": map[string]any{
					"vpc": map[string]any{
						"stack_name": "my-vpc-stack",
					},
					"no-stack-name": map[string]any{},
					"empty-stack-name": map[string]any{
						"stack_name": "",
					},
				},
			},
		},
	}

	tests := []struct {
		name          string
		stacksMap     map[string]any
		stack         string
		componentName string
		wantName      string
		wantOK        bool
	}{
		{
			name:          "resolves configured stack_name",
			stacksMap:     validStacksMap,
			stack:         "dev",
			componentName: "vpc",
			wantName:      "my-vpc-stack",
			wantOK:        true,
		},
		{
			name:          "missing stack key",
			stacksMap:     validStacksMap,
			stack:         "does-not-exist",
			componentName: "vpc",
		},
		{
			name:          "missing components key",
			stacksMap:     map[string]any{"dev": map[string]any{}},
			stack:         "dev",
			componentName: "vpc",
		},
		{
			name: "missing type key",
			stacksMap: map[string]any{
				"dev": map[string]any{"components": map[string]any{}},
			},
			stack:         "dev",
			componentName: "vpc",
		},
		{
			name:          "missing component key",
			stacksMap:     validStacksMap,
			stack:         "dev",
			componentName: "does-not-exist",
		},
		{
			name:          "missing stack_name field",
			stacksMap:     validStacksMap,
			stack:         "dev",
			componentName: "no-stack-name",
		},
		{
			name:          "empty stack_name field",
			stacksMap:     validStacksMap,
			stack:         "dev",
			componentName: "empty-stack-name",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			name, ok := cloudFormationComponentStackName(tt.stacksMap, tt.stack, tt.componentName)
			assert.Equal(t, tt.wantOK, ok)
			assert.Equal(t, tt.wantName, name)
		})
	}
}

// configuredCloudFormationStackNames must build the set of configured
// stack_name values (only including components that actually configured one)
// from the described stacks map.
func TestConfiguredCloudFormationStackNames(t *testing.T) {
	origDescribe, origList := cfnDescribeStacks, cfnListAllComponents
	t.Cleanup(func() {
		cfnDescribeStacks = origDescribe
		cfnListAllComponents = origList
	})

	stacksMap := map[string]any{
		"dev": map[string]any{
			"components": map[string]any{
				"aws/cloudformation": map[string]any{
					"vpc":           map[string]any{"stack_name": "my-vpc-stack"},
					"no-stack-name": map[string]any{},
				},
			},
		},
	}

	cfnDescribeStacks = func(*schema.AtmosConfiguration, string, []string, []string, []string, bool, bool, bool, bool, []string, auth.AuthManager) (map[string]any, error) {
		return stacksMap, nil
	}
	cfnListAllComponents = func(context.Context, string, map[string]any) ([]string, error) {
		return []string{"vpc", "no-stack-name"}, nil
	}

	names, err := configuredCloudFormationStackNames(&schema.AtmosConfiguration{}, "dev", nil)
	require.NoError(t, err)
	assert.Equal(t, map[string]bool{"my-vpc-stack": true}, names)
}

// configuredCloudFormationStackNames must propagate a describe-stacks failure.
func TestConfiguredCloudFormationStackNames_DescribeError(t *testing.T) {
	origDescribe := cfnDescribeStacks
	t.Cleanup(func() { cfnDescribeStacks = origDescribe })

	sentinel := errors.New("describe failed")
	cfnDescribeStacks = func(*schema.AtmosConfiguration, string, []string, []string, []string, bool, bool, bool, bool, []string, auth.AuthManager) (map[string]any, error) {
		return nil, sentinel
	}

	_, err := configuredCloudFormationStackNames(&schema.AtmosConfiguration{}, "dev", nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, sentinel)
}

// configuredCloudFormationStackNames must propagate a list-components failure.
func TestConfiguredCloudFormationStackNames_ListError(t *testing.T) {
	origDescribe, origList := cfnDescribeStacks, cfnListAllComponents
	t.Cleanup(func() {
		cfnDescribeStacks = origDescribe
		cfnListAllComponents = origList
	})

	cfnDescribeStacks = func(*schema.AtmosConfiguration, string, []string, []string, []string, bool, bool, bool, bool, []string, auth.AuthManager) (map[string]any, error) {
		return map[string]any{}, nil
	}
	sentinel := errors.New("list failed")
	cfnListAllComponents = func(context.Context, string, map[string]any) ([]string, error) {
		return nil, sentinel
	}

	_, err := configuredCloudFormationStackNames(&schema.AtmosConfiguration{}, "dev", nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, sentinel)
}
