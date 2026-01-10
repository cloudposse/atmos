package providers

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
	assert.Equal(t, "providers", g.Name())
}

func TestGenerator_DefaultFilename(t *testing.T) {
	g := &Generator{}
	assert.Equal(t, "providers_override.tf.json", g.DefaultFilename())
}

func TestGenerator_ShouldGenerate(t *testing.T) {
	tests := []struct {
		name     string
		ctx      *generator.GeneratorContext
		expected bool
	}{
		{
			name: "returns true when ProvidersSection has data",
			ctx: &generator.GeneratorContext{
				ProvidersSection: map[string]any{
					"aws": map[string]any{"region": "us-east-1"},
				},
			},
			expected: true,
		},
		{
			name: "returns false when ProvidersSection is empty",
			ctx: &generator.GeneratorContext{
				ProvidersSection: map[string]any{},
			},
			expected: false,
		},
		{
			name: "returns false when ProvidersSection is nil",
			ctx: &generator.GeneratorContext{
				ProvidersSection: nil,
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
			name: "valid context with providers",
			ctx: &generator.GeneratorContext{
				ProvidersSection: map[string]any{
					"aws": map[string]any{"region": "us-east-1"},
				},
			},
			wantErr: false,
		},
		{
			name: "valid context with empty providers",
			ctx: &generator.GeneratorContext{
				ProvidersSection: map[string]any{},
			},
			wantErr: false,
		},
		{
			name:    "nil context returns error",
			ctx:     nil,
			wantErr: true,
			errType: generator.ErrInvalidContext,
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
			name: "generates provider override with single provider",
			ctx: &generator.GeneratorContext{
				ProvidersSection: map[string]any{
					"aws": map[string]any{
						"region": "us-east-1",
					},
				},
			},
			expected: map[string]any{
				"provider": map[string]any{
					"aws": map[string]any{
						"region": "us-east-1",
					},
				},
			},
			wantErr: false,
		},
		{
			name: "generates provider override with multiple providers",
			ctx: &generator.GeneratorContext{
				ProvidersSection: map[string]any{
					"aws": map[string]any{
						"region": "us-east-1",
					},
					"kubernetes": map[string]any{
						"config_path": "~/.kube/config",
					},
				},
			},
			expected: map[string]any{
				"provider": map[string]any{
					"aws": map[string]any{
						"region": "us-east-1",
					},
					"kubernetes": map[string]any{
						"config_path": "~/.kube/config",
					},
				},
			},
			wantErr: false,
		},
		{
			name: "generates provider override with assume_role",
			ctx: &generator.GeneratorContext{
				ProvidersSection: map[string]any{
					"aws": map[string]any{
						"region": "us-east-1",
						"assume_role": map[string]any{
							"role_arn":     "arn:aws:iam::123456789012:role/TerraformRole",
							"session_name": "atmos-vpc",
						},
					},
				},
			},
			expected: map[string]any{
				"provider": map[string]any{
					"aws": map[string]any{
						"region": "us-east-1",
						"assume_role": map[string]any{
							"role_arn":     "arn:aws:iam::123456789012:role/TerraformRole",
							"session_name": "atmos-vpc",
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "generates provider override with default_tags",
			ctx: &generator.GeneratorContext{
				ProvidersSection: map[string]any{
					"aws": map[string]any{
						"region": "us-east-1",
						"default_tags": map[string]any{
							"tags": map[string]any{
								"Environment": "prod",
								"ManagedBy":   "Atmos",
							},
						},
					},
				},
			},
			expected: map[string]any{
				"provider": map[string]any{
					"aws": map[string]any{
						"region": "us-east-1",
						"default_tags": map[string]any{
							"tags": map[string]any{
								"Environment": "prod",
								"ManagedBy":   "Atmos",
							},
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "returns nil for empty ProvidersSection",
			ctx: &generator.GeneratorContext{
				ProvidersSection: map[string]any{},
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
