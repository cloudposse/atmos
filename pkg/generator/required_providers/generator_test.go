package required_providers

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/generator"
)

func TestGenerator_Name(t *testing.T) {
	g := &Generator{}
	assert.Equal(t, "required_providers", g.Name())
}

func TestGenerator_DefaultFilename(t *testing.T) {
	g := &Generator{}
	assert.Equal(t, "terraform_override.tf.json", g.DefaultFilename())
}

func TestGenerator_ShouldGenerate(t *testing.T) {
	tests := []struct {
		name     string
		ctx      *generator.GeneratorContext
		expected bool
	}{
		{
			name: "returns true when RequiredVersion is set",
			ctx: &generator.GeneratorContext{
				RequiredVersion: ">= 1.10.1",
			},
			expected: true,
		},
		{
			name: "returns true when RequiredProviders has data",
			ctx: &generator.GeneratorContext{
				RequiredProviders: map[string]map[string]any{
					"aws": {"source": "hashicorp/aws", "version": "~> 5.0"},
				},
			},
			expected: true,
		},
		{
			name: "returns true when both are set",
			ctx: &generator.GeneratorContext{
				RequiredVersion: ">= 1.10.1",
				RequiredProviders: map[string]map[string]any{
					"aws": {"source": "hashicorp/aws", "version": "~> 5.0"},
				},
			},
			expected: true,
		},
		{
			name: "returns false when both are empty",
			ctx: &generator.GeneratorContext{
				RequiredVersion:   "",
				RequiredProviders: nil,
			},
			expected: false,
		},
		{
			name: "returns false when RequiredProviders is empty map",
			ctx: &generator.GeneratorContext{
				RequiredVersion:   "",
				RequiredProviders: map[string]map[string]any{},
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := &Generator{}
			result := g.ShouldGenerate(tt.ctx)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGenerator_Validate(t *testing.T) {
	tests := []struct {
		name    string
		ctx     *generator.GeneratorContext
		wantErr bool
		errType error
	}{
		{
			name: "valid context with version and providers",
			ctx: &generator.GeneratorContext{
				RequiredVersion: ">= 1.10.1",
				RequiredProviders: map[string]map[string]any{
					"aws": {"source": "hashicorp/aws", "version": "~> 5.0"},
				},
			},
			wantErr: false,
		},
		{
			name: "valid context with only version",
			ctx: &generator.GeneratorContext{
				RequiredVersion: ">= 1.5.0",
			},
			wantErr: false,
		},
		{
			name: "valid context with only providers",
			ctx: &generator.GeneratorContext{
				RequiredProviders: map[string]map[string]any{
					"aws": {"source": "hashicorp/aws"},
				},
			},
			wantErr: false,
		},
		{
			name: "valid empty context",
			ctx: &generator.GeneratorContext{
				RequiredVersion:   "",
				RequiredProviders: nil,
			},
			wantErr: false,
		},
		{
			name:    "nil context returns error",
			ctx:     nil,
			wantErr: true,
			errType: generator.ErrInvalidContext,
		},
		{
			name: "provider missing source field",
			ctx: &generator.GeneratorContext{
				RequiredProviders: map[string]map[string]any{
					"aws": {"version": "~> 5.0"}, // missing source
				},
			},
			wantErr: true,
			errType: generator.ErrMissingProviderSource,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := &Generator{}
			err := g.Validate(tt.ctx)

			if tt.wantErr {
				assert.Error(t, err)
				if tt.errType != nil {
					assert.True(t, errors.Is(err, tt.errType))
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestGenerator_Generate(t *testing.T) {
	tests := []struct {
		name     string
		ctx      *generator.GeneratorContext
		expected map[string]any
		wantErr  bool
	}{
		{
			name: "generates terraform block with version and providers",
			ctx: &generator.GeneratorContext{
				RequiredVersion: ">= 1.10.1",
				RequiredProviders: map[string]map[string]any{
					"aws": {"source": "hashicorp/aws", "version": "~> 5.0"},
				},
			},
			expected: map[string]any{
				"terraform": map[string]any{
					"required_version": ">= 1.10.1",
					"required_providers": map[string]any{
						"aws": map[string]any{
							"source":  "hashicorp/aws",
							"version": "~> 5.0",
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "generates terraform block with only version",
			ctx: &generator.GeneratorContext{
				RequiredVersion: ">= 1.5.0",
			},
			expected: map[string]any{
				"terraform": map[string]any{
					"required_version": ">= 1.5.0",
				},
			},
			wantErr: false,
		},
		{
			name: "generates terraform block with only providers",
			ctx: &generator.GeneratorContext{
				RequiredProviders: map[string]map[string]any{
					"aws":    {"source": "hashicorp/aws", "version": "~> 5.0"},
					"random": {"source": "hashicorp/random", "version": ">= 3.0"},
				},
			},
			expected: map[string]any{
				"terraform": map[string]any{
					"required_providers": map[string]any{
						"aws":    map[string]any{"source": "hashicorp/aws", "version": "~> 5.0"},
						"random": map[string]any{"source": "hashicorp/random", "version": ">= 3.0"},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "generates with complex provider config",
			ctx: &generator.GeneratorContext{
				RequiredVersion: ">= 1.10.1",
				RequiredProviders: map[string]map[string]any{
					"aws": {
						"source":  "hashicorp/aws",
						"version": "~> 5.0",
					},
					"kubernetes": {
						"source":  "hashicorp/kubernetes",
						"version": ">= 2.0",
					},
					"helm": {
						"source":  "hashicorp/helm",
						"version": "~> 2.10",
					},
				},
			},
			expected: map[string]any{
				"terraform": map[string]any{
					"required_version": ">= 1.10.1",
					"required_providers": map[string]any{
						"aws":        map[string]any{"source": "hashicorp/aws", "version": "~> 5.0"},
						"kubernetes": map[string]any{"source": "hashicorp/kubernetes", "version": ">= 2.0"},
						"helm":       map[string]any{"source": "hashicorp/helm", "version": "~> 2.10"},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "returns nil for empty context",
			ctx: &generator.GeneratorContext{
				RequiredVersion:   "",
				RequiredProviders: nil,
			},
			expected: nil,
			wantErr:  false,
		},
		{
			name: "returns nil for empty RequiredProviders map",
			ctx: &generator.GeneratorContext{
				RequiredVersion:   "",
				RequiredProviders: map[string]map[string]any{},
			},
			expected: nil,
			wantErr:  false,
		},
		{
			name:     "returns error for nil context",
			ctx:      nil,
			expected: nil,
			wantErr:  true,
		},
		{
			name: "returns error for missing source",
			ctx: &generator.GeneratorContext{
				RequiredProviders: map[string]map[string]any{
					"aws": {"version": "~> 5.0"}, // missing source
				},
			},
			expected: nil,
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := &Generator{}
			result, err := g.Generate(context.Background(), tt.ctx)

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expected, result)
			}
		})
	}
}

func TestGenerator_Registration(t *testing.T) {
	// Verify the generator is registered via init().
	gen, err := generator.GetRegistry().Get(Name)
	require.NoError(t, err)
	assert.Equal(t, Name, gen.Name())
}
