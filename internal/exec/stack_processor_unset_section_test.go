package exec

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// TestExtractComponentSections_UnsetWholeSection verifies that a component section set to
// the `!unset` YAML tag (decoded to the plain string "!unset" before YAML functions run) is
// treated as absent instead of failing the map type assertion (issue #2994).
func TestExtractComponentSections_UnsetWholeSection(t *testing.T) {
	tests := []struct {
		section string
		get     func(r *ComponentProcessorResult) map[string]any
	}{
		{cfg.VarsSectionName, func(r *ComponentProcessorResult) map[string]any { return r.ComponentVars }},
		{cfg.SettingsSectionName, func(r *ComponentProcessorResult) map[string]any { return r.ComponentSettings }},
		{cfg.EnvSectionName, func(r *ComponentProcessorResult) map[string]any { return r.ComponentEnv }},
		{cfg.RetrySectionName, func(r *ComponentProcessorResult) map[string]any { return r.ComponentRetry }},
		{cfg.HooksSectionName, func(r *ComponentProcessorResult) map[string]any { return r.ComponentHooks }},
	}

	for _, tt := range tests {
		t.Run(tt.section, func(t *testing.T) {
			opts := ComponentProcessorOptions{
				ComponentType: cfg.TerraformComponentType,
				Component:     "vpc",
				StackName:     "test-stack",
				ComponentMap:  map[string]any{tt.section: u.AtmosYamlFuncUnset},
				AtmosConfig:   &schema.AtmosConfiguration{},
			}
			result := &ComponentProcessorResult{}

			require.NoError(t, extractComponentSections(&opts, result))
			assert.Empty(t, tt.get(result))
			// The caller's component map must not be mutated.
			assert.Equal(t, u.AtmosYamlFuncUnset, opts.ComponentMap[tt.section])
		})
	}
}

// TestDescribeStacks_UnsetWholeSection runs the unset-section scenario end to end: a section set
// to `!unset` at one layer drops every lower layer (global and base component), while overrides
// still apply on top of a component-level unset.
func TestDescribeStacks_UnsetWholeSection(t *testing.T) {
	fixtureDir, err := filepath.Abs(filepath.Join("..", "..", "tests", "fixtures", "scenarios", "unset-section"))
	require.NoError(t, err)
	t.Chdir(fixtureDir)
	t.Setenv("ATMOS_CLI_CONFIG_PATH", fixtureDir)
	t.Setenv("ATMOS_BASE_PATH", fixtureDir)

	atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, true)
	require.NoError(t, err)

	// The embedded manifest schema accepts `!unset` for whole sections.
	require.NoError(t, ValidateStacks(&atmosConfig))

	stacks, err := ExecuteDescribeStacks(&atmosConfig, "", nil, nil, nil, false, true, true, false, nil, nil)
	require.NoError(t, err)

	component := func(stack, name string) map[string]any {
		t.Helper()
		stackMap, ok := stacks[stack].(map[string]any)
		require.True(t, ok, "stack %s not found", stack)
		components, ok := stackMap[cfg.ComponentsSectionName].(map[string]any)
		require.True(t, ok)
		terraform, ok := components[cfg.TerraformComponentType].(map[string]any)
		require.True(t, ok)
		c, ok := terraform[name].(map[string]any)
		require.True(t, ok, "component %s not found in stack %s", name, stack)
		return c
	}

	t.Run("component unset drops the global layer", func(t *testing.T) {
		vpc := component("deploy/dev", "vpc")
		assert.Empty(t, vpc[cfg.VarsSectionName], "global vars, including stage, are dropped")
		assert.NotContains(t, vpc, cfg.RetrySectionName)
	})

	t.Run("component unset drops the inherited base component layer", func(t *testing.T) {
		child := component("deploy/dev", "vpc-child")
		assert.Empty(t, child[cfg.VarsSectionName])
		assert.NotContains(t, child, cfg.RetrySectionName)
		assert.Equal(t, []any{"vpc-base"}, child[cfg.InheritanceSectionName])
	})

	t.Run("base component unset drops its ancestors and the global layer", func(t *testing.T) {
		grandchild := component("deploy/dev", "vpc-grandchild")
		assert.Equal(t, map[string]any{"max_attempts": 5}, grandchild[cfg.RetrySectionName])
		assert.Equal(t, map[string]any{"cidr": "10.0.0.0/16", "stage": "dev"}, grandchild[cfg.VarsSectionName])
	})

	t.Run("overrides apply on top of a component unset", func(t *testing.T) {
		overridden := component("deploy/prod", "vpc-overrides")
		assert.Equal(t, map[string]any{"max_attempts": 7}, overridden[cfg.RetrySectionName])
	})

	t.Run("overrides unset drops every lower layer", func(t *testing.T) {
		overridden := component("deploy/prod", "vpc-overrides")
		assert.Empty(t, overridden[cfg.VarsSectionName])
	})

	t.Run("sections without unset still inherit", func(t *testing.T) {
		base := component("deploy/dev", "vpc-base")
		assert.Equal(t, map[string]any{"max_attempts": 3, "backoff_strategy": "constant"}, base[cfg.RetrySectionName])
		assert.Equal(t, map[string]any{"cidr": "10.0.0.0/16", "stage": "dev"}, base[cfg.VarsSectionName])
	})
}
