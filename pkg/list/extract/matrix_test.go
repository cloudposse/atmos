package extract

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/matrix"
)

// Compile-time sentinel to ensure matrix.Entry has the fields we rely on.
var _ = matrix.Entry{Stack: "", Component: "", ComponentPath: "", ComponentType: ""}

func TestStacksMatrixEntries(t *testing.T) {
	t.Run("nil stacksMap returns nil", func(t *testing.T) {
		entries := StacksMatrixEntries(nil)
		assert.Nil(t, entries)
	})

	t.Run("empty stacksMap returns nil", func(t *testing.T) {
		entries := StacksMatrixEntries(map[string]any{})
		assert.Nil(t, entries)
	})

	t.Run("single stack with one terraform component", func(t *testing.T) {
		stacksMap := map[string]any{
			"ue1-dev": map[string]any{
				"components": map[string]any{
					"terraform": map[string]any{
						"vpc": map[string]any{
							"component_info": map[string]any{
								"component_type": "terraform",
								"component_path": "components/terraform/vpc",
							},
						},
					},
				},
			},
		}
		entries := StacksMatrixEntries(stacksMap)
		require.Len(t, entries, 1)
		assert.Equal(t, "ue1-dev", entries[0].Stack)
		assert.Equal(t, "vpc", entries[0].Component)
		assert.Equal(t, "components/terraform/vpc", entries[0].ComponentPath)
		assert.Equal(t, "terraform", entries[0].ComponentType)
	})

	t.Run("multiple stacks with multiple components", func(t *testing.T) {
		stacksMap := map[string]any{
			"ue1-dev": map[string]any{
				"components": map[string]any{
					"terraform": map[string]any{
						"vpc": map[string]any{
							"component_info": map[string]any{
								"component_path": "components/terraform/vpc",
							},
						},
						"eks": map[string]any{
							"component_info": map[string]any{
								"component_path": "components/terraform/eks",
							},
						},
					},
				},
			},
			"ue1-staging": map[string]any{
				"components": map[string]any{
					"terraform": map[string]any{
						"vpc": map[string]any{
							"component_info": map[string]any{
								"component_path": "components/terraform/vpc",
							},
						},
					},
				},
			},
		}
		entries := StacksMatrixEntries(stacksMap)
		require.Len(t, entries, 3)

		// Verify deterministic ordering (stacks and components sorted alphabetically).
		assert.Equal(t, "ue1-dev", entries[0].Stack)
		assert.Equal(t, "eks", entries[0].Component)
		assert.Equal(t, "ue1-dev", entries[1].Stack)
		assert.Equal(t, "vpc", entries[1].Component)
		assert.Equal(t, "ue1-staging", entries[2].Stack)
		assert.Equal(t, "vpc", entries[2].Component)
	})

	t.Run("component without component_info", func(t *testing.T) {
		stacksMap := map[string]any{
			"ue1-dev": map[string]any{
				"components": map[string]any{
					"terraform": map[string]any{
						"vpc": map[string]any{
							"vars": map[string]any{"region": "us-east-1"},
						},
					},
				},
			},
		}
		entries := StacksMatrixEntries(stacksMap)
		require.Len(t, entries, 1)
		assert.Equal(t, "vpc", entries[0].Component)
		assert.Equal(t, "terraform", entries[0].ComponentType)
		assert.Equal(t, "", entries[0].ComponentPath)
	})

	t.Run("stack without components key", func(t *testing.T) {
		stacksMap := map[string]any{
			"ue1-dev": map[string]any{
				"vars": map[string]any{"region": "us-east-1"},
			},
		}
		entries := StacksMatrixEntries(stacksMap)
		assert.Nil(t, entries)
	})

	t.Run("helmfile components", func(t *testing.T) {
		stacksMap := map[string]any{
			"ue1-dev": map[string]any{
				"components": map[string]any{
					"helmfile": map[string]any{
						"nginx": map[string]any{
							"component_info": map[string]any{
								"component_path": "components/helmfile/nginx",
							},
						},
					},
				},
			},
		}
		entries := StacksMatrixEntries(stacksMap)
		require.Len(t, entries, 1)
		assert.Equal(t, "nginx", entries[0].Component)
		assert.Equal(t, "helmfile", entries[0].ComponentType)
		assert.Equal(t, "components/helmfile/nginx", entries[0].ComponentPath)
	})
}
