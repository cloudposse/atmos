package exec

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"

	errUtils "github.com/cloudposse/atmos/errors"
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

// TestBuildTerraformWorkspace_IgnoreMissingTemplateValues verifies that the global
// `templates.settings.ignore_missing_template_values` flag is honored when the stack
// `name_template` is rendered to build the Terraform workspace.
//
// Regression test for #2345: the `ProcessTmpl` call site for the name template
// hardcoded `ignoreMissingTemplateValues=false`, so a `name_template` that referenced
// a missing key always errored even when the user set the global flag to `true`.
func TestBuildTerraformWorkspace_IgnoreMissingTemplateValues(t *testing.T) {
	// The template references `.vars.missing_key`, which is absent from the component section.
	const nameTemplate = "{{ .vars.tenant }}-{{ .vars.missing_key }}"

	newConfig := func(ignoreMissing bool) *schema.AtmosConfiguration {
		return &schema.AtmosConfiguration{
			Components: schema.Components{
				Terraform: schema.Terraform{WorkspacesEnabled: boolPtr(true)},
			},
			Stacks: schema.Stacks{NameTemplate: nameTemplate},
			Templates: schema.Templates{
				Settings: schema.TemplatesSettings{IgnoreMissingTemplateValues: ignoreMissing},
			},
		}
	}

	info := schema.ConfigAndStacksInfo{
		ComponentBackendType: "s3",
		Component:            "test-component",
		Stack:                "dev/us-east-1",
		ComponentSection: map[string]any{
			"vars": map[string]any{"tenant": "acme"},
		},
	}

	t.Run("flag disabled: missing template key errors", func(t *testing.T) {
		_, err := BuildTerraformWorkspace(newConfig(false), info)
		assert.Error(t, err, "with ignore_missing_template_values=false, a missing name_template key must error")
	})

	t.Run("flag enabled: missing template key tolerated", func(t *testing.T) {
		workspace, err := BuildTerraformWorkspace(newConfig(true), info)
		assert.NoError(t, err, "with ignore_missing_template_values=true, a missing name_template key must not error")
		// `tenant` resolves; the missing key renders as `<no value>` (missingkey=default).
		assert.Equal(t, "acme-<no value>", workspace, "tenant must resolve and the missing key must not abort rendering")
	})
}

// TestBuildDependentStackNameFromDependsOnLegacy covers both resolution branches and the
// unresolved path, which now returns a wrapped static error (errUtils.ErrInvalidDependsOn).
func TestBuildDependentStackNameFromDependsOnLegacy(t *testing.T) {
	allStacks := []string{"prod-ue1", "dev-ue1"}
	componentsInStack := []string{"vpc", "eks"}

	t.Run("resolves to a stack", func(t *testing.T) {
		got, err := BuildDependentStackNameFromDependsOnLegacy("prod-ue1", allStacks, "dev-ue1", componentsInStack, "app")
		assert.NoError(t, err)
		assert.Equal(t, "prod-ue1", got)
	})

	t.Run("resolves to a component in the current stack", func(t *testing.T) {
		got, err := BuildDependentStackNameFromDependsOnLegacy("vpc", allStacks, "dev-ue1", componentsInStack, "app")
		assert.NoError(t, err)
		assert.Equal(t, "dev-ue1-vpc", got)
	})

	t.Run("unresolved dependency returns ErrInvalidDependsOn", func(t *testing.T) {
		_, err := BuildDependentStackNameFromDependsOnLegacy("nope", allStacks, "dev-ue1", componentsInStack, "app")
		assert.ErrorIs(t, err, errUtils.ErrInvalidDependsOn)
	})
}

// TestBuildDependentStackNameFromDependsOn covers the resolution and unresolved paths; the
// unresolved path now returns a wrapped static error (errUtils.ErrInvalidSettingsDependsOn).
func TestBuildDependentStackNameFromDependsOn(t *testing.T) {
	allStacks := []string{"prod-ue1-vpc", "dev-ue1-eks"}

	t.Run("resolves component in stack", func(t *testing.T) {
		got, err := BuildDependentStackNameFromDependsOn("app", "dev-ue1", "vpc", "prod-ue1", allStacks)
		assert.NoError(t, err)
		assert.Equal(t, "prod-ue1-vpc", got)
	})

	t.Run("unresolved dependency returns ErrInvalidSettingsDependsOn", func(t *testing.T) {
		_, err := BuildDependentStackNameFromDependsOn("app", "dev-ue1", "missing", "prod-ue1", allStacks)
		assert.ErrorIs(t, err, errUtils.ErrInvalidSettingsDependsOn)
	})
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
			name: "rain component",
			atmosConfig: schema.AtmosConfiguration{
				BasePath: string(filepath.Separator) + "base",
				Components: schema.Components{
					Rain: schema.Rain{
						BasePath: "rain",
					},
				},
			},
			componentSectionMap: map[string]any{
				cfg.ComponentSectionName: "cloudformation/app",
			},
			componentType: cfg.RainComponentType,
			expectedPath:  string(filepath.Separator) + filepath.Join("base", "rain", "cloudformation", "app"),
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
			result, err := BuildComponentPath(&tt.atmosConfig, &tt.componentSectionMap, tt.componentType)
			assert.NoError(t, err)
			assert.Equal(t, tt.expectedPath, result)
		})
	}

	t.Run("rain component rejects absolute component path", func(t *testing.T) {
		atmosConfig := schema.AtmosConfiguration{
			BasePath: string(filepath.Separator) + "base",
			Components: schema.Components{
				Rain: schema.Rain{
					BasePath: "rain",
				},
			},
		}
		componentSectionMap := map[string]any{
			cfg.ComponentSectionName: filepath.Join(string(filepath.Separator), "tmp", "rain"),
		}

		result, err := BuildComponentPath(&atmosConfig, &componentSectionMap, cfg.RainComponentType)

		assert.Empty(t, result)
		assert.ErrorIs(t, err, errUtils.ErrInvalidFilePath)
	})
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
			result, err := BuildComponentPath(&atmosConfig, &tt.componentSection, cfg.TerraformComponentType, tt.fallback...)
			assert.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}
}
