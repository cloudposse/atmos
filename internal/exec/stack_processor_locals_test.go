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
