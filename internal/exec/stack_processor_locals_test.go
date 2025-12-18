package exec

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestExtractAndResolveLocals_Basic(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{}
	section := map[string]any{
		"locals": map[string]any{
			"name":     "myapp",
			"env":      "prod",
			"combined": "{{ .locals.name }}-{{ .locals.env }}",
		},
	}

	result, err := ExtractAndResolveLocals(atmosConfig, section, nil, "test.yaml")

	require.NoError(t, err)
	assert.Equal(t, "myapp", result["name"])
	assert.Equal(t, "prod", result["env"])
	assert.Equal(t, "myapp-prod", result["combined"])
}

func TestExtractAndResolveLocals_NoLocalsSection(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{}
	section := map[string]any{
		"vars": map[string]any{
			"foo": "bar",
		},
	}

	result, err := ExtractAndResolveLocals(atmosConfig, section, nil, "test.yaml")

	require.NoError(t, err)
	assert.Nil(t, result)
}

func TestExtractAndResolveLocals_EmptyLocals(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{}
	section := map[string]any{
		"locals": map[string]any{},
	}

	result, err := ExtractAndResolveLocals(atmosConfig, section, nil, "test.yaml")

	require.NoError(t, err)
	assert.NotNil(t, result)
	assert.Empty(t, result)
}

func TestExtractAndResolveLocals_WithParentLocals(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{}
	parentLocals := map[string]any{
		"global": "parent-value",
	}
	section := map[string]any{
		"locals": map[string]any{
			"child": "{{ .locals.global }}-child",
		},
	}

	result, err := ExtractAndResolveLocals(atmosConfig, section, parentLocals, "test.yaml")

	require.NoError(t, err)
	assert.Equal(t, "parent-value", result["global"])
	assert.Equal(t, "parent-value-child", result["child"])
}

func TestExtractAndResolveLocals_NoSectionWithParent(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{}
	parentLocals := map[string]any{
		"parent": "value",
	}

	result, err := ExtractAndResolveLocals(atmosConfig, nil, parentLocals, "test.yaml")

	require.NoError(t, err)
	assert.Equal(t, "value", result["parent"])
}

func TestExtractAndResolveLocals_InvalidLocalsType(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{}
	section := map[string]any{
		"locals": "not a map",
	}

	_, err := ExtractAndResolveLocals(atmosConfig, section, nil, "test.yaml")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "locals must be a map")
}

func TestExtractAndResolveLocals_CycleDetection(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{}
	section := map[string]any{
		"locals": map[string]any{
			"a": "{{ .locals.b }}",
			"b": "{{ .locals.a }}",
		},
	}

	_, err := ExtractAndResolveLocals(atmosConfig, section, nil, "test.yaml")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "circular dependency")
}

func TestProcessStackLocals_AllScopes(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{}
	stackConfig := map[string]any{
		"locals": map[string]any{
			"global_var": "global-value",
		},
		"terraform": map[string]any{
			"locals": map[string]any{
				"tf_var": "{{ .locals.global_var }}-terraform",
			},
		},
		"helmfile": map[string]any{
			"locals": map[string]any{
				"hf_var": "{{ .locals.global_var }}-helmfile",
			},
		},
		"packer": map[string]any{
			"locals": map[string]any{
				"pk_var": "{{ .locals.global_var }}-packer",
			},
		},
	}

	ctx, err := ProcessStackLocals(atmosConfig, stackConfig, "test.yaml")

	require.NoError(t, err)
	require.NotNil(t, ctx)

	// Global locals.
	assert.Equal(t, "global-value", ctx.Global["global_var"])

	// Terraform locals (merged with global).
	assert.Equal(t, "global-value", ctx.Terraform["global_var"])
	assert.Equal(t, "global-value-terraform", ctx.Terraform["tf_var"])

	// Helmfile locals (merged with global).
	assert.Equal(t, "global-value", ctx.Helmfile["global_var"])
	assert.Equal(t, "global-value-helmfile", ctx.Helmfile["hf_var"])

	// Packer locals (merged with global).
	assert.Equal(t, "global-value", ctx.Packer["global_var"])
	assert.Equal(t, "global-value-packer", ctx.Packer["pk_var"])
}

func TestProcessStackLocals_NoLocals(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{}
	stackConfig := map[string]any{
		"vars": map[string]any{
			"foo": "bar",
		},
	}

	ctx, err := ProcessStackLocals(atmosConfig, stackConfig, "test.yaml")

	require.NoError(t, err)
	require.NotNil(t, ctx)
	assert.Nil(t, ctx.Global)
	assert.Nil(t, ctx.Terraform)
	assert.Nil(t, ctx.Helmfile)
	assert.Nil(t, ctx.Packer)
}

func TestLocalsContext_GetForComponentType(t *testing.T) {
	ctx := &LocalsContext{
		Global:    map[string]any{"scope": "global"},
		Terraform: map[string]any{"scope": "terraform"},
		Helmfile:  map[string]any{"scope": "helmfile"},
		Packer:    map[string]any{"scope": "packer"},
	}

	assert.Equal(t, "terraform", ctx.GetForComponentType(cfg.TerraformSectionName)["scope"])
	assert.Equal(t, "helmfile", ctx.GetForComponentType(cfg.HelmfileSectionName)["scope"])
	assert.Equal(t, "packer", ctx.GetForComponentType(cfg.PackerSectionName)["scope"])
	assert.Equal(t, "global", ctx.GetForComponentType("unknown")["scope"])
}

func TestLocalsContext_GetForComponentType_Nil(t *testing.T) {
	var ctx *LocalsContext
	assert.Nil(t, ctx.GetForComponentType(cfg.TerraformSectionName))
}

func TestResolveComponentLocals(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{}
	parentLocals := map[string]any{
		"region": "us-east-1",
	}
	componentConfig := map[string]any{
		"locals": map[string]any{
			"name": "my-component-{{ .locals.region }}",
		},
		"vars": map[string]any{
			"foo": "bar",
		},
	}

	result, err := ResolveComponentLocals(atmosConfig, componentConfig, parentLocals, "test.yaml")

	require.NoError(t, err)
	assert.Equal(t, "us-east-1", result["region"])
	assert.Equal(t, "my-component-us-east-1", result["name"])
}

func TestStripLocalsFromSection(t *testing.T) {
	section := map[string]any{
		"locals": map[string]any{
			"foo": "bar",
		},
		"vars": map[string]any{
			"key": "value",
		},
	}

	result := StripLocalsFromSection(section)

	assert.NotContains(t, result, "locals")
	assert.Contains(t, result, "vars")
	assert.Equal(t, map[string]any{"key": "value"}, result["vars"])
}

func TestStripLocalsFromSection_NoLocals(t *testing.T) {
	section := map[string]any{
		"vars": map[string]any{
			"key": "value",
		},
	}

	result := StripLocalsFromSection(section)

	// Should return the same map.
	assert.Equal(t, section, result)
}

func TestStripLocalsFromSection_Nil(t *testing.T) {
	result := StripLocalsFromSection(nil)
	assert.Nil(t, result)
}

func TestExtractAndResolveLocals_EmptyLocalsWithParent(t *testing.T) {
	// Test that empty locals section with parent returns parent locals copy.
	atmosConfig := &schema.AtmosConfiguration{}
	parentLocals := map[string]any{
		"parent_key": "parent_value",
	}
	section := map[string]any{
		"locals": map[string]any{},
	}

	result, err := ExtractAndResolveLocals(atmosConfig, section, parentLocals, "test.yaml")

	require.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, "parent_value", result["parent_key"])
}

func TestExtractAndResolveLocals_NilSection(t *testing.T) {
	// Test with nil section.
	atmosConfig := &schema.AtmosConfiguration{}

	result, err := ExtractAndResolveLocals(atmosConfig, nil, nil, "test.yaml")

	require.NoError(t, err)
	assert.Nil(t, result)
}

func TestProcessStackLocals_GlobalError(t *testing.T) {
	// Test error handling when global locals have a cycle.
	atmosConfig := &schema.AtmosConfiguration{}
	stackConfig := map[string]any{
		"locals": map[string]any{
			"a": "{{ .locals.b }}",
			"b": "{{ .locals.a }}",
		},
	}

	_, err := ProcessStackLocals(atmosConfig, stackConfig, "test.yaml")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to resolve global locals")
}

func TestProcessStackLocals_TerraformError(t *testing.T) {
	// Test error handling when terraform locals have a cycle.
	atmosConfig := &schema.AtmosConfiguration{}
	stackConfig := map[string]any{
		"locals": map[string]any{
			"global_var": "value",
		},
		"terraform": map[string]any{
			"locals": map[string]any{
				"a": "{{ .locals.b }}",
				"b": "{{ .locals.a }}",
			},
		},
	}

	_, err := ProcessStackLocals(atmosConfig, stackConfig, "test.yaml")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to resolve terraform locals")
}

func TestProcessStackLocals_HelmfileError(t *testing.T) {
	// Test error handling when helmfile locals have a cycle.
	atmosConfig := &schema.AtmosConfiguration{}
	stackConfig := map[string]any{
		"locals": map[string]any{
			"global_var": "value",
		},
		"helmfile": map[string]any{
			"locals": map[string]any{
				"a": "{{ .locals.b }}",
				"b": "{{ .locals.a }}",
			},
		},
	}

	_, err := ProcessStackLocals(atmosConfig, stackConfig, "test.yaml")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to resolve helmfile locals")
}

func TestProcessStackLocals_PackerError(t *testing.T) {
	// Test error handling when packer locals have a cycle.
	atmosConfig := &schema.AtmosConfiguration{}
	stackConfig := map[string]any{
		"locals": map[string]any{
			"global_var": "value",
		},
		"packer": map[string]any{
			"locals": map[string]any{
				"a": "{{ .locals.b }}",
				"b": "{{ .locals.a }}",
			},
		},
	}

	_, err := ProcessStackLocals(atmosConfig, stackConfig, "test.yaml")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to resolve packer locals")
}

func TestProcessStackLocals_OnlyTerraformSection(t *testing.T) {
	// Test with only terraform section, no helmfile or packer.
	atmosConfig := &schema.AtmosConfiguration{}
	stackConfig := map[string]any{
		"locals": map[string]any{
			"global_var": "global-value",
		},
		"terraform": map[string]any{
			"locals": map[string]any{
				"tf_var": "{{ .locals.global_var }}-terraform",
			},
		},
	}

	ctx, err := ProcessStackLocals(atmosConfig, stackConfig, "test.yaml")

	require.NoError(t, err)
	require.NotNil(t, ctx)

	// Global locals.
	assert.Equal(t, "global-value", ctx.Global["global_var"])

	// Terraform locals.
	assert.Equal(t, "global-value-terraform", ctx.Terraform["tf_var"])

	// Helmfile and Packer should inherit from global.
	assert.Equal(t, ctx.Global, ctx.Helmfile)
	assert.Equal(t, ctx.Global, ctx.Packer)
}

func TestProcessStackLocals_OnlyHelmfileSection(t *testing.T) {
	// Test with only helmfile section.
	atmosConfig := &schema.AtmosConfiguration{}
	stackConfig := map[string]any{
		"locals": map[string]any{
			"global_var": "global-value",
		},
		"helmfile": map[string]any{
			"locals": map[string]any{
				"hf_var": "{{ .locals.global_var }}-helmfile",
			},
		},
	}

	ctx, err := ProcessStackLocals(atmosConfig, stackConfig, "test.yaml")

	require.NoError(t, err)
	require.NotNil(t, ctx)

	// Helmfile locals.
	assert.Equal(t, "global-value-helmfile", ctx.Helmfile["hf_var"])

	// Terraform and Packer should inherit from global.
	assert.Equal(t, ctx.Global, ctx.Terraform)
	assert.Equal(t, ctx.Global, ctx.Packer)
}

func TestProcessStackLocals_OnlyPackerSection(t *testing.T) {
	// Test with only packer section.
	atmosConfig := &schema.AtmosConfiguration{}
	stackConfig := map[string]any{
		"locals": map[string]any{
			"global_var": "global-value",
		},
		"packer": map[string]any{
			"locals": map[string]any{
				"pk_var": "{{ .locals.global_var }}-packer",
			},
		},
	}

	ctx, err := ProcessStackLocals(atmosConfig, stackConfig, "test.yaml")

	require.NoError(t, err)
	require.NotNil(t, ctx)

	// Packer locals.
	assert.Equal(t, "global-value-packer", ctx.Packer["pk_var"])

	// Terraform and Helmfile should inherit from global.
	assert.Equal(t, ctx.Global, ctx.Terraform)
	assert.Equal(t, ctx.Global, ctx.Helmfile)
}

func TestProcessStackLocals_NonMapSections(t *testing.T) {
	// Test with non-map sections (should be ignored).
	atmosConfig := &schema.AtmosConfiguration{}
	stackConfig := map[string]any{
		"locals": map[string]any{
			"global_var": "global-value",
		},
		"terraform": "not a map",
		"helmfile":  123,
		"packer":    []string{"not", "a", "map"},
	}

	ctx, err := ProcessStackLocals(atmosConfig, stackConfig, "test.yaml")

	require.NoError(t, err)
	require.NotNil(t, ctx)

	// All should inherit from global since sections are not maps.
	assert.Equal(t, ctx.Global, ctx.Terraform)
	assert.Equal(t, ctx.Global, ctx.Helmfile)
	assert.Equal(t, ctx.Global, ctx.Packer)
}

func TestResolveComponentLocals_NoLocalsSection(t *testing.T) {
	// Test component without locals section.
	atmosConfig := &schema.AtmosConfiguration{}
	parentLocals := map[string]any{
		"parent": "value",
	}
	componentConfig := map[string]any{
		"vars": map[string]any{
			"foo": "bar",
		},
	}

	result, err := ResolveComponentLocals(atmosConfig, componentConfig, parentLocals, "test.yaml")

	require.NoError(t, err)
	assert.Equal(t, "value", result["parent"])
}

func TestResolveComponentLocals_Error(t *testing.T) {
	// Test component locals with cycle.
	atmosConfig := &schema.AtmosConfiguration{}
	componentConfig := map[string]any{
		"locals": map[string]any{
			"a": "{{ .locals.b }}",
			"b": "{{ .locals.a }}",
		},
	}

	_, err := ResolveComponentLocals(atmosConfig, componentConfig, nil, "test.yaml")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "circular dependency")
}

func TestCopyParentLocals_EmptyMap(t *testing.T) {
	// Test with empty parent locals map.
	result := copyParentLocals(map[string]any{})
	assert.NotNil(t, result)
	assert.Empty(t, result)
}

func TestCopyOrCreateParentLocals_EmptyMap(t *testing.T) {
	// Test with empty parent locals map.
	result := copyOrCreateParentLocals(map[string]any{})
	assert.NotNil(t, result)
	assert.Empty(t, result)
}

func TestCopyOrCreateParentLocals_Nil(t *testing.T) {
	// Test with nil parent locals.
	result := copyOrCreateParentLocals(nil)
	assert.NotNil(t, result)
	assert.Empty(t, result)
}

func TestCopyOrCreateParentLocals_WithData(t *testing.T) {
	// Test with data.
	parentLocals := map[string]any{
		"key1": "value1",
		"key2": "value2",
	}
	result := copyOrCreateParentLocals(parentLocals)
	assert.Equal(t, parentLocals, result)
	// Verify it's a copy.
	result["key1"] = "modified"
	assert.Equal(t, "value1", parentLocals["key1"])
}
