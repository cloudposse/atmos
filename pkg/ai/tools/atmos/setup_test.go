package atmos

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/ai/tools"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestRegisterTools_NoAllowList_RegistersEverything(t *testing.T) {
	registry := tools.NewRegistry()
	atmosConfig := &schema.AtmosConfiguration{}

	err := RegisterTools(registry, atmosConfig, nil)

	require.NoError(t, err)
	assert.Positive(t, registry.Count())
	_, err = registry.Get("atmos_list_stacks")
	assert.NoError(t, err, "atmos_list_stacks should be registered when no allow-list is configured")
	_, err = registry.Get("write_component_file")
	assert.NoError(t, err, "write_component_file should be registered when no allow-list is configured")
}

func TestRegisterTools_WithAllowList_FiltersRegistration(t *testing.T) {
	registry := tools.NewRegistry()
	atmosConfig := &schema.AtmosConfiguration{
		AI: schema.AISettings{
			Tools: schema.AIToolSettings{
				Allowed: []string{"atmos_list_stacks", "atmos_describe_*"},
			},
		},
	}

	err := RegisterTools(registry, atmosConfig, nil)

	require.NoError(t, err)
	assert.Equal(t, registry.Count(), len(registry.List()))

	_, err = registry.Get("atmos_list_stacks")
	assert.NoError(t, err, "atmos_list_stacks matches the allow-list exactly")

	_, err = registry.Get("atmos_describe_component")
	assert.NoError(t, err, "atmos_describe_component matches the atmos_describe_* wildcard")

	_, err = registry.Get("write_component_file")
	assert.Error(t, err, "write_component_file is not in the allow-list and must not be registered")

	const prefix = "atmos_describe_"
	for _, tool := range registry.List() {
		matched := tool.Name() == "atmos_list_stacks" || (len(tool.Name()) >= len(prefix) && tool.Name()[:len(prefix)] == prefix)
		assert.True(t, matched, "registered tool %q must match the allow-list", tool.Name())
	}
}

func TestRegisterTools_EmptyAllowList_TreatedAsUnset(t *testing.T) {
	registry := tools.NewRegistry()
	atmosConfig := &schema.AtmosConfiguration{
		AI: schema.AISettings{
			Tools: schema.AIToolSettings{
				Allowed: []string{},
			},
		},
	}

	err := RegisterTools(registry, atmosConfig, nil)

	require.NoError(t, err)
	_, err = registry.Get("write_component_file")
	assert.NoError(t, err, "an empty (unset) allow-list must not filter out any tools")
}

func TestIsToolAllowed(t *testing.T) {
	tests := []struct {
		name     string
		allowed  []string
		toolName string
		expected bool
	}{
		{"no allow-list allows anything", nil, "any_tool", true},
		{"empty allow-list allows anything", []string{}, "any_tool", true},
		{"exact match allowed", []string{"list_stacks"}, "list_stacks", true},
		{"exact match not allowed", []string{"list_stacks"}, "write_component_file", false},
		{"wildcard match allowed", []string{"describe_*"}, "describe_component", true},
		{"wildcard match not allowed", []string{"describe_*"}, "list_stacks", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			atmosConfig := &schema.AtmosConfiguration{
				AI: schema.AISettings{
					Tools: schema.AIToolSettings{Allowed: tt.allowed},
				},
			}
			assert.Equal(t, tt.expected, isToolAllowed(atmosConfig, tt.toolName))
		})
	}
}
