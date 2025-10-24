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
		componentConfig map[string]any
		stackConfig     map[string]any
		want            map[string]string
		wantErr         bool
	}{
		{
			name:            "no dependencies",
			componentConfig: map[string]any{},
			stackConfig:     map[string]any{},
			want:            map[string]string{},
		},
		{
			name:            "stack-level dependencies only",
			componentConfig: map[string]any{},
			stackConfig: map[string]any{
				"dependencies": map[string]any{
					"tools": map[string]any{
						"terraform": "~> 1.10.0",
						"tflint":    "^0.54.0",
					},
				},
			},
			want: map[string]string{
				"terraform": "~> 1.10.0",
				"tflint":    "^0.54.0",
			},
		},
		{
			name: "component-level dependencies only",
			componentConfig: map[string]any{
				"dependencies": map[string]any{
					"tools": map[string]any{
						"terraform": "1.10.3",
						"checkov":   "latest",
					},
				},
			},
			stackConfig: map[string]any{},
			want: map[string]string{
				"terraform": "1.10.3",
				"checkov":   "latest",
			},
		},
		{
			name: "stack and component dependencies merged",
			componentConfig: map[string]any{
				"dependencies": map[string]any{
					"tools": map[string]any{
						"terraform": "1.10.3", // Override (satisfies ~> 1.10.0)
						"checkov":   "latest", // Add
					},
				},
			},
			stackConfig: map[string]any{
				"dependencies": map[string]any{
					"tools": map[string]any{
						"terraform": "~> 1.10.0",
						"tflint":    "^0.54.0",
					},
				},
			},
			want: map[string]string{
				"terraform": "1.10.3",
				"tflint":    "^0.54.0",
				"checkov":   "latest",
			},
		},
		{
			name: "component override violates stack constraint",
			componentConfig: map[string]any{
				"dependencies": map[string]any{
					"tools": map[string]any{
						"terraform": "1.9.8", // Does not satisfy ~> 1.10.0
					},
				},
			},
			stackConfig: map[string]any{
				"dependencies": map[string]any{
					"tools": map[string]any{
						"terraform": "~> 1.10.0",
					},
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resolver := NewResolver(&schema.AtmosConfiguration{})
			got, err := resolver.ResolveComponentDependencies(tt.componentConfig, tt.stackConfig)

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
