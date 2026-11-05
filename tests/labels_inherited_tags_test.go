package tests

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	e "github.com/cloudposse/atmos/internal/exec"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
)

// TestLabelsOverrideRendersInheritedTags verifies that a component whose `vars.tags` is a
// map-returning YAML function (`!labels`) still gets the Go templates and YAML functions of the
// `vars.tags` map it inherits rendered, exactly as it would for a plain-map override.
func TestLabelsOverrideRendersInheritedTags(t *testing.T) {
	t.Chdir("./fixtures/scenarios/atmos-labels-inherited-tags")

	describeTags := func(t *testing.T, component string) map[string]any {
		t.Helper()
		section, err := e.ExecuteDescribeComponent(&e.ExecuteDescribeComponentParams{
			Component: component, Stack: "inherited-tags",
			ProcessTemplates: true, ProcessYamlFunctions: true,
		})
		require.NoError(t, err)
		vars, ok := section["vars"].(map[string]any)
		require.True(t, ok, "vars section must be a map")
		tags, ok := vars["tags"].(map[string]any)
		require.True(t, ok, "vars.tags must be a map, got %T", vars["tags"])
		return tags
	}

	t.Run("map-returning function override renders inherited templates and functions", func(t *testing.T) {
		tags := describeTags(t, "labeled")
		assert.Equal(t, "platform", tags["cost-center"])
		assert.Equal(t, "static", tags["plain"])
		assert.Equal(t, "inherited-tags", tags["stack"])
		assert.Equal(t, "labeled", tags["component"])
		repository, ok := tags["repository"].(string)
		require.True(t, ok, "repository must be a string")
		assert.NotEmpty(t, repository)
		assert.False(t, strings.HasPrefix(repository, "!"), "YAML function left unresolved: %q", repository)
		assert.Len(t, tags, 5, "no unexpected tags: %v", tags)
	})

	// Negative path: a plain-map override has always rendered the inherited tags. It pins that the
	// fix for the function override does not change that behavior.
	t.Run("plain map override still renders inherited templates and functions", func(t *testing.T) {
		tags := describeTags(t, "plain-map")
		assert.Equal(t, "value", tags["extra"])
		assert.Equal(t, "static", tags["plain"])
		assert.Equal(t, "inherited-tags", tags["stack"])
		assert.Equal(t, "plain-map", tags["component"])
		assert.False(t, strings.HasPrefix(tags["repository"].(string), "!"))
	})
}

// TestLabelsOverrideLeavesInheritedTemplatesWhenTemplatesDisabled verifies that describing stacks
// with template processing off but YAML functions on (the `list` commands pass the two flags
// independently) resolves the inherited YAML functions but leaves the inherited Go templates
// exactly as written, because the whole-document template pass was skipped too.
func TestLabelsOverrideLeavesInheritedTemplatesWhenTemplatesDisabled(t *testing.T) {
	t.Chdir("./fixtures/scenarios/atmos-labels-inherited-tags")

	atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, true)
	require.NoError(t, err)

	describeTags := func(t *testing.T, processTemplates bool) map[string]any {
		t.Helper()
		stacks, err := e.ExecuteDescribeStacks(
			&atmosConfig, "inherited-tags", []string{"labeled"}, nil, []string{"vars"},
			false, processTemplates, true, false, nil, nil,
		)
		require.NoError(t, err)
		stack, ok := stacks["inherited-tags"].(map[string]any)
		require.True(t, ok, "stack must be a map, got %T", stacks["inherited-tags"])
		components, ok := stack["components"].(map[string]any)
		require.True(t, ok, "components must be a map")
		terraform, ok := components["terraform"].(map[string]any)
		require.True(t, ok, "terraform components must be a map")
		labeled, ok := terraform["labeled"].(map[string]any)
		require.True(t, ok, "labeled component must be a map")
		vars, ok := labeled["vars"].(map[string]any)
		require.True(t, ok, "vars section must be a map")
		tags, ok := vars["tags"].(map[string]any)
		require.True(t, ok, "vars.tags must be a map, got %T", vars["tags"])
		return tags
	}

	t.Run("templates disabled leaves inherited templates as written", func(t *testing.T) {
		tags := describeTags(t, false)
		assert.Equal(t, "{{ .atmos_stack }}", tags["stack"])
		assert.Equal(t, "{{ .atmos_component }}", tags["component"])
		assert.Equal(t, "platform", tags["cost-center"])
		repository, ok := tags["repository"].(string)
		require.True(t, ok, "repository must be a string")
		assert.False(t, strings.HasPrefix(repository, "!"), "YAML function left unresolved: %q", repository)
	})

	// Negative path: with templates enabled the same inherited strings are rendered.
	t.Run("templates enabled renders inherited templates", func(t *testing.T) {
		tags := describeTags(t, true)
		assert.Equal(t, "inherited-tags", tags["stack"])
		assert.Equal(t, "labeled", tags["component"])
	})
}

// Replaced map leaves and replaced lists must never render their missing references.
// Exercise both public describe paths with actual YAML functions and Go templates.
func TestLabelsOverrideSkipsReplacedInheritedValues(t *testing.T) {
	t.Chdir("./fixtures/scenarios/atmos-labels-inherited-tags")
	e.ClearFindStacksMapCache()
	t.Cleanup(e.ClearFindStacksMapCache)

	check := func(t *testing.T, section map[string]any) {
		t.Helper()
		vars, ok := section["vars"].(map[string]any)
		require.True(t, ok)
		assert.Equal(t, map[string]any{"owner": "platform", "stack": "replaced-inherited-tags"}, vars["tags"])
		assert.Equal(t, []any{"current"}, vars["members"])
	}

	t.Run("describe component", func(t *testing.T) {
		section, err := e.ExecuteDescribeComponent(&e.ExecuteDescribeComponentParams{
			Component: "probe", Stack: "replaced-inherited-tags",
			ProcessTemplates: true, ProcessYamlFunctions: true,
		})
		require.NoError(t, err)
		check(t, section)
	})

	t.Run("describe stacks", func(t *testing.T) {
		config, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, true)
		require.NoError(t, err)
		stacks, err := e.ExecuteDescribeStacks(
			&config, "replaced-inherited-tags", []string{"probe"}, nil, []string{"vars"},
			false, true, true, false, nil, nil,
		)
		require.NoError(t, err)
		stack := stacks["replaced-inherited-tags"].(map[string]any)
		components := stack["components"].(map[string]any)["terraform"].(map[string]any)
		check(t, components["probe"].(map[string]any))
	})
}
