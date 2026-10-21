package tests

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	e "github.com/cloudposse/atmos/internal/exec"
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
