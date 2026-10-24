package composition

import (
	"errors"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/auth"
	"github.com/cloudposse/atmos/pkg/schema"
)

func withCompletionStubs(t *testing.T, stacks map[string]any, initErr, describeErr error) {
	t.Helper()
	originalInit := initCliConfigForCompletion
	originalDescribe := describeStacksForCompletion
	t.Cleanup(func() {
		initCliConfigForCompletion = originalInit
		describeStacksForCompletion = originalDescribe
	})

	initCliConfigForCompletion = func(schema.ConfigAndStacksInfo, bool) (schema.AtmosConfiguration, error) {
		if initErr != nil {
			return schema.AtmosConfiguration{}, initErr
		}
		return schema.AtmosConfiguration{}, nil
	}
	describeStacksForCompletion = func(
		_ *schema.AtmosConfiguration,
		_ string,
		_, _, _ []string,
		_, _, _, _ bool,
		_ []string,
		_ auth.AuthManager,
	) (map[string]any, error) {
		return stacks, describeErr
	}
}

func compositionStack(componentNames ...string) map[string]any {
	components := map[string]any{}
	for _, name := range componentNames {
		components[name] = map[string]any{"composition": name}
	}
	return map[string]any{
		"components": map[string]any{
			"container": components,
		},
	}
}

func TestCompositionStackFlagCompletion_FiltersByComposition(t *testing.T) {
	withCompletionStubs(t, map[string]any{
		"prod": compositionStack("storefront"),
		"dev":  compositionStack("storefront", "payments"),
		"test": compositionStack("payments"),
	}, nil, nil)

	stacks, directive := compositionStackFlagCompletion(&cobra.Command{Use: "up"}, []string{"storefront"}, "")
	assert.Equal(t, cobra.ShellCompDirectiveNoFileComp, directive)
	assert.Equal(t, []string{"dev", "prod"}, stacks)
}

func TestCompositionStackFlagCompletion_ListsAllCompositionStacks(t *testing.T) {
	withCompletionStubs(t, map[string]any{
		"empty": map[string]any{"components": map[string]any{"container": map[string]any{"api": map[string]any{}}}},
		"prod":  compositionStack("storefront"),
		"dev":   compositionStack("payments"),
	}, nil, nil)

	stacks, directive := compositionStackFlagCompletion(&cobra.Command{Use: "up"}, nil, "")
	assert.Equal(t, cobra.ShellCompDirectiveNoFileComp, directive)
	assert.Equal(t, []string{"dev", "prod"}, stacks)
}

func TestCompositionStackFlagCompletion_DegradesGracefully(t *testing.T) {
	t.Run("configuration failure", func(t *testing.T) {
		withCompletionStubs(t, nil, errors.New("config failed"), nil)

		stacks, directive := compositionStackFlagCompletion(&cobra.Command{Use: "up"}, nil, "")
		assert.Nil(t, stacks)
		assert.Equal(t, cobra.ShellCompDirectiveNoFileComp, directive)
	})

	t.Run("stack loading failure", func(t *testing.T) {
		withCompletionStubs(t, nil, nil, errors.New("describe failed"))

		stacks, directive := compositionStackFlagCompletion(&cobra.Command{Use: "up"}, nil, "")
		assert.Nil(t, stacks)
		assert.Equal(t, cobra.ShellCompDirectiveNoFileComp, directive)
	})
}

func TestCompositionStackNames_IgnoresMalformedStackData(t *testing.T) {
	stacks := compositionStackNames(map[string]any{
		"bad-stack": "not a map",
		"bad-type":  map[string]any{"components": "not a map"},
		"valid":     compositionStack("storefront"),
	}, "storefront")
	require.Equal(t, []string{"valid"}, stacks)
}
