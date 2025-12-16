package terraform

import (
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
)

func TestParseTerraformRunOptions(t *testing.T) {
	tests := []struct {
		name     string
		setup    func(*viper.Viper)
		expected *TerraformRunOptions
	}{
		{
			name: "all flags set",
			setup: func(v *viper.Viper) {
				v.Set("process-templates", true)
				v.Set("process-functions", false)
				v.Set("skip", []string{"func1", "func2"})
				v.Set("dry-run", true)
				v.Set("query", ".components.terraform.vpc")
				v.Set("components", []string{"vpc", "eks"})
				v.Set("all", true)
				v.Set("affected", true)
			},
			expected: &TerraformRunOptions{
				ProcessTemplates: true,
				ProcessFunctions: false,
				Skip:             []string{"func1", "func2"},
				DryRun:           true,
				Query:            ".components.terraform.vpc",
				Components:       []string{"vpc", "eks"},
				All:              true,
				Affected:         true,
			},
		},
		{
			name:  "empty values (defaults)",
			setup: func(v *viper.Viper) {},
			expected: &TerraformRunOptions{
				ProcessTemplates: false,
				ProcessFunctions: false,
				Skip:             nil,
				DryRun:           false,
				Query:            "",
				Components:       nil,
				All:              false,
				Affected:         false,
			},
		},
		{
			name: "only processing flags set",
			setup: func(v *viper.Viper) {
				v.Set("process-templates", true)
				v.Set("process-functions", true)
			},
			expected: &TerraformRunOptions{
				ProcessTemplates: true,
				ProcessFunctions: true,
				Skip:             nil,
				DryRun:           false,
				Query:            "",
				Components:       nil,
				All:              false,
				Affected:         false,
			},
		},
		{
			name: "only multi-component flags set",
			setup: func(v *viper.Viper) {
				v.Set("all", true)
				v.Set("affected", false)
				v.Set("components", []string{"comp1", "comp2", "comp3"})
			},
			expected: &TerraformRunOptions{
				ProcessTemplates: false,
				ProcessFunctions: false,
				Skip:             nil,
				DryRun:           false,
				Query:            "",
				Components:       []string{"comp1", "comp2", "comp3"},
				All:              true,
				Affected:         false,
			},
		},
		{
			name: "dry-run only",
			setup: func(v *viper.Viper) {
				v.Set("dry-run", true)
			},
			expected: &TerraformRunOptions{
				ProcessTemplates: false,
				ProcessFunctions: false,
				Skip:             nil,
				DryRun:           true,
				Query:            "",
				Components:       nil,
				All:              false,
				Affected:         false,
			},
		},
		{
			name: "skip with single item",
			setup: func(v *viper.Viper) {
				v.Set("skip", []string{"template_function"})
			},
			expected: &TerraformRunOptions{
				ProcessTemplates: false,
				ProcessFunctions: false,
				Skip:             []string{"template_function"},
				DryRun:           false,
				Query:            "",
				Components:       nil,
				All:              false,
				Affected:         false,
			},
		},
		{
			name: "query only",
			setup: func(v *viper.Viper) {
				v.Set("query", ".components.terraform | keys")
			},
			expected: &TerraformRunOptions{
				ProcessTemplates: false,
				ProcessFunctions: false,
				Skip:             nil,
				DryRun:           false,
				Query:            ".components.terraform | keys",
				Components:       nil,
				All:              false,
				Affected:         false,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := viper.New()
			tt.setup(v)

			result := ParseTerraformRunOptions(v)

			assert.Equal(t, tt.expected.ProcessTemplates, result.ProcessTemplates, "ProcessTemplates should match")
			assert.Equal(t, tt.expected.ProcessFunctions, result.ProcessFunctions, "ProcessFunctions should match")
			assert.Equal(t, tt.expected.Skip, result.Skip, "Skip should match")
			assert.Equal(t, tt.expected.DryRun, result.DryRun, "DryRun should match")
			assert.Equal(t, tt.expected.Query, result.Query, "Query should match")
			assert.Equal(t, tt.expected.Components, result.Components, "Components should match")
			assert.Equal(t, tt.expected.All, result.All, "All should match")
			assert.Equal(t, tt.expected.Affected, result.Affected, "Affected should match")
		})
	}
}

func TestTerraformRunOptions_Fields(t *testing.T) {
	// Test that TerraformRunOptions struct has all expected fields.
	opts := TerraformRunOptions{
		ProcessTemplates: true,
		ProcessFunctions: true,
		Skip:             []string{"skip1"},
		DryRun:           true,
		Query:            "query",
		Components:       []string{"comp1"},
		All:              true,
		Affected:         true,
	}

	assert.True(t, opts.ProcessTemplates)
	assert.True(t, opts.ProcessFunctions)
	assert.Equal(t, []string{"skip1"}, opts.Skip)
	assert.True(t, opts.DryRun)
	assert.Equal(t, "query", opts.Query)
	assert.Equal(t, []string{"comp1"}, opts.Components)
	assert.True(t, opts.All)
	assert.True(t, opts.Affected)
}
