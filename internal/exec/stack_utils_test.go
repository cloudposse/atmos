package exec

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"

	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestBuildTerraformWorkspace(t *testing.T) {
	tests := []struct {
		name              string
		backendType       string
		workspacesEnabled *bool
		stack             string
		expectedWorkspace string
		shouldReturnError bool
	}{
		{
			name:              "Default behavior (workspaces enabled, non-HTTP backend)",
			backendType:       "s3",
			workspacesEnabled: nil,
			stack:             "dev/us-east-1",
			expectedWorkspace: "dev-us-east-1",
			shouldReturnError: false,
		},
		{
			name:              "HTTP backend automatically disables workspaces",
			backendType:       "http",
			workspacesEnabled: nil,
			stack:             "dev/us-east-1",
			expectedWorkspace: "default",
			shouldReturnError: false,
		},
		{
			name:              "Explicitly disabled workspaces",
			backendType:       "s3",
			workspacesEnabled: boolPtr(false),
			stack:             "dev/us-east-1",
			expectedWorkspace: "default",
			shouldReturnError: false,
		},
		{
			name:              "Explicitly enabled workspaces",
			backendType:       "s3",
			workspacesEnabled: boolPtr(true),
			stack:             "dev/us-east-1",
			expectedWorkspace: "dev-us-east-1",
			shouldReturnError: false,
		},
		{
			name:              "HTTP backend with explicitly enabled workspaces",
			backendType:       "http",
			workspacesEnabled: boolPtr(true),
			stack:             "dev/us-east-1",
			expectedWorkspace: "default",
			shouldReturnError: false,
		},
		{
			name:              "Empty stack name",
			backendType:       "s3",
			workspacesEnabled: nil,
			stack:             "",
			expectedWorkspace: "",
			shouldReturnError: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup test config.
			atmosConfig := schema.AtmosConfiguration{
				Components: schema.Components{
					Terraform: schema.Terraform{
						WorkspacesEnabled: tc.workspacesEnabled,
					},
				},
			}

			info := schema.ConfigAndStacksInfo{
				ComponentBackendType: tc.backendType,
				Component:            "test-component",
				Stack:                tc.stack,
			}

			// Test function.
			workspace, err := BuildTerraformWorkspace(&atmosConfig, info)

			// Assert results.
			if tc.shouldReturnError {
				assert.Error(t, err, "Expected error for case: %s", tc.name)
			} else {
				assert.NoError(t, err, "Did not expect error for case: %s", tc.name)
				assert.Equal(t, tc.expectedWorkspace, workspace, "Expected workspace to match for case: %s", tc.name)
			}
		})
	}
}

func TestBuildComponentPath(t *testing.T) {
	tests := []struct {
		name                string
		atmosConfig         schema.AtmosConfiguration
		componentSectionMap map[string]any
		componentType       string
		expectedPath        string
	}{
		{
			name: "terraform component",
			atmosConfig: schema.AtmosConfiguration{
				BasePath: "/base",
				Components: schema.Components{
					Terraform: schema.Terraform{
						BasePath: "terraform",
					},
				},
			},
			componentSectionMap: map[string]any{
				cfg.ComponentSectionName: "infra/networking",
			},
			componentType: cfg.TerraformComponentType,
			expectedPath:  filepath.Join("/base", "terraform", "infra/networking"),
		},
		{
			name: "helmfile component",
			atmosConfig: schema.AtmosConfiguration{
				BasePath: "/base",
				Components: schema.Components{
					Helmfile: schema.Helmfile{
						BasePath: "helmfile",
					},
				},
			},
			componentSectionMap: map[string]any{
				cfg.ComponentSectionName: "apps/frontend",
			},
			componentType: cfg.HelmfileComponentType,
			expectedPath:  filepath.Join("/base", "helmfile", "apps/frontend"),
		},
		{
			name: "packer component",
			atmosConfig: schema.AtmosConfiguration{
				BasePath: "/base",
				Components: schema.Components{
					Packer: schema.Packer{
						BasePath: "packer",
					},
				},
			},
			componentSectionMap: map[string]any{
				cfg.ComponentSectionName: "images/web",
			},
			componentType: cfg.PackerComponentType,
			expectedPath:  filepath.Join("/base", "packer", "images/web"),
		},
		{
			name: "unknown component type",
			atmosConfig: schema.AtmosConfiguration{
				BasePath: "/base",
			},
			componentSectionMap: map[string]any{
				cfg.ComponentSectionName: "test/component",
			},
			componentType: "unknown",
			expectedPath:  "",
		},
		{
			name: "missing component section",
			atmosConfig: schema.AtmosConfiguration{
				BasePath: "/base",
			},
			componentSectionMap: map[string]any{},
			componentType:       cfg.TerraformComponentType,
			expectedPath:        "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := BuildComponentPath(&tt.atmosConfig, &tt.componentSectionMap, tt.componentType)
			assert.Equal(t, tt.expectedPath, result)
		})
	}
}

func TestBuildComponentPathWithFallback(t *testing.T) {
	atmosConfig := schema.AtmosConfiguration{
		BasePath: string(filepath.Separator) + "base",
		Components: schema.Components{
			Terraform: schema.Terraform{
				BasePath: "terraform",
			},
		},
	}

	tests := []struct {
		name             string
		componentSection map[string]any
		fallback         []string
		expected         string
	}{
		{
			name: "uses fallback when component field is missing",
			componentSection: map[string]any{
				"source": map[string]any{"uri": "github.com/example/vpc"},
				"vars":   map[string]any{"name": "test"},
			},
			fallback: []string{"vpc-sourced"},
			expected: string(filepath.Separator) + filepath.Join("base", "terraform", "vpc-sourced"),
		},
		{
			name: "explicit component field takes precedence over fallback",
			componentSection: map[string]any{
				"component": "vpc",
				"vars":      map[string]any{"name": "test"},
			},
			fallback: []string{"vpc-production"},
			expected: string(filepath.Separator) + filepath.Join("base", "terraform", "vpc"),
		},
		{
			name: "returns empty when no component field and no fallback",
			componentSection: map[string]any{
				"vars": map[string]any{"name": "test"},
			},
			fallback: nil,
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := BuildComponentPath(&atmosConfig, &tt.componentSection, cfg.TerraformComponentType, tt.fallback...)
			assert.Equal(t, tt.expected, result)
		})
	}
}
