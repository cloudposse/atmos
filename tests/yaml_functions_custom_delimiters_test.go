package tests

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	e "github.com/cloudposse/atmos/internal/exec"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
)

// TestYAMLFunctionsWithCustomDelimiters tests that YAML functions work correctly
// when custom template delimiters containing single quotes are configured.
// This is the regression test for GitHub issue #2052.
func TestYAMLFunctionsWithCustomDelimiters(t *testing.T) {
	t.Chdir("./fixtures/scenarios/atmos-terraform-state-custom-delimiters")

	atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, true)
	require.NoError(t, err)
	require.True(t, atmosConfig.Initialized, "atmos config should be initialized")

	// Verify custom delimiters are configured.
	require.Len(t, atmosConfig.Templates.Settings.Delimiters, 2)
	assert.Equal(t, "'{{", atmosConfig.Templates.Settings.Delimiters[0])
	assert.Equal(t, "}}'", atmosConfig.Templates.Settings.Delimiters[1])

	t.Run("component-1 with custom delimiter templates in regular vars", func(t *testing.T) {
		// component-1 uses custom delimiters in regular var templates:
		//   foo: "'{{ .settings.config.a }}'"
		componentSection, err := e.ExecuteDescribeComponent(
			&e.ExecuteDescribeComponentParams{
				Component:            "component-1",
				Stack:                "test",
				ProcessTemplates:     true,
				ProcessYamlFunctions: true,
			},
		)

		require.NoError(t, err, "component-1 should load without errors using custom delimiters")
		require.NotNil(t, componentSection)

		vars, ok := componentSection["vars"].(map[string]interface{})
		require.True(t, ok, "vars should be a map")

		// Verify templates were resolved correctly.
		assert.Equal(t, "component-1-a", vars["foo"], "template '{{ .settings.config.a }}' should resolve to 'component-1-a'")
		assert.Equal(t, "component-1-b", vars["bar"], "template '{{ .settings.config.b }}' should resolve to 'component-1-b'")
		assert.Equal(t, "component-1-c", vars["baz"], "template '{{ .settings.config.c }}' should resolve to 'component-1-c'")
	})

	t.Run("component-2 with static and templated terraform.state args (issue #2052)", func(t *testing.T) {
		// component-2 uses !terraform.state with both static and templated args.
		// The templated case is the core regression test for GitHub issue #2052:
		//   !terraform.state component-1 '{{ .stack }}' bar
		// With custom delimiters ["'{{", "}}'"], the template '{{ .stack }}'
		// should be processed first, then terraform.state should execute.
		componentSection, err := e.ExecuteDescribeComponent(
			&e.ExecuteDescribeComponentParams{
				Component:            "component-2",
				Stack:                "test",
				ProcessTemplates:     true,
				ProcessYamlFunctions: true,
			},
		)

		require.NoError(t, err, "component-2 should NOT fail with 'did not find expected key' (issue #2052)")
		require.NotNil(t, componentSection)

		vars, ok := componentSection["vars"].(map[string]interface{})
		require.True(t, ok, "vars should be a map")

		// Static arg: !terraform.state component-1 foo
		assert.Equal(t, "foo-from-state", vars["foo"], "static terraform.state should resolve from state file")
		// Templated arg: !terraform.state component-1 '{{ .stack }}' bar/baz
		assert.Equal(t, "bar-from-state", vars["bar"], "templated terraform.state with custom delimiters should resolve correctly")
		assert.Equal(t, "baz-from-state", vars["baz"], "templated terraform.state with custom delimiters should resolve correctly")
	})
}
