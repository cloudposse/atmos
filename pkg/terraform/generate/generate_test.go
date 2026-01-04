package generate

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/auth"
	"github.com/cloudposse/atmos/pkg/schema"
)

// MockStackProcessor implements StackProcessor for testing.
type MockStackProcessor struct {
	ProcessStacksFn    func(atmosConfig *schema.AtmosConfiguration, info schema.ConfigAndStacksInfo, checkStack, processTemplates, processYamlFunctions bool, skip []string, authManager auth.AuthManager) (schema.ConfigAndStacksInfo, error)
	FindStacksMapFn    func(atmosConfig *schema.AtmosConfiguration, ignoreMissingFiles bool) (map[string]any, map[string]map[string]any, error)
	ProcessStacksCalls int
	FindStacksMapCalls int
}

//nolint:gocritic // hugeParam: mock must match interface signature which uses value type
func (m *MockStackProcessor) ProcessStacks(
	atmosConfig *schema.AtmosConfiguration,
	info schema.ConfigAndStacksInfo,
	checkStack bool,
	processTemplates bool,
	processYamlFunctions bool,
	skip []string,
	authManager auth.AuthManager,
) (schema.ConfigAndStacksInfo, error) {
	m.ProcessStacksCalls++
	if m.ProcessStacksFn != nil {
		return m.ProcessStacksFn(atmosConfig, info, checkStack, processTemplates, processYamlFunctions, skip, authManager)
	}
	return info, nil
}

func (m *MockStackProcessor) FindStacksMap(atmosConfig *schema.AtmosConfiguration, ignoreMissingFiles bool) (map[string]any, map[string]map[string]any, error) {
	m.FindStacksMapCalls++
	if m.FindStacksMapFn != nil {
		return m.FindStacksMapFn(atmosConfig, ignoreMissingFiles)
	}
	return map[string]any{}, map[string]map[string]any{}, nil
}

func TestNewService(t *testing.T) {
	mock := &MockStackProcessor{}
	service := NewService(mock)
	assert.NotNil(t, service)
}

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

func TestService_ExecuteForComponent(t *testing.T) {
	tempDir := t.TempDir()

	mock := &MockStackProcessor{
		ProcessStacksFn: func(atmosConfig *schema.AtmosConfiguration, info schema.ConfigAndStacksInfo, checkStack, processTemplates, processYamlFunctions bool, skip []string, authManager auth.AuthManager) (schema.ConfigAndStacksInfo, error) {
			info.ComponentSection = map[string]any{
				"generate": map[string]any{
					"test.json": map[string]any{
						"key": "value",
					},
				},
			}
			info.FinalComponent = "vpc"
			return info, nil
		},
	}

	service := NewService(mock)

	atmosConfig := &schema.AtmosConfiguration{
		BasePath: tempDir,
		Components: schema.Components{
			Terraform: schema.Terraform{
				BasePath: "components/terraform",
			},
		},
	}

	// Create the component directory.
	componentDir := filepath.Join(tempDir, "components", "terraform", "vpc")
	err := os.MkdirAll(componentDir, 0o755)
	require.NoError(t, err)

	err = service.ExecuteForComponent(atmosConfig, "vpc", "dev-us-west-2", false, false)
	require.NoError(t, err)

	// Verify ProcessStacks was called.
	assert.Equal(t, 1, mock.ProcessStacksCalls)

	// Verify file was created.
	content, err := os.ReadFile(filepath.Join(componentDir, "test.json"))
	require.NoError(t, err)
	assert.Contains(t, string(content), "key")
}

func TestService_ExecuteForComponent_NoGenerateSection(t *testing.T) {
	mock := &MockStackProcessor{
		ProcessStacksFn: func(atmosConfig *schema.AtmosConfiguration, info schema.ConfigAndStacksInfo, checkStack, processTemplates, processYamlFunctions bool, skip []string, authManager auth.AuthManager) (schema.ConfigAndStacksInfo, error) {
			// No generate section.
			info.ComponentSection = map[string]any{
				"vars": map[string]any{},
			}
			return info, nil
		},
	}

	service := NewService(mock)

	atmosConfig := &schema.AtmosConfiguration{
		BasePath: t.TempDir(),
	}

	// Should return nil when no generate section.
	err := service.ExecuteForComponent(atmosConfig, "vpc", "dev", false, false)
	require.NoError(t, err)
}

func TestService_ExecuteForAll(t *testing.T) {
	tempDir := t.TempDir()

	mock := &MockStackProcessor{
		FindStacksMapFn: func(atmosConfig *schema.AtmosConfiguration, ignoreMissingFiles bool) (map[string]any, map[string]map[string]any, error) {
			return map[string]any{
				"dev-us-west-2": map[string]any{
					"components": map[string]any{
						"terraform": map[string]any{
							"vpc": map[string]any{
								"generate": map[string]any{
									"output.json": map[string]any{
										"stack": "dev",
									},
								},
							},
						},
					},
				},
			}, map[string]map[string]any{}, nil
		},
	}

	service := NewService(mock)

	atmosConfig := &schema.AtmosConfiguration{
		BasePath: tempDir,
		Components: schema.Components{
			Terraform: schema.Terraform{
				BasePath: "components/terraform",
			},
		},
	}

	// Create the component directory.
	componentDir := filepath.Join(tempDir, "components", "terraform", "vpc")
	err := os.MkdirAll(componentDir, 0o755)
	require.NoError(t, err)

	err = service.ExecuteForAll(atmosConfig, nil, nil, false, false)
	require.NoError(t, err)

	// Verify FindStacksMap was called.
	assert.Equal(t, 1, mock.FindStacksMapCalls)

	// Verify file was created.
	content, err := os.ReadFile(filepath.Join(componentDir, "output.json"))
	require.NoError(t, err)
	assert.Contains(t, string(content), "stack")
}

func TestService_ExecuteForAll_WithFilters(t *testing.T) {
	mock := &MockStackProcessor{
		FindStacksMapFn: func(atmosConfig *schema.AtmosConfiguration, ignoreMissingFiles bool) (map[string]any, map[string]map[string]any, error) {
			return map[string]any{
				"dev-us-west-2":  map[string]any{},
				"prod-us-west-2": map[string]any{},
			}, map[string]map[string]any{}, nil
		},
	}

	service := NewService(mock)

	atmosConfig := &schema.AtmosConfiguration{
		BasePath: t.TempDir(),
	}

	// Filter to only dev stacks.
	err := service.ExecuteForAll(atmosConfig, []string{"dev-*"}, nil, false, false)
	require.NoError(t, err)

	// FindStacksMap should be called.
	assert.Equal(t, 1, mock.FindStacksMapCalls)
}

func TestService_GenerateFilesForComponent_Disabled(t *testing.T) {
	mock := &MockStackProcessor{}
	service := NewService(mock)

	atmosConfig := &schema.AtmosConfiguration{
		Components: schema.Components{
			Terraform: schema.Terraform{
				AutoGenerateFiles: false,
			},
		},
	}

	info := &schema.ConfigAndStacksInfo{}

	// Should return nil when auto generate is disabled.
	err := service.GenerateFilesForComponent(atmosConfig, info, "/tmp")
	require.NoError(t, err)
}

func TestService_GenerateFilesForComponent_NoGenerateSection(t *testing.T) {
	mock := &MockStackProcessor{}
	service := NewService(mock)

	atmosConfig := &schema.AtmosConfiguration{
		Components: schema.Components{
			Terraform: schema.Terraform{
				AutoGenerateFiles: true,
			},
		},
	}

	info := &schema.ConfigAndStacksInfo{
		ComponentSection: map[string]any{
			"vars": map[string]any{},
		},
	}

	// Should return nil when no generate section.
	err := service.GenerateFilesForComponent(atmosConfig, info, "/tmp")
	require.NoError(t, err)
}

func TestExecAdapter(t *testing.T) {
	var processStacksCalled bool
	var findStacksMapCalled bool

	mockProcessStacks := func(atmosConfig *schema.AtmosConfiguration, info schema.ConfigAndStacksInfo, checkStack, processTemplates, processYamlFunctions bool, skip []string, authManager auth.AuthManager) (schema.ConfigAndStacksInfo, error) {
		processStacksCalled = true
		return info, nil
	}

	mockFindStacksMap := func(atmosConfig *schema.AtmosConfiguration, ignoreMissingFiles bool) (map[string]any, map[string]map[string]any, error) {
		findStacksMapCalled = true
		return map[string]any{}, map[string]map[string]any{}, nil
	}

	adapter := NewExecAdapter(mockProcessStacks, mockFindStacksMap)

	// Test ProcessStacks.
	_, err := adapter.ProcessStacks(nil, schema.ConfigAndStacksInfo{}, false, false, false, nil, nil)
	require.NoError(t, err)
	assert.True(t, processStacksCalled)

	// Test FindStacksMap.
	_, _, err = adapter.FindStacksMap(nil, false)
	require.NoError(t, err)
	assert.True(t, findStacksMapCalled)
}
