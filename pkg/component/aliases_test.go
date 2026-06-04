package component

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

func TestAliasMapBuiltInAndPluginAliases(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{
		Components: schema.Components{
			Terraform: schema.Terraform{Aliases: []string{"opentofu"}},
			Helmfile:  schema.Helmfile{Aliases: []string{"helm"}},
			Plugins: map[string]any{
				"cloudformation": map[string]any{
					"aliases": []any{"rain", "cft"},
				},
			},
		},
	}

	aliases, err := AliasMap(atmosConfig)
	require.NoError(t, err)

	assert.Equal(t, map[string]string{
		"opentofu": "terraform",
		"helm":     "helmfile",
		"rain":     "cloudformation",
		"cft":      "cloudformation",
	}, aliases)
	assert.Equal(t, "terraform", CanonicalType(atmosConfig, " OpenTofu "))
}

func TestAliasMapValidation(t *testing.T) {
	tests := []struct {
		name        string
		config      schema.AtmosConfiguration
		errContains string
	}{
		{
			name: "empty alias",
			config: schema.AtmosConfiguration{Components: schema.Components{
				Terraform: schema.Terraform{Aliases: []string{""}},
			}},
			errContains: "cannot be empty",
		},
		{
			name: "self alias",
			config: schema.AtmosConfiguration{Components: schema.Components{
				Terraform: schema.Terraform{Aliases: []string{"terraform"}},
			}},
			errContains: "cannot alias itself",
		},
		{
			name: "duplicate alias for same type",
			config: schema.AtmosConfiguration{Components: schema.Components{
				Terraform: schema.Terraform{Aliases: []string{"opentofu", "opentofu"}},
			}},
			errContains: "declared more than once",
		},
		{
			name: "duplicate alias across types",
			config: schema.AtmosConfiguration{Components: schema.Components{
				Terraform: schema.Terraform{Aliases: []string{"opentofu"}},
				Helmfile:  schema.Helmfile{Aliases: []string{"opentofu"}},
			}},
			errContains: "maps to both",
		},
		{
			name: "alias conflicts with registered type",
			config: schema.AtmosConfiguration{Components: schema.Components{
				Terraform: schema.Terraform{Aliases: []string{"helmfile"}},
			}},
			errContains: "conflicts with registered component type",
		},
		{
			name: "empty plugin alias",
			config: schema.AtmosConfiguration{Components: schema.Components{
				Plugins: map[string]any{"cloudformation": map[string]any{"aliases": ""}},
			}},
			errContains: "cannot be empty",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := AliasMap(&tt.config)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.errContains)
		})
	}
}

func TestNormalizeComponentSectionsMergesAliasesAndTracksEnvelope(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{
		Components: schema.Components{
			Terraform: schema.Terraform{Aliases: []string{"opentofu"}},
		},
	}
	components := map[string]any{
		"terraform": map[string]any{
			"eks": map[string]any{"vars": map[string]any{"name": "eks"}},
		},
		"opentofu": map[string]any{
			"vpc": map[string]any{"vars": map[string]any{"name": "vpc"}},
		},
	}

	normalized, envelopes, err := NormalizeComponentSections(atmosConfig, components)
	require.NoError(t, err)

	assert.NotContains(t, normalized, "opentofu")
	tf, ok := normalized["terraform"].(map[string]any)
	require.True(t, ok)
	assert.Contains(t, tf, "eks")
	assert.Contains(t, tf, "vpc")
	assert.Equal(t, "opentofu", envelopes["terraform"]["vpc"])
}

func TestNormalizeComponentSectionsRejectsDuplicateComponentsAcrossEnvelopes(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{
		Components: schema.Components{
			Terraform: schema.Terraform{Aliases: []string{"opentofu"}},
		},
	}
	components := map[string]any{
		"terraform": map[string]any{"vpc": map[string]any{}},
		"opentofu":  map[string]any{"vpc": map[string]any{}},
	}

	_, _, err := NormalizeComponentSections(atmosConfig, components)
	require.Error(t, err)
	assert.Contains(t, err.Error(), `component "vpc" is defined under both`)
}
