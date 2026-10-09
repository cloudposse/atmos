package cmd

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/auth"
	"github.com/cloudposse/atmos/pkg/schema"
)

func mockSourceSelectionStacks(t *testing.T, stacks map[string]any) {
	t.Helper()
	originalInit, originalDescribe := initCliConfigForPrompt, executeDescribeStacksFunc
	t.Cleanup(func() { initCliConfigForPrompt, executeDescribeStacksFunc = originalInit, originalDescribe })
	initCliConfigForPrompt = func(schema.ConfigAndStacksInfo, bool) (schema.AtmosConfiguration, error) {
		return schema.AtmosConfiguration{}, nil
	}
	executeDescribeStacksFunc = func(_ *schema.AtmosConfiguration, _ string, _, _, _ []string, _, _, _, _ bool, _ []string, _ auth.AuthManager) (map[string]any, error) {
		return stacks, nil
	}
}

func sourceSelectionStack(componentType string, components map[string]any) map[string]any {
	return map[string]any{"components": map[string]any{componentType: components}}
}

func TestSourceSelection_CommandComponentType(t *testing.T) {
	withSource := map[string]any{"source": map[string]any{"uri": "github.com/example/component"}}
	builders := map[string]func(*Config) *cobra.Command{
		"delete": DeleteCommand, "pull": PullCommand, "describe": DescribeCommand, "list": ListCommand,
	}
	for _, componentType := range []string{"aws/cloudformation", "terraform", "helmfile", "packer"} {
		t.Run(componentType, func(t *testing.T) {
			// Keep a same-name component in another type and a stack with source only in that other type.
			otherType := "terraform"
			if componentType == otherType {
				otherType = "aws/cloudformation"
			}
			mixed := sourceSelectionStack(componentType, map[string]any{"shared": withSource, "local": map[string]any{}})
			mixed["components"].(map[string]any)[otherType] = map[string]any{"foreign": withSource, "shared": withSource}
			stacks := map[string]any{
				"own-a":      sourceSelectionStack(componentType, map[string]any{"shared": withSource}),
				"own-b":      sourceSelectionStack(componentType, map[string]any{"other": withSource}),
				"mixed":      mixed,
				"foreign":    sourceSelectionStack(otherType, map[string]any{"shared": withSource}),
				"unvendored": sourceSelectionStack(componentType, map[string]any{"shared": map[string]any{}}),
			}
			mockSourceSelectionStacks(t, stacks)
			for name, builder := range builders {
				t.Run(name, func(t *testing.T) {
					cmd := builder(&Config{ComponentType: componentType, CLIName: "unrelated display name"})
					components, directive := ComponentArgCompletion(cmd, nil, "")
					assert.Equal(t, []string{"other", "shared"}, components)
					assert.Equal(t, cobra.ShellCompDirectiveNoFileComp, directive)
					components, _ = ComponentArgCompletion(cmd, []string{"shared"}, "")
					assert.Empty(t, components)
					allStacks, _ := StackFlagCompletion(cmd, nil, "")
					assert.Equal(t, []string{"mixed", "own-a", "own-b"}, allStacks)
					selectedStacks, _ := StackFlagCompletion(cmd, []string{"shared"}, "")
					assert.Equal(t, []string{"mixed", "own-a"}, selectedStacks)
					selectedStacks, _ = StackFlagCompletion(cmd, []string{"foreign"}, "")
					assert.Empty(t, selectedStacks)
				})
			}
		})
	}
}

func TestSourceSelection_CloudFormationOnly(t *testing.T) {
	mockSourceSelectionStacks(t, map[string]any{
		"dev": sourceSelectionStack("aws/cloudformation", map[string]any{
			"network": map[string]any{"source": map[string]any{"uri": "github.com/example/network"}},
		}),
	})
	cmd := DeleteCommand(&Config{ComponentType: "aws/cloudformation"})
	components, _ := ComponentArgCompletion(cmd, nil, "")
	require.Equal(t, []string{"network"}, components)
	stacks, _ := StackFlagCompletion(cmd, []string{"network"}, "")
	assert.Equal(t, []string{"dev"}, stacks)
}

func TestSourceSelection_UnconfiguredCommandDoesNotDefaultToTerraform(t *testing.T) {
	mockSourceSelectionStacks(t, map[string]any{
		"dev": sourceSelectionStack("terraform", map[string]any{
			"network": map[string]any{"source": map[string]any{"uri": "github.com/example/network"}},
		}),
	})
	cmd := &cobra.Command{Use: "terraform"}
	components, _ := ComponentArgCompletion(cmd, nil, "")
	assert.Empty(t, components)
	stacks, _ := StackFlagCompletion(cmd, []string{"network"}, "")
	assert.Empty(t, stacks)
	stacks, _ = StackFlagCompletion(cmd, nil, "")
	assert.Empty(t, stacks)
}
