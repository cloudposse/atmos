package varfile

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/generator"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestGenerator_Name(t *testing.T) {
	g := &Generator{}
	assert.Equal(t, "varfile", g.Name())
}

func TestGenerator_DefaultFilename(t *testing.T) {
	g := &Generator{}
	assert.Equal(t, "terraform.tfvars.json", g.DefaultFilename())
}

func TestGenerator_ShouldGenerate(t *testing.T) {
	tests := []struct {
		name     string
		ctx      *generator.GeneratorContext
		expected bool
	}{
		{
			name: "returns true when VarsSection has data",
			ctx: &generator.GeneratorContext{
				VarsSection: map[string]any{
					"vpc_cidr": "10.0.0.0/16",
				},
			},
			expected: true,
		},
		{
			name: "returns false when VarsSection is empty",
			ctx: &generator.GeneratorContext{
				VarsSection: map[string]any{},
			},
			expected: false,
		},
		{
			name: "returns false when VarsSection is nil",
			ctx: &generator.GeneratorContext{
				VarsSection: nil,
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
			name: "valid context with vars",
			ctx: &generator.GeneratorContext{
				VarsSection: map[string]any{"key": "value"},
			},
			wantErr: false,
		},
		{
			name: "valid context with empty vars",
			ctx: &generator.GeneratorContext{
				VarsSection: map[string]any{},
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
			name: "generates varfile content with simple vars",
			ctx: &generator.GeneratorContext{
				VarsSection: map[string]any{
					"vpc_cidr":    "10.0.0.0/16",
					"environment": "dev",
				},
			},
			expected: map[string]any{
				"vpc_cidr":    "10.0.0.0/16",
				"environment": "dev",
			},
			wantErr: false,
		},
		{
			name: "generates varfile content with nested vars",
			ctx: &generator.GeneratorContext{
				VarsSection: map[string]any{
					"tags": map[string]any{
						"Environment": "prod",
						"Team":        "platform",
					},
					"subnets": []any{"10.0.1.0/24", "10.0.2.0/24"},
				},
			},
			expected: map[string]any{
				"tags": map[string]any{
					"Environment": "prod",
					"Team":        "platform",
				},
				"subnets": []any{"10.0.1.0/24", "10.0.2.0/24"},
			},
			wantErr: false,
		},
		{
			name: "returns nil for empty VarsSection",
			ctx: &generator.GeneratorContext{
				VarsSection: map[string]any{},
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

func TestConstructFilename(t *testing.T) {
	tests := []struct {
		name     string
		ctx      *generator.GeneratorContext
		expected string
	}{
		{
			name: "simple component without folder prefix",
			ctx: &generator.GeneratorContext{
				StackInfo: &schema.ConfigAndStacksInfo{
					ContextPrefix: "plat-dev-us-east-1",
					Component:     "vpc",
				},
			},
			expected: "plat-dev-us-east-1-vpc.terraform.tfvars.json",
		},
		{
			name: "component with folder prefix",
			ctx: &generator.GeneratorContext{
				StackInfo: &schema.ConfigAndStacksInfo{
					ContextPrefix:                 "plat-dev-us-east-1",
					Component:                     "aurora",
					ComponentFolderPrefixReplaced: "rds",
				},
			},
			expected: "plat-dev-us-east-1-rds-aurora.terraform.tfvars.json",
		},
		{
			name: "empty folder prefix uses simple format",
			ctx: &generator.GeneratorContext{
				StackInfo: &schema.ConfigAndStacksInfo{
					ContextPrefix:                 "prod",
					Component:                     "eks",
					ComponentFolderPrefixReplaced: "",
				},
			},
			expected: "prod-eks.terraform.tfvars.json",
		},
		{
			name:     "nil StackInfo returns default",
			ctx:      &generator.GeneratorContext{},
			expected: "terraform.tfvars.json",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ConstructFilename(tt.ctx)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGenerator_Registration(t *testing.T) {
	// Verify the generator is registered via init().
	gen, err := generator.GetRegistry().Get(Name)
	require.NoError(t, err)
	assert.Equal(t, Name, gen.Name())
}
