package dependencies

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

func TestResolveWorkflowDependencies(t *testing.T) {
	tests := []struct {
		name        string
		workflowDef *schema.WorkflowDefinition
		want        map[string]string
	}{
		{
			name:        "nil workflow",
			workflowDef: nil,
			want:        map[string]string{},
		},
		{
			name: "workflow without dependencies",
			workflowDef: &schema.WorkflowDefinition{
				Description: "Test workflow",
			},
			want: map[string]string{},
		},
		{
			name: "workflow with dependencies",
			workflowDef: &schema.WorkflowDefinition{
				Description: "Deploy infrastructure",
				Dependencies: &schema.Dependencies{
					Tools: map[string]string{
						"terraform": "~> 1.10.0",
						"aws-cli":   "^2.0.0",
						"jq":        "latest",
					},
				},
			},
			want: map[string]string{
				"terraform": "~> 1.10.0",
				"aws-cli":   "^2.0.0",
				"jq":        "latest",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resolver := NewResolver(&schema.AtmosConfiguration{})
			got, err := resolver.ResolveWorkflowDependencies(tt.workflowDef)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestResolveCommandDependencies(t *testing.T) {
	tests := []struct {
		name    string
		command *schema.Command
		want    map[string]string
	}{
		{
			name:    "nil command",
			command: nil,
			want:    map[string]string{},
		},
		{
			name: "command without dependencies",
			command: &schema.Command{
				Name:        "deploy",
				Description: "Deploy application",
			},
			want: map[string]string{},
		},
		{
			name: "command with dependencies",
			command: &schema.Command{
				Name:        "deploy",
				Description: "Deploy with tools",
				Dependencies: &schema.Dependencies{
					Tools: map[string]string{
						"terraform": "~> 1.10.0",
						"kubectl":   "^1.32.0",
					},
				},
			},
			want: map[string]string{
				"terraform": "~> 1.10.0",
				"kubectl":   "^1.32.0",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resolver := NewResolver(&schema.AtmosConfiguration{})
			got, err := resolver.ResolveCommandDependencies(tt.command)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestResolveComponentDependencies(t *testing.T) {
	tests := []struct {
		name            string
		componentType   string
		stackConfig     map[string]any
		componentConfig map[string]any
		want            map[string]string
		wantErr         bool
	}{
		{
			name:            "no dependencies",
			componentType:   "terraform",
			stackConfig:     map[string]any{},
			componentConfig: map[string]any{},
			want:            map[string]string{},
		},
		{
			name:          "scope 1: global dependencies only",
			componentType: "terraform",
			stackConfig: map[string]any{
				"dependencies": map[string]any{
					"tools": map[string]any{
						"aws-cli": "^2.0.0",
						"jq":      "latest",
					},
				},
			},
			componentConfig: map[string]any{},
			want: map[string]string{
				"aws-cli": "^2.0.0",
				"jq":      "latest",
			},
		},
		{
			name:          "scope 2: component type dependencies only",
			componentType: "terraform",
			stackConfig: map[string]any{
				"terraform": map[string]any{
					"dependencies": map[string]any{
						"tools": map[string]any{
							"terraform": "~> 1.10.0",
							"tflint":    "^0.54.0",
						},
					},
				},
			},
			componentConfig: map[string]any{},
			want: map[string]string{
				"terraform": "~> 1.10.0",
				"tflint":    "^0.54.0",
			},
		},
		{
			name:          "scope 3: component instance dependencies only",
			componentType: "terraform",
			stackConfig:   map[string]any{},
			componentConfig: map[string]any{
				"dependencies": map[string]any{
					"tools": map[string]any{
						"terraform": "1.10.3",
						"checkov":   "latest",
					},
				},
			},
			want: map[string]string{
				"terraform": "1.10.3",
				"checkov":   "latest",
			},
		},
		{
			name:          "all 3 scopes merged with proper precedence",
			componentType: "terraform",
			stackConfig: map[string]any{
				// Scope 1: Global
				"dependencies": map[string]any{
					"tools": map[string]any{
						"aws-cli": "^2.0.0",
						"jq":      "latest",
					},
				},
				// Scope 2: Component type
				"terraform": map[string]any{
					"dependencies": map[string]any{
						"tools": map[string]any{
							"terraform": "~> 1.10.0",
							"tflint":    "^0.54.0",
						},
					},
				},
			},
			// Scope 3: Component instance
			componentConfig: map[string]any{
				"dependencies": map[string]any{
					"tools": map[string]any{
						"terraform": "1.10.3", // Overrides scope 2 (satisfies constraint)
						"checkov":   "latest", // Adds new tool
					},
				},
			},
			want: map[string]string{
				"aws-cli":   "^2.0.0",  // From scope 1
				"jq":        "latest",  // From scope 1
				"terraform": "1.10.3",  // From scope 3 (overrides scope 2)
				"tflint":    "^0.54.0", // From scope 2
				"checkov":   "latest",  // From scope 3
			},
		},
		{
			name:          "component instance override violates component type constraint",
			componentType: "terraform",
			stackConfig: map[string]any{
				"terraform": map[string]any{
					"dependencies": map[string]any{
						"tools": map[string]any{
							"terraform": "~> 1.10.0",
						},
					},
				},
			},
			componentConfig: map[string]any{
				"dependencies": map[string]any{
					"tools": map[string]any{
						"terraform": "1.9.8", // Does not satisfy ~> 1.10.0
					},
				},
			},
			wantErr: true,
		},
		{
			name:          "component type override violates global constraint",
			componentType: "terraform",
			stackConfig: map[string]any{
				"dependencies": map[string]any{
					"tools": map[string]any{
						"aws-cli": "^2.15.0", // Requires >= 2.15.0
					},
				},
				"terraform": map[string]any{
					"dependencies": map[string]any{
						"tools": map[string]any{
							"aws-cli": "2.14.0", // Does not satisfy ^2.15.0
						},
					},
				},
			},
			componentConfig: map[string]any{},
			wantErr:         true,
		},
		{
			name:          "different component type (helmfile) uses only relevant scopes",
			componentType: "helmfile",
			stackConfig: map[string]any{
				"dependencies": map[string]any{
					"tools": map[string]any{
						"aws-cli": "^2.0.0",
					},
				},
				"terraform": map[string]any{
					"dependencies": map[string]any{
						"tools": map[string]any{
							"terraform": "~> 1.10.0", // Should be ignored for helmfile
						},
					},
				},
				"helmfile": map[string]any{
					"dependencies": map[string]any{
						"tools": map[string]any{
							"helmfile": "latest",
							"kubectl":  "^1.32.0",
						},
					},
				},
			},
			componentConfig: map[string]any{},
			want: map[string]string{
				"aws-cli":  "^2.0.0",
				"helmfile": "latest",
				"kubectl":  "^1.32.0",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resolver := NewResolver(&schema.AtmosConfiguration{})
			got, err := resolver.ResolveComponentDependencies(tt.componentType, tt.stackConfig, tt.componentConfig)

			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.want, got)
			}
		})
	}
}

func TestExtractDependenciesFromConfig(t *testing.T) {
	tests := []struct {
		name   string
		config map[string]any
		want   map[string]string
	}{
		{
			name:   "nil config",
			config: nil,
			want:   nil,
		},
		{
			name:   "empty config",
			config: map[string]any{},
			want:   nil,
		},
		{
			name: "config without dependencies",
			config: map[string]any{
				"vars": map[string]any{
					"name": "vpc",
				},
			},
			want: nil,
		},
		{
			name: "config with dependencies but no tools",
			config: map[string]any{
				"dependencies": map[string]any{
					"other": "value",
				},
			},
			want: nil,
		},
		{
			name: "config with valid dependencies",
			config: map[string]any{
				"dependencies": map[string]any{
					"tools": map[string]any{
						"terraform": "1.10.3",
						"tflint":    "0.54.2",
					},
				},
			},
			want: map[string]string{
				"terraform": "1.10.3",
				"tflint":    "0.54.2",
			},
		},
		{
			name: "config with mixed types in tools (filters non-strings)",
			config: map[string]any{
				"dependencies": map[string]any{
					"tools": map[string]any{
						"terraform": "1.10.3",
						"invalid":   123, // Non-string value
					},
				},
			},
			want: map[string]string{
				"terraform": "1.10.3",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractDependenciesFromConfig(tt.config)
			assert.Equal(t, tt.want, got)
		})
	}
}
