package generate

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/cloudposse/atmos/pkg/schema"
)

func TestGetGenerateSectionFromComponent(t *testing.T) {
	tests := []struct {
		name             string
		componentSection map[string]any
		expectNil        bool
		expectKeys       []string
	}{
		{
			name:             "nil section",
			componentSection: nil,
			expectNil:        true,
		},
		{
			name:             "no generate section",
			componentSection: map[string]any{"vars": map[string]any{}},
			expectNil:        true,
		},
		{
			name: "has generate section",
			componentSection: map[string]any{
				"generate": map[string]any{
					"file1.json": map[string]any{},
					"file2.yaml": "content",
				},
			},
			expectNil:  false,
			expectKeys: []string{"file1.json", "file2.yaml"},
		},
		{
			name: "generate section wrong type",
			componentSection: map[string]any{
				"generate": "not a map",
			},
			expectNil: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GetGenerateSectionFromComponent(tt.componentSection)
			if tt.expectNil {
				assert.Nil(t, result)
			} else {
				assert.NotNil(t, result)
				for _, key := range tt.expectKeys {
					_, ok := result[key]
					assert.True(t, ok, "expected key %s not found", key)
				}
			}
		})
	}
}

func TestGetFilenamesForComponent(t *testing.T) {
	tests := []struct {
		name              string
		componentSection  map[string]any
		expectedFilenames []string
	}{
		{
			name:              "nil section",
			componentSection:  nil,
			expectedFilenames: nil,
		},
		{
			name: "has generate section",
			componentSection: map[string]any{
				"generate": map[string]any{
					"file1.json": map[string]any{},
					"file2.yaml": "content",
				},
			},
			expectedFilenames: []string{"file1.json", "file2.yaml"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GetFilenamesForComponent(tt.componentSection)
			if tt.expectedFilenames == nil {
				assert.Nil(t, result)
			} else {
				assert.Len(t, result, len(tt.expectedFilenames))
				for _, expected := range tt.expectedFilenames {
					assert.Contains(t, result, expected)
				}
			}
		})
	}
}

func TestBuildTemplateContext(t *testing.T) {
	info := &schema.ConfigAndStacksInfo{
		ComponentFromArg: "mycomponent",
		Stack:            "dev-us-west-2",
		StackFile:        "stacks/dev.yaml",
		FinalComponent:   "vpc",
		BaseComponent:    "vpc-base",
		Context: schema.Context{
			Namespace:   "myns",
			Tenant:      "mytenant",
			Environment: "dev",
			Stage:       "test",
			Region:      "us-west-2",
		},
		TerraformWorkspace: "dev-us-west-2-vpc",
		ComponentVarsSection: map[string]any{
			"name": "test",
		},
		ComponentSettingsSection: map[string]any{
			"version": "1.0",
		},
	}

	context := BuildTemplateContext(info)

	assert.Equal(t, "mycomponent", context["atmos_component"])
	assert.Equal(t, "dev-us-west-2", context["atmos_stack"])
	assert.Equal(t, "stacks/dev.yaml", context["atmos_stack_file"])
	assert.Equal(t, "vpc", context["component"])
	assert.Equal(t, "vpc-base", context["base_component"])
	assert.Equal(t, "myns", context["namespace"])
	assert.Equal(t, "mytenant", context["tenant"])
	assert.Equal(t, "dev", context["environment"])
	assert.Equal(t, "test", context["stage"])
	assert.Equal(t, "us-west-2", context["region"])
	assert.Equal(t, "dev-us-west-2-vpc", context["workspace"])
	assert.NotNil(t, context["vars"])
	assert.NotNil(t, context["settings"])
}

func TestBuildTemplateContextFromSection(t *testing.T) {
	componentSection := map[string]any{
		"vars": map[string]any{
			"namespace":   "ns",
			"environment": "prod",
			"name":        "test",
		},
		"settings": map[string]any{
			"version": "2.0",
		},
		"backend_type": "s3",
		"workspace":    "prod-workspace",
	}

	context := BuildTemplateContextFromSection(componentSection, "mycomp", "prod-stack")

	assert.Equal(t, "mycomp", context["atmos_component"])
	assert.Equal(t, "prod-stack", context["atmos_stack"])
	assert.Equal(t, "mycomp", context["component"])
	assert.Equal(t, "ns", context["namespace"])
	assert.Equal(t, "prod", context["environment"])
	assert.Equal(t, "s3", context["backend_type"])
	assert.Equal(t, "prod-workspace", context["workspace"])
	assert.NotNil(t, context["vars"])
	assert.NotNil(t, context["settings"])
}

func TestIsAbstractComponent(t *testing.T) {
	tests := []struct {
		name             string
		componentSection map[string]any
		expected         bool
	}{
		{
			name:             "no metadata",
			componentSection: map[string]any{},
			expected:         false,
		},
		{
			name: "metadata without type",
			componentSection: map[string]any{
				"metadata": map[string]any{
					"component": "base",
				},
			},
			expected: false,
		},
		{
			name: "abstract component",
			componentSection: map[string]any{
				"metadata": map[string]any{
					"type": "abstract",
				},
			},
			expected: true,
		},
		{
			name: "real component",
			componentSection: map[string]any{
				"metadata": map[string]any{
					"type": "real",
				},
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsAbstractComponent(tt.componentSection)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGetComponentPath(t *testing.T) {
	tests := []struct {
		name             string
		componentSection map[string]any
		componentName    string
		expected         string
	}{
		{
			name:             "no metadata",
			componentSection: map[string]any{},
			componentName:    "mycomp",
			expected:         "mycomp",
		},
		{
			name: "metadata without component",
			componentSection: map[string]any{
				"metadata": map[string]any{
					"type": "real",
				},
			},
			componentName: "mycomp",
			expected:      "mycomp",
		},
		{
			name: "metadata with component path",
			componentSection: map[string]any{
				"metadata": map[string]any{
					"component": "shared/vpc",
				},
			},
			componentName: "mycomp",
			expected:      "shared/vpc",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GetComponentPath(tt.componentSection, tt.componentName)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestMatchesStackFilter(t *testing.T) {
	tests := []struct {
		name      string
		stackName string
		filters   []string
		expected  bool
	}{
		{
			name:      "exact match",
			stackName: "dev-us-west-2",
			filters:   []string{"dev-us-west-2"},
			expected:  true,
		},
		{
			name:      "no match",
			stackName: "dev-us-west-2",
			filters:   []string{"prod-us-west-2"},
			expected:  false,
		},
		{
			name:      "glob pattern match",
			stackName: "dev-us-west-2",
			filters:   []string{"dev-*"},
			expected:  true,
		},
		{
			name:      "prefix wildcard match",
			stackName: "dev-us-west-2-vpc",
			filters:   []string{"dev-us-west-2*"},
			expected:  true,
		},
		{
			name:      "empty filters",
			stackName: "any-stack",
			filters:   []string{},
			expected:  false,
		},
		{
			name:      "multiple filters one matches",
			stackName: "dev-us-west-2",
			filters:   []string{"prod-*", "staging-*", "dev-*"},
			expected:  true,
		},
		{
			name:      "basename match - simple filter matches basename of path",
			stackName: "deploy/dev",
			filters:   []string{"dev"},
			expected:  true,
		},
		{
			name:      "basename match - nested path",
			stackName: "stacks/deploy/prod",
			filters:   []string{"prod"},
			expected:  true,
		},
		{
			name:      "basename match - full path also works",
			stackName: "deploy/dev",
			filters:   []string{"deploy/dev"},
			expected:  true,
		},
		{
			name:      "basename match - glob on full path",
			stackName: "deploy/dev",
			filters:   []string{"*/dev"},
			expected:  true,
		},
		{
			name:      "basename match - no match when filter doesn't match basename or path",
			stackName: "deploy/dev",
			filters:   []string{"staging"},
			expected:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := MatchesStackFilter(tt.stackName, tt.filters)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestExtractTerraformSection(t *testing.T) {
	tests := []struct {
		name         string
		stackSection any
		expectNil    bool
	}{
		{
			name:         "nil section",
			stackSection: nil,
			expectNil:    true,
		},
		{
			name:         "not a map",
			stackSection: "string",
			expectNil:    true,
		},
		{
			name:         "no components",
			stackSection: map[string]any{},
			expectNil:    true,
		},
		{
			name: "no terraform section",
			stackSection: map[string]any{
				"components": map[string]any{
					"helmfile": map[string]any{},
				},
			},
			expectNil: true,
		},
		{
			name: "has terraform section",
			stackSection: map[string]any{
				"components": map[string]any{
					"terraform": map[string]any{
						"vpc": map[string]any{},
					},
				},
			},
			expectNil: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ExtractTerraformSection(tt.stackSection)
			if tt.expectNil {
				assert.Nil(t, result)
			} else {
				assert.NotNil(t, result)
			}
		})
	}
}
