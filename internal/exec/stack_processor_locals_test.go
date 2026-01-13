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

// =============================================================================
// File-Scoped Locals Unit Tests
// =============================================================================

// TestLocalsContext_MergeForTemplateContext verifies the merge behavior for template context.
func TestLocalsContext_MergeForTemplateContext(t *testing.T) {
	ctx := &LocalsContext{
		Global: map[string]any{
			"namespace":   "global-ns",
			"environment": "global-env",
		},
		Terraform: map[string]any{
			"namespace":      "terraform-ns",
			"backend_bucket": "tf-bucket",
		},
		Helmfile: map[string]any{
			"namespace":    "helmfile-ns",
			"release_name": "hf-release",
		},
		Packer: map[string]any{
			"namespace":  "packer-ns",
			"image_name": "pk-image",
		},
		HasTerraformLocals: true,
		HasHelmfileLocals:  true,
		HasPackerLocals:    true,
	}

	merged := ctx.MergeForTemplateContext()

	// Packer (last) should win for namespace since all sections define it.
	assert.Equal(t, "packer-ns", merged["namespace"])

	// Section-specific values should be present.
	assert.Equal(t, "tf-bucket", merged["backend_bucket"])
	assert.Equal(t, "hf-release", merged["release_name"])
	assert.Equal(t, "pk-image", merged["image_name"])

	// Global-only values should be present.
	assert.Equal(t, "global-env", merged["environment"])
}

// TestLocalsContext_MergeForTemplateContext_OnlyGlobal verifies merge with only global locals.
func TestLocalsContext_MergeForTemplateContext_OnlyGlobal(t *testing.T) {
	ctx := &LocalsContext{
		Global: map[string]any{
			"namespace": "global-ns",
		},
		// No section-specific locals (flags are false).
		HasTerraformLocals: false,
		HasHelmfileLocals:  false,
		HasPackerLocals:    false,
	}

	merged := ctx.MergeForTemplateContext()

	assert.Equal(t, "global-ns", merged["namespace"])
	assert.Len(t, merged, 1)
}

// TestLocalsContext_MergeForTemplateContext_Nil verifies nil context returns nil.
func TestLocalsContext_MergeForTemplateContext_Nil(t *testing.T) {
	var ctx *LocalsContext
	merged := ctx.MergeForTemplateContext()
	assert.Nil(t, merged)
}

// TestLocalsContext_MergeForTemplateContext_EmptyGlobal verifies empty global with sections.
func TestLocalsContext_MergeForTemplateContext_EmptyGlobal(t *testing.T) {
	ctx := &LocalsContext{
		Global: map[string]any{},
		Terraform: map[string]any{
			"tf_var": "tf-value",
		},
		HasTerraformLocals: true,
	}

	merged := ctx.MergeForTemplateContext()

	assert.Equal(t, "tf-value", merged["tf_var"])
}

// TestProcessStackLocals_SectionLocalsOverrideGlobal verifies section locals override global.
func TestProcessStackLocals_SectionLocalsOverrideGlobal(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{}
	stackConfig := map[string]any{
		"locals": map[string]any{
			"namespace": "global-namespace",
			"shared":    "global-shared",
		},
		"terraform": map[string]any{
			"locals": map[string]any{
				"namespace": "terraform-namespace",
			},
		},
	}

	ctx, err := ProcessStackLocals(atmosConfig, stackConfig, "test.yaml")

	require.NoError(t, err)
	require.NotNil(t, ctx)

	// Global should have original value.
	assert.Equal(t, "global-namespace", ctx.Global["namespace"])
	assert.Equal(t, "global-shared", ctx.Global["shared"])

	// Terraform should have overridden namespace but inherit shared.
	assert.Equal(t, "terraform-namespace", ctx.Terraform["namespace"])
	assert.Equal(t, "global-shared", ctx.Terraform["shared"])
}

// TestProcessStackLocals_HasFlagsSetCorrectly verifies Has*Locals flags are set.
func TestProcessStackLocals_HasFlagsSetCorrectly(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{}

	tests := []struct {
		name            string
		stackConfig     map[string]any
		expectTerraform bool
		expectHelmfile  bool
		expectPacker    bool
	}{
		{
			name: "only terraform locals",
			stackConfig: map[string]any{
				"terraform": map[string]any{
					"locals": map[string]any{"key": "value"},
				},
			},
			expectTerraform: true,
			expectHelmfile:  false,
			expectPacker:    false,
		},
		{
			name: "only helmfile locals",
			stackConfig: map[string]any{
				"helmfile": map[string]any{
					"locals": map[string]any{"key": "value"},
				},
			},
			expectTerraform: false,
			expectHelmfile:  true,
			expectPacker:    false,
		},
		{
			name: "only packer locals",
			stackConfig: map[string]any{
				"packer": map[string]any{
					"locals": map[string]any{"key": "value"},
				},
			},
			expectTerraform: false,
			expectHelmfile:  false,
			expectPacker:    true,
		},
		{
			name: "all sections with locals",
			stackConfig: map[string]any{
				"terraform": map[string]any{
					"locals": map[string]any{"key": "value"},
				},
				"helmfile": map[string]any{
					"locals": map[string]any{"key": "value"},
				},
				"packer": map[string]any{
					"locals": map[string]any{"key": "value"},
				},
			},
			expectTerraform: true,
			expectHelmfile:  true,
			expectPacker:    true,
		},
		{
			name: "sections without locals key",
			stackConfig: map[string]any{
				"terraform": map[string]any{
					"vars": map[string]any{"key": "value"},
				},
				"helmfile": map[string]any{
					"vars": map[string]any{"key": "value"},
				},
			},
			expectTerraform: false,
			expectHelmfile:  false,
			expectPacker:    false,
		},
		{
			name: "empty locals section sets flag",
			stackConfig: map[string]any{
				"terraform": map[string]any{
					"locals": map[string]any{}, // Empty locals section.
				},
			},
			expectTerraform: true, // Flag is set based on key presence, not content.
			expectHelmfile:  false,
			expectPacker:    false,
		},
		{
			name: "all sections with empty locals",
			stackConfig: map[string]any{
				"terraform": map[string]any{
					"locals": map[string]any{},
				},
				"helmfile": map[string]any{
					"locals": map[string]any{},
				},
				"packer": map[string]any{
					"locals": map[string]any{},
				},
			},
			expectTerraform: true,
			expectHelmfile:  true,
			expectPacker:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, err := ProcessStackLocals(atmosConfig, tt.stackConfig, "test.yaml")

			require.NoError(t, err)
			require.NotNil(t, ctx)

			assert.Equal(t, tt.expectTerraform, ctx.HasTerraformLocals, "HasTerraformLocals mismatch")
			assert.Equal(t, tt.expectHelmfile, ctx.HasHelmfileLocals, "HasHelmfileLocals mismatch")
			assert.Equal(t, tt.expectPacker, ctx.HasPackerLocals, "HasPackerLocals mismatch")
		})
	}
}

// TestExtractAndResolveLocals_NestedTemplateReferences tests deeply nested template references.
func TestExtractAndResolveLocals_NestedTemplateReferences(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{}
	section := map[string]any{
		"locals": map[string]any{
			"a":     "base",
			"b":     "{{ .locals.a }}-level1",
			"c":     "{{ .locals.b }}-level2",
			"d":     "{{ .locals.c }}-level3",
			"final": "{{ .locals.d }}-final",
		},
	}

	result, err := ExtractAndResolveLocals(atmosConfig, section, nil, "test.yaml")

	require.NoError(t, err)
	assert.Equal(t, "base", result["a"])
	assert.Equal(t, "base-level1", result["b"])
	assert.Equal(t, "base-level1-level2", result["c"])
	assert.Equal(t, "base-level1-level2-level3", result["d"])
	assert.Equal(t, "base-level1-level2-level3-final", result["final"])
}

// TestExtractAndResolveLocals_MixedStaticAndTemplateValues tests mixed values.
func TestExtractAndResolveLocals_MixedStaticAndTemplateValues(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{}
	section := map[string]any{
		"locals": map[string]any{
			"static_string": "hello",
			"static_int":    42,
			"static_bool":   true,
			"static_list":   []any{"a", "b", "c"},
			"template_val":  "{{ .locals.static_string }}-world",
		},
	}

	result, err := ExtractAndResolveLocals(atmosConfig, section, nil, "test.yaml")

	require.NoError(t, err)
	assert.Equal(t, "hello", result["static_string"])
	assert.Equal(t, 42, result["static_int"])
	assert.Equal(t, true, result["static_bool"])
	assert.Equal(t, []any{"a", "b", "c"}, result["static_list"])
	assert.Equal(t, "hello-world", result["template_val"])
}

// TestExtractAndResolveLocals_ParentLocalsNotModified verifies parent locals are not modified.
func TestExtractAndResolveLocals_ParentLocalsNotModified(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{}
	parentLocals := map[string]any{
		"parent_key": "parent_value",
	}
	section := map[string]any{
		"locals": map[string]any{
			"parent_key": "child_override",
			"child_key":  "child_value",
		},
	}

	result, err := ExtractAndResolveLocals(atmosConfig, section, parentLocals, "test.yaml")

	require.NoError(t, err)

	// Result should have overridden value.
	assert.Equal(t, "child_override", result["parent_key"])
	assert.Equal(t, "child_value", result["child_key"])

	// Parent locals should NOT be modified.
	assert.Equal(t, "parent_value", parentLocals["parent_key"])
	assert.NotContains(t, parentLocals, "child_key")
}

// TestProcessStackLocals_IsolationBetweenSections verifies sections don't affect each other.
func TestProcessStackLocals_IsolationBetweenSections(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{}
	stackConfig := map[string]any{
		"locals": map[string]any{
			"shared": "global",
		},
		"terraform": map[string]any{
			"locals": map[string]any{
				"tf_only": "terraform-value",
			},
		},
		"helmfile": map[string]any{
			"locals": map[string]any{
				"hf_only": "helmfile-value",
			},
		},
	}

	ctx, err := ProcessStackLocals(atmosConfig, stackConfig, "test.yaml")

	require.NoError(t, err)
	require.NotNil(t, ctx)

	// Terraform should have its own local plus global.
	assert.Equal(t, "terraform-value", ctx.Terraform["tf_only"])
	assert.Equal(t, "global", ctx.Terraform["shared"])
	assert.NotContains(t, ctx.Terraform, "hf_only", "terraform should not have helmfile locals")

	// Helmfile should have its own local plus global.
	assert.Equal(t, "helmfile-value", ctx.Helmfile["hf_only"])
	assert.Equal(t, "global", ctx.Helmfile["shared"])
	assert.NotContains(t, ctx.Helmfile, "tf_only", "helmfile should not have terraform locals")
}

// TestMergeForTemplateContext_EmptyLocals verifies that merging empty locals sections has no effect.
func TestMergeForTemplateContext_EmptyLocals(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{}

	// Stack config with global locals and empty section locals.
	stackConfig := map[string]any{
		"locals": map[string]any{
			"namespace":   "acme",
			"environment": "prod",
		},
		"terraform": map[string]any{
			"locals": map[string]any{}, // Empty locals section.
		},
		"helmfile": map[string]any{
			"locals": map[string]any{}, // Empty locals section.
		},
	}

	ctx, err := ProcessStackLocals(atmosConfig, stackConfig, "test.yaml")
	require.NoError(t, err)
	require.NotNil(t, ctx)

	// Flags should be set because locals key exists.
	assert.True(t, ctx.HasTerraformLocals, "HasTerraformLocals should be true for empty locals")
	assert.True(t, ctx.HasHelmfileLocals, "HasHelmfileLocals should be true for empty locals")
	assert.False(t, ctx.HasPackerLocals, "HasPackerLocals should be false when not defined")

	// MergeForTemplateContext should return only global locals since section locals are empty.
	merged := ctx.MergeForTemplateContext()
	assert.Equal(t, "acme", merged["namespace"])
	assert.Equal(t, "prod", merged["environment"])
	assert.Len(t, merged, 2, "merged should only contain global locals")
}
