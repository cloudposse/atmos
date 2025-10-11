package exec

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	cfg "github.com/cloudposse/atmos/pkg/config"
)

func TestFindComponentStacks(t *testing.T) {
	tests := []struct {
		name              string
		componentType     string
		component         string
		baseComponent     string
		componentStackMap map[string]map[string][]string
		expectedStacks    []string
	}{
		{
			name:          "find terraform component in multiple stacks",
			componentType: cfg.TerraformComponentType,
			component:     "vpc",
			baseComponent: "",
			componentStackMap: map[string]map[string][]string{
				cfg.TerraformComponentType: {
					"vpc": {"stack1", "stack2", "stack3"},
				},
			},
			expectedStacks: []string{"stack1", "stack2", "stack3"},
		},
		{
			name:          "find component with base component",
			componentType: cfg.TerraformComponentType,
			component:     "vpc-prod",
			baseComponent: "vpc",
			componentStackMap: map[string]map[string][]string{
				cfg.TerraformComponentType: {
					"vpc-prod": {"stack1"},
					"vpc":      {"stack2", "stack3"},
				},
			},
			expectedStacks: []string{"stack1", "stack2", "stack3"},
		},
		{
			name:          "component not found returns empty list",
			componentType: cfg.TerraformComponentType,
			component:     "nonexistent",
			baseComponent: "",
			componentStackMap: map[string]map[string][]string{
				cfg.TerraformComponentType: {
					"vpc": {"stack1"},
				},
			},
			expectedStacks: []string{},
		},
		{
			name:              "empty component stack map",
			componentType:     cfg.TerraformComponentType,
			component:         "vpc",
			baseComponent:     "",
			componentStackMap: map[string]map[string][]string{},
			expectedStacks:    []string{},
		},
		{
			name:          "component type not found",
			componentType: cfg.HelmfileComponentType,
			component:     "app",
			baseComponent: "",
			componentStackMap: map[string]map[string][]string{
				cfg.TerraformComponentType: {
					"vpc": {"stack1"},
				},
			},
			expectedStacks: []string{},
		},
		{
			name:          "deduplicate stacks from component and base component",
			componentType: cfg.TerraformComponentType,
			component:     "vpc-prod",
			baseComponent: "vpc",
			componentStackMap: map[string]map[string][]string{
				cfg.TerraformComponentType: {
					"vpc-prod": {"stack1", "stack2"},
					"vpc":      {"stack2", "stack3"},
				},
			},
			expectedStacks: []string{"stack1", "stack2", "stack3"},
		},
		{
			name:          "helmfile component type",
			componentType: cfg.HelmfileComponentType,
			component:     "nginx",
			baseComponent: "",
			componentStackMap: map[string]map[string][]string{
				cfg.HelmfileComponentType: {
					"nginx": {"dev", "staging", "prod"},
				},
			},
			expectedStacks: []string{"dev", "prod", "staging"},
		},
		{
			name:          "base component exists but component does not",
			componentType: cfg.TerraformComponentType,
			component:     "nonexistent",
			baseComponent: "vpc",
			componentStackMap: map[string]map[string][]string{
				cfg.TerraformComponentType: {
					"vpc": {"stack1", "stack2"},
				},
			},
			expectedStacks: []string{"stack1", "stack2"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stacks, err := FindComponentStacks(tt.componentType, tt.component, tt.baseComponent, tt.componentStackMap)
			require.NoError(t, err)
			assert.Equal(t, tt.expectedStacks, stacks)
		})
	}
}

func TestFindComponentDependenciesLegacy(t *testing.T) {
	tests := []struct {
		name           string
		stack          string
		componentType  string
		component      string
		baseComponents []string
		stackImports   map[string]map[string]any
		expectedDeps   []string
	}{
		{
			name:           "import with global vars section",
			stack:          "test-stack",
			componentType:  cfg.TerraformComponentType,
			component:      "vpc",
			baseComponents: []string{},
			stackImports: map[string]map[string]any{
				"globals": {
					cfg.VarsSectionName: map[string]any{
						"region": "us-east-1",
					},
				},
			},
			expectedDeps: []string{"globals", "test-stack"},
		},
		{
			name:           "import with global backend section",
			stack:          "test-stack",
			componentType:  cfg.TerraformComponentType,
			component:      "vpc",
			baseComponents: []string{},
			stackImports: map[string]map[string]any{
				"backend-config": {
					cfg.BackendSectionName: map[string]any{
						"bucket": "tfstate",
					},
				},
			},
			expectedDeps: []string{"backend-config", "test-stack"},
		},
		{
			name:           "import with terraform-specific vars",
			stack:          "test-stack",
			componentType:  cfg.TerraformComponentType,
			component:      "vpc",
			baseComponents: []string{},
			stackImports: map[string]map[string]any{
				"terraform-globals": {
					cfg.TerraformSectionName: map[string]any{
						cfg.VarsSectionName: map[string]any{
							"terraform_version": "1.5.0",
						},
					},
				},
			},
			expectedDeps: []string{"terraform-globals", "test-stack"},
		},
		{
			name:           "import with component definition",
			stack:          "test-stack",
			componentType:  cfg.TerraformComponentType,
			component:      "vpc",
			baseComponents: []string{},
			stackImports: map[string]map[string]any{
				"vpc-config": {
					cfg.ComponentsSectionName: map[string]any{
						cfg.TerraformComponentType: map[string]any{
							"vpc": map[string]any{
								cfg.VarsSectionName: map[string]any{
									"cidr": "10.0.0.0/16",
								},
							},
						},
					},
				},
			},
			expectedDeps: []string{"test-stack", "vpc-config"},
		},
		{
			name:           "import with base component inline definition",
			stack:          "test-stack",
			componentType:  cfg.TerraformComponentType,
			component:      "vpc-prod",
			baseComponents: []string{"vpc"},
			stackImports: map[string]map[string]any{
				"vpc-base": {
					cfg.ComponentsSectionName: map[string]any{
						cfg.TerraformComponentType: map[string]any{
							"vpc": map[string]any{
								cfg.VarsSectionName: map[string]any{
									"cidr": "10.0.0.0/16",
								},
							},
						},
					},
				},
			},
			expectedDeps: []string{"test-stack", "vpc-base"},
		},
		{
			name:           "no dependencies when imports are empty",
			stack:          "test-stack",
			componentType:  cfg.TerraformComponentType,
			component:      "vpc",
			baseComponents: []string{},
			stackImports: map[string]map[string]any{
				"empty-import": {},
			},
			expectedDeps: []string{"test-stack"},
		},
		{
			name:           "no dependencies when sections exist but are empty",
			stack:          "test-stack",
			componentType:  cfg.TerraformComponentType,
			component:      "vpc",
			baseComponents: []string{},
			stackImports: map[string]map[string]any{
				"empty-sections": {
					cfg.VarsSectionName:     map[string]any{},
					cfg.SettingsSectionName: map[string]any{},
				},
			},
			expectedDeps: []string{"test-stack"},
		},
		{
			name:           "multiple imports with different sections",
			stack:          "test-stack",
			componentType:  cfg.TerraformComponentType,
			component:      "vpc",
			baseComponents: []string{},
			stackImports: map[string]map[string]any{
				"globals": {
					cfg.VarsSectionName: map[string]any{
						"region": "us-east-1",
					},
				},
				"backend": {
					cfg.BackendSectionName: map[string]any{
						"bucket": "tfstate",
					},
				},
				"env": {
					cfg.EnvSectionName: map[string]any{
						"AWS_PROFILE": "default",
					},
				},
			},
			expectedDeps: []string{"backend", "env", "globals", "test-stack"},
		},
		{
			name:           "import with env section",
			stack:          "test-stack",
			componentType:  cfg.TerraformComponentType,
			component:      "vpc",
			baseComponents: []string{},
			stackImports: map[string]map[string]any{
				"env-config": {
					cfg.EnvSectionName: map[string]any{
						"AWS_REGION": "us-west-2",
					},
				},
			},
			expectedDeps: []string{"env-config", "test-stack"},
		},
		{
			name:           "import with settings section",
			stack:          "test-stack",
			componentType:  cfg.TerraformComponentType,
			component:      "vpc",
			baseComponents: []string{},
			stackImports: map[string]map[string]any{
				"settings": {
					cfg.SettingsSectionName: map[string]any{
						"validation": true,
					},
				},
			},
			expectedDeps: []string{"settings", "test-stack"},
		},
		{
			name:           "import with remote state backend",
			stack:          "test-stack",
			componentType:  cfg.TerraformComponentType,
			component:      "app",
			baseComponents: []string{},
			stackImports: map[string]map[string]any{
				"remote-state": {
					cfg.RemoteStateBackendSectionName: map[string]any{
						"bucket": "remote-tfstate",
					},
				},
			},
			expectedDeps: []string{"remote-state", "test-stack"},
		},
		{
			name:           "import with backend type",
			stack:          "test-stack",
			componentType:  cfg.TerraformComponentType,
			component:      "vpc",
			baseComponents: []string{},
			stackImports: map[string]map[string]any{
				"backend-type": {
					cfg.BackendTypeSectionName: "s3",
				},
			},
			expectedDeps: []string{"backend-type", "test-stack"},
		},
		{
			name:           "import with remote state backend type",
			stack:          "test-stack",
			componentType:  cfg.TerraformComponentType,
			component:      "vpc",
			baseComponents: []string{},
			stackImports: map[string]map[string]any{
				"remote-backend-type": {
					cfg.RemoteStateBackendTypeSectionName: "s3",
				},
			},
			expectedDeps: []string{"remote-backend-type", "test-stack"},
		},
		{
			name:           "base component with imports that reference other files",
			stack:          "test-stack",
			componentType:  cfg.TerraformComponentType,
			component:      "vpc-prod",
			baseComponents: []string{"vpc"},
			stackImports: map[string]map[string]any{
				"vpc-base": {
					cfg.ComponentsSectionName: map[string]any{
						cfg.TerraformComponentType: map[string]any{
							"vpc": map[string]any{
								cfg.VarsSectionName: map[string]any{
									"cidr": "10.0.0.0/16",
								},
							},
						},
					},
					cfg.ImportSectionName: []any{
						"vpc-base-import",
					},
				},
				"vpc-base-import": {
					cfg.ComponentsSectionName: map[string]any{
						cfg.TerraformComponentType: map[string]any{
							"vpc": map[string]any{
								cfg.VarsSectionName: map[string]any{
									"cidr": "10.0.0.0/16",
								},
							},
						},
					},
				},
			},
			// "vpc-base" isn't included because its base component section matches the imported one
			// "vpc-base-import" IS included because it defines the base component "vpc"
			expectedDeps: []string{"test-stack", "vpc-base-import"},
		},
		{
			name:           "base component with imports - different base component sections",
			stack:          "test-stack",
			componentType:  cfg.TerraformComponentType,
			component:      "vpc-prod",
			baseComponents: []string{"vpc"},
			stackImports: map[string]map[string]any{
				"vpc-override": {
					cfg.ComponentsSectionName: map[string]any{
						cfg.TerraformComponentType: map[string]any{
							"vpc": map[string]any{
								cfg.VarsSectionName: map[string]any{
									"cidr": "192.168.0.0/16",
								},
							},
						},
					},
					cfg.ImportSectionName: []any{
						"vpc-base-different",
					},
				},
				"vpc-base-different": {
					cfg.ComponentsSectionName: map[string]any{
						cfg.TerraformComponentType: map[string]any{
							"vpc": map[string]any{
								cfg.VarsSectionName: map[string]any{
									"cidr": "10.0.0.0/16",
								},
							},
						},
					},
				},
			},
			// "vpc-override" included because it has different base component section
			// "vpc-base-different" included because it defines the base component "vpc"
			expectedDeps: []string{"test-stack", "vpc-base-different", "vpc-override"},
		},
		{
			name:           "base component with empty section",
			stack:          "test-stack",
			componentType:  cfg.TerraformComponentType,
			component:      "vpc-prod",
			baseComponents: []string{"vpc", "network"},
			stackImports: map[string]map[string]any{
				"base-components": {
					cfg.ComponentsSectionName: map[string]any{
						cfg.TerraformComponentType: map[string]any{
							"vpc": map[string]any{},
							"network": map[string]any{
								cfg.VarsSectionName: map[string]any{
									"enabled": true,
								},
							},
						},
					},
				},
			},
			expectedDeps: []string{"base-components", "test-stack"},
		},
		{
			name:           "multiple base components",
			stack:          "test-stack",
			componentType:  cfg.TerraformComponentType,
			component:      "app",
			baseComponents: []string{"base-app", "common"},
			stackImports: map[string]map[string]any{
				"app-base": {
					cfg.ComponentsSectionName: map[string]any{
						cfg.TerraformComponentType: map[string]any{
							"base-app": map[string]any{
								cfg.VarsSectionName: map[string]any{
									"app_name": "myapp",
								},
							},
						},
					},
				},
				"app-common": {
					cfg.ComponentsSectionName: map[string]any{
						cfg.TerraformComponentType: map[string]any{
							"common": map[string]any{
								cfg.SettingsSectionName: map[string]any{
									"enabled": true,
								},
							},
						},
					},
				},
			},
			expectedDeps: []string{"app-base", "app-common", "test-stack"},
		},
		{
			name:           "base component not in import components section",
			stack:          "test-stack",
			componentType:  cfg.TerraformComponentType,
			component:      "app",
			baseComponents: []string{"base-app"},
			stackImports: map[string]map[string]any{
				"some-import": {
					cfg.ComponentsSectionName: map[string]any{
						cfg.TerraformComponentType: map[string]any{
							"other-component": map[string]any{
								cfg.VarsSectionName: map[string]any{
									"foo": "bar",
								},
							},
						},
					},
				},
			},
			expectedDeps: []string{"test-stack"},
		},
		{
			name:           "import with wrong component type for base component",
			stack:          "test-stack",
			componentType:  cfg.TerraformComponentType,
			component:      "app",
			baseComponents: []string{"base-app"},
			stackImports: map[string]map[string]any{
				"helmfile-import": {
					cfg.ComponentsSectionName: map[string]any{
						cfg.HelmfileComponentType: map[string]any{
							"base-app": map[string]any{
								cfg.VarsSectionName: map[string]any{
									"foo": "bar",
								},
							},
						},
					},
				},
			},
			expectedDeps: []string{"test-stack"},
		},
		{
			name:           "import without components section for base component check",
			stack:          "test-stack",
			componentType:  cfg.TerraformComponentType,
			component:      "app",
			baseComponents: []string{"base-app"},
			stackImports: map[string]map[string]any{
				"no-components": {
					cfg.VarsSectionName: map[string]any{
						"global": "value",
					},
				},
			},
			expectedDeps: []string{"no-components", "test-stack"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deps, err := FindComponentDependenciesLegacy(
				tt.stack,
				tt.componentType,
				tt.component,
				tt.baseComponents,
				tt.stackImports,
			)
			require.NoError(t, err)
			assert.ElementsMatch(t, tt.expectedDeps, deps)
		})
	}
}
