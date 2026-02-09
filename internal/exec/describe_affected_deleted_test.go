package exec

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
)

// TestDetectDeletedComponents_ComponentDeleted tests detection of a single component deletion.
func TestDetectDeletedComponents_ComponentDeleted(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{}

	// BASE has vpc and prometheus; HEAD has only vpc.
	remoteStacks := map[string]any{
		"dev-us-east-1": map[string]any{
			"components": map[string]any{
				cfg.TerraformComponentType: map[string]any{
					"vpc": map[string]any{
						"vars": map[string]any{"cidr": "10.0.0.0/16"},
					},
					"prometheus": map[string]any{
						"vars": map[string]any{"enabled": true},
					},
				},
			},
		},
	}

	currentStacks := map[string]any{
		"dev-us-east-1": map[string]any{
			"components": map[string]any{
				cfg.TerraformComponentType: map[string]any{
					"vpc": map[string]any{
						"vars": map[string]any{"cidr": "10.0.0.0/16"},
					},
					// prometheus is deleted in HEAD.
				},
			},
		},
	}

	deleted, err := detectDeletedComponents(&remoteStacks, &currentStacks, atmosConfig, "")
	require.NoError(t, err)
	require.Len(t, deleted, 1)

	assert.Equal(t, "prometheus", deleted[0].Component)
	assert.Equal(t, "dev-us-east-1", deleted[0].Stack)
	assert.Equal(t, cfg.TerraformComponentType, deleted[0].ComponentType)
	assert.Equal(t, affectedReasonDeleted, deleted[0].Affected)
	assert.Equal(t, deletionTypeComponent, deleted[0].DeletionType)
	assert.True(t, deleted[0].Deleted)
	assert.Contains(t, deleted[0].AffectedAll, affectedReasonDeleted)
}

// TestDetectDeletedComponents_EntireStackDeleted tests detection when an entire stack is deleted.
func TestDetectDeletedComponents_EntireStackDeleted(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{}

	// BASE has a stack with two components; HEAD doesn't have the stack.
	remoteStacks := map[string]any{
		"staging-us-west-2": map[string]any{
			"components": map[string]any{
				cfg.TerraformComponentType: map[string]any{
					"vpc": map[string]any{
						"vars": map[string]any{"cidr": "10.1.0.0/16"},
					},
					"eks": map[string]any{
						"vars": map[string]any{"cluster_name": "staging"},
					},
				},
			},
		},
	}

	// Stack doesn't exist in HEAD.
	currentStacks := map[string]any{}

	deleted, err := detectDeletedComponents(&remoteStacks, &currentStacks, atmosConfig, "")
	require.NoError(t, err)
	require.Len(t, deleted, 2)

	// Verify both components are marked as deleted with stack deletion type.
	componentNames := make(map[string]bool)
	for _, d := range deleted {
		componentNames[d.Component] = true
		assert.Equal(t, "staging-us-west-2", d.Stack)
		assert.Equal(t, affectedReasonDeletedStack, d.Affected)
		assert.Equal(t, deletionTypeStack, d.DeletionType)
		assert.True(t, d.Deleted)
	}

	assert.True(t, componentNames["vpc"])
	assert.True(t, componentNames["eks"])
}

// TestDetectDeletedComponents_AbstractComponentNotReported tests that abstract components are skipped.
func TestDetectDeletedComponents_AbstractComponentNotReported(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{}

	// BASE has an abstract component that's "deleted" in HEAD.
	remoteStacks := map[string]any{
		"dev-us-east-1": map[string]any{
			"components": map[string]any{
				cfg.TerraformComponentType: map[string]any{
					"vpc": map[string]any{
						"vars": map[string]any{"cidr": "10.0.0.0/16"},
					},
					"abstract-component": map[string]any{
						"metadata": map[string]any{
							"type": "abstract",
						},
						"vars": map[string]any{"enabled": true},
					},
				},
			},
		},
	}

	currentStacks := map[string]any{
		"dev-us-east-1": map[string]any{
			"components": map[string]any{
				cfg.TerraformComponentType: map[string]any{
					"vpc": map[string]any{
						"vars": map[string]any{"cidr": "10.0.0.0/16"},
					},
					// abstract-component is "deleted" but should not be reported.
				},
			},
		},
	}

	deleted, err := detectDeletedComponents(&remoteStacks, &currentStacks, atmosConfig, "")
	require.NoError(t, err)
	// Abstract component should not be reported as deleted.
	require.Len(t, deleted, 0)
}

// TestDetectDeletedComponents_WithStackFilter tests that --stack filter is respected.
func TestDetectDeletedComponents_WithStackFilter(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{}

	// BASE has two stacks with deleted components.
	remoteStacks := map[string]any{
		"dev-us-east-1": map[string]any{
			"components": map[string]any{
				cfg.TerraformComponentType: map[string]any{
					"prometheus": map[string]any{
						"vars": map[string]any{"enabled": true},
					},
				},
			},
		},
		"prod-us-east-1": map[string]any{
			"components": map[string]any{
				cfg.TerraformComponentType: map[string]any{
					"grafana": map[string]any{
						"vars": map[string]any{"enabled": true},
					},
				},
			},
		},
	}

	// Both components are deleted in HEAD.
	currentStacks := map[string]any{
		"dev-us-east-1": map[string]any{
			"components": map[string]any{
				cfg.TerraformComponentType: map[string]any{},
			},
		},
		"prod-us-east-1": map[string]any{
			"components": map[string]any{
				cfg.TerraformComponentType: map[string]any{},
			},
		},
	}

	// Filter to only dev stack.
	deleted, err := detectDeletedComponents(&remoteStacks, &currentStacks, atmosConfig, "dev-us-east-1")
	require.NoError(t, err)
	require.Len(t, deleted, 1)
	assert.Equal(t, "prometheus", deleted[0].Component)
	assert.Equal(t, "dev-us-east-1", deleted[0].Stack)
}

// TestDetectDeletedComponents_MultipleComponentTypes tests deletion across different component types.
func TestDetectDeletedComponents_MultipleComponentTypes(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{}

	// BASE has terraform, helmfile, and packer components.
	remoteStacks := map[string]any{
		"dev-us-east-1": map[string]any{
			"components": map[string]any{
				cfg.TerraformComponentType: map[string]any{
					"prometheus": map[string]any{
						"vars": map[string]any{"enabled": true},
					},
				},
				cfg.HelmfileComponentType: map[string]any{
					"nginx": map[string]any{
						"vars": map[string]any{"replicas": 3},
					},
				},
				cfg.PackerComponentType: map[string]any{
					"ami-builder": map[string]any{
						"vars": map[string]any{"ami_name": "test"},
					},
				},
			},
		},
	}

	// All components deleted in HEAD.
	currentStacks := map[string]any{
		"dev-us-east-1": map[string]any{
			"components": map[string]any{
				cfg.TerraformComponentType: map[string]any{},
				cfg.HelmfileComponentType:  map[string]any{},
				cfg.PackerComponentType:    map[string]any{},
			},
		},
	}

	deleted, err := detectDeletedComponents(&remoteStacks, &currentStacks, atmosConfig, "")
	require.NoError(t, err)
	require.Len(t, deleted, 3)

	// Verify all component types are represented.
	componentTypes := make(map[string]bool)
	for _, d := range deleted {
		componentTypes[d.ComponentType] = true
	}

	assert.True(t, componentTypes[cfg.TerraformComponentType])
	assert.True(t, componentTypes[cfg.HelmfileComponentType])
	assert.True(t, componentTypes[cfg.PackerComponentType])
}

// TestDetectDeletedComponents_NoComponentsSection tests when HEAD stack has no components section.
func TestDetectDeletedComponents_NoComponentsSection(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{}

	remoteStacks := map[string]any{
		"dev-us-east-1": map[string]any{
			"components": map[string]any{
				cfg.TerraformComponentType: map[string]any{
					"vpc": map[string]any{
						"vars": map[string]any{"cidr": "10.0.0.0/16"},
					},
				},
			},
		},
	}

	// Stack exists but has no components section.
	currentStacks := map[string]any{
		"dev-us-east-1": map[string]any{
			"vars": map[string]any{"stage": "dev"},
			// No components section.
		},
	}

	deleted, err := detectDeletedComponents(&remoteStacks, &currentStacks, atmosConfig, "")
	require.NoError(t, err)
	require.Len(t, deleted, 1)
	assert.Equal(t, "vpc", deleted[0].Component)
	assert.Equal(t, deletionTypeStack, deleted[0].DeletionType)
}

// TestDetectDeletedComponents_NoDeletions tests when nothing is deleted.
func TestDetectDeletedComponents_NoDeletions(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{}

	// Same stacks and components in both BASE and HEAD.
	stacks := map[string]any{
		"dev-us-east-1": map[string]any{
			"components": map[string]any{
				cfg.TerraformComponentType: map[string]any{
					"vpc": map[string]any{
						"vars": map[string]any{"cidr": "10.0.0.0/16"},
					},
				},
			},
		},
	}

	deleted, err := detectDeletedComponents(&stacks, &stacks, atmosConfig, "")
	require.NoError(t, err)
	require.Len(t, deleted, 0)
}

// TestDetectDeletedComponents_StackSlug tests that stack_slug is correctly generated.
func TestDetectDeletedComponents_StackSlug(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{}

	remoteStacks := map[string]any{
		"dev-us-east-1": map[string]any{
			"components": map[string]any{
				cfg.TerraformComponentType: map[string]any{
					"monitoring/prometheus": map[string]any{
						"vars": map[string]any{"enabled": true},
					},
				},
			},
		},
	}

	currentStacks := map[string]any{
		"dev-us-east-1": map[string]any{
			"components": map[string]any{
				cfg.TerraformComponentType: map[string]any{},
			},
		},
	}

	deleted, err := detectDeletedComponents(&remoteStacks, &currentStacks, atmosConfig, "")
	require.NoError(t, err)
	require.Len(t, deleted, 1)
	// Component name with "/" should have it replaced with "-" in stack_slug.
	assert.Equal(t, "dev-us-east-1-monitoring-prometheus", deleted[0].StackSlug)
}

// TestIsAbstractComponent tests the isAbstractComponent helper.
func TestIsAbstractComponent(t *testing.T) {
	tests := []struct {
		name             string
		componentSection map[string]any
		expected         bool
	}{
		{
			name:             "abstract component",
			componentSection: map[string]any{"metadata": map[string]any{"type": "abstract"}},
			expected:         true,
		},
		{
			name:             "real component with metadata",
			componentSection: map[string]any{"metadata": map[string]any{"type": "real"}},
			expected:         false,
		},
		{
			name:             "component without metadata type",
			componentSection: map[string]any{"metadata": map[string]any{"enabled": true}},
			expected:         false,
		},
		{
			name:             "component without metadata",
			componentSection: map[string]any{"vars": map[string]any{"enabled": true}},
			expected:         false,
		},
		{
			name:             "empty component section",
			componentSection: map[string]any{},
			expected:         false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isAbstractComponent(tt.componentSection)
			assert.Equal(t, tt.expected, result)
		})
	}
}
