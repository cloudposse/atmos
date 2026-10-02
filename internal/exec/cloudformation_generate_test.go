package exec

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
)

// TestCloudFormationGenerateMerge verifies every precedence layer through real stack processing.
func TestCloudFormationGenerateMerge(t *testing.T) {
	config := map[string]any{
		"generate":                      map[string]any{"global.txt": "global", "winner.txt": "global"},
		cfg.CloudFormationComponentType: map[string]any{"generate": map[string]any{"type.txt": "type", "winner.txt": "type"}},
		"components": map[string]any{cfg.CloudFormationComponentType: map[string]any{
			"base": map[string]any{"metadata": map[string]any{"type": "abstract"}, "generate": map[string]any{"base.txt": "base", "winner.txt": "base"}},
			"demo": map[string]any{
				"metadata": map[string]any{"inherits": []any{"base"}}, "stack_name": "demo", "path": "template.yaml",
				"generate":  map[string]any{"component.txt": "component", "winner.txt": "component"},
				"overrides": map[string]any{"generate": map[string]any{"winner.txt": "override"}},
			},
		}},
	}
	root := t.TempDir()
	result, _, err := ProcessStackConfig(&schema.AtmosConfiguration{}, root, root, root, root, root, "dev.yaml", config, false, false, "", map[string]map[string][]string{}, map[string]map[string]any{}, false)
	require.NoError(t, err)
	demo := result["components"].(map[string]any)[cfg.CloudFormationComponentType].(map[string]any)["demo"].(map[string]any)
	assert.Equal(t, map[string]any{"global.txt": "global", "type.txt": "type", "base.txt": "base", "component.txt": "component", "winner.txt": "override"}, demo["generate"])
}
