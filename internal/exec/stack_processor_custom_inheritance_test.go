package exec

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

// inheritanceTestConfig returns a minimal config for the native deep-merge.
func inheritanceTestConfig() *schema.AtmosConfiguration {
	return &schema.AtmosConfiguration{}
}

func TestResolveCustomComponentInheritance_DeepMergesBase(t *testing.T) {
	all := map[string]any{
		"web/defaults": map[string]any{
			"metadata": map[string]any{"type": "abstract"},
			"run": map[string]any{
				"ports": []any{map[string]any{"host": 8080, "container": 80}},
			},
			"image": "base-image",
		},
		"api": map[string]any{
			"metadata": map[string]any{"inherits": []any{"web/defaults"}},
			"image":    "nginx:alpine", // overrides base image
		},
	}

	merged, err := resolveCustomComponentInheritance(inheritanceTestConfig(), all["api"].(map[string]any), all, map[string]bool{})
	require.NoError(t, err)

	// Inherited run.ports from the base.
	run, ok := merged["run"].(map[string]any)
	require.True(t, ok, "run section should be inherited")
	ports, ok := run["ports"].([]any)
	require.True(t, ok)
	require.Len(t, ports, 1)

	// Own image wins over the base image.
	assert.Equal(t, "nginx:alpine", merged["image"])
}

func TestResolveCustomComponentInheritance_AbstractDoesNotPoison(t *testing.T) {
	all := map[string]any{
		"base": map[string]any{
			"metadata": map[string]any{"type": "abstract"},
			"image":    "base",
		},
		"concrete": map[string]any{
			"metadata": map[string]any{"inherits": []any{"base"}},
			"image":    "concrete",
		},
	}

	merged, err := resolveCustomComponentInheritance(inheritanceTestConfig(), all["concrete"].(map[string]any), all, map[string]bool{})
	require.NoError(t, err)

	metadata, ok := merged["metadata"].(map[string]any)
	require.True(t, ok)
	// The concrete component must NOT have inherited `type: abstract`.
	_, hasType := metadata["type"]
	assert.False(t, hasType, "abstract type must not propagate to the concrete component")
}

func TestResolveCustomComponentInheritance_NoInheritsUnchanged(t *testing.T) {
	component := map[string]any{"image": "x"}
	merged, err := resolveCustomComponentInheritance(inheritanceTestConfig(), component, map[string]any{}, map[string]bool{})
	require.NoError(t, err)
	assert.Equal(t, "x", merged["image"])
}

func TestResolveCustomComponentInheritance_CycleGuard(t *testing.T) {
	all := map[string]any{
		"a": map[string]any{"metadata": map[string]any{"inherits": []any{"b"}}, "image": "a"},
		"b": map[string]any{"metadata": map[string]any{"inherits": []any{"a"}}, "image": "b"},
	}
	// Must terminate (not stack-overflow) despite the A↔B cycle.
	merged, err := resolveCustomComponentInheritance(inheritanceTestConfig(), all["a"].(map[string]any), all, map[string]bool{})
	require.NoError(t, err)
	assert.Equal(t, "a", merged["image"]) // self wins
}

func TestResolveCustomComponentInheritance_UnknownBaseSkipped(t *testing.T) {
	all := map[string]any{
		"api": map[string]any{"metadata": map[string]any{"inherits": []any{"missing"}}, "image": "x"},
	}
	merged, err := resolveCustomComponentInheritance(inheritanceTestConfig(), all["api"].(map[string]any), all, map[string]bool{})
	require.NoError(t, err)
	assert.Equal(t, "x", merged["image"])
}
