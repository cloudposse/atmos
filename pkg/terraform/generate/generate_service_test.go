package generate

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/cloudposse/atmos/pkg/auth"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestNewService(t *testing.T) {
	ctrl := gomock.NewController(t)
	mock := NewMockStackProcessor(ctrl)
	service := NewService(mock)
	assert.NotNil(t, service)
}

func TestService_ExecuteForComponent(t *testing.T) {
	tempDir := t.TempDir()
	ctrl := gomock.NewController(t)
	mock := NewMockStackProcessor(ctrl)

	mock.EXPECT().
		ProcessStacks(gomock.Any(), gomock.Any(), true, true, true, nil, nil).
		DoAndReturn(func(atmosConfig *schema.AtmosConfiguration, info schema.ConfigAndStacksInfo, checkStack, processTemplates, processYamlFunctions bool, skip []string, authManager auth.AuthManager) (schema.ConfigAndStacksInfo, error) {
			info.ComponentSection = map[string]any{
				"generate": map[string]any{
					"test.json": map[string]any{
						"key": "value",
					},
				},
			}
			info.FinalComponent = "vpc"
			return info, nil
		})

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

	// Verify file was created.
	content, err := os.ReadFile(filepath.Join(componentDir, "test.json"))
	require.NoError(t, err)
	assert.Contains(t, string(content), "key")
}

func TestService_ExecuteForComponent_NoGenerateSection(t *testing.T) {
	ctrl := gomock.NewController(t)
	mock := NewMockStackProcessor(ctrl)

	mock.EXPECT().
		ProcessStacks(gomock.Any(), gomock.Any(), true, true, true, nil, nil).
		DoAndReturn(func(atmosConfig *schema.AtmosConfiguration, info schema.ConfigAndStacksInfo, checkStack, processTemplates, processYamlFunctions bool, skip []string, authManager auth.AuthManager) (schema.ConfigAndStacksInfo, error) {
			// No generate section.
			info.ComponentSection = map[string]any{
				"vars": map[string]any{},
			}
			return info, nil
		})

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
	ctrl := gomock.NewController(t)
	mock := NewMockStackProcessor(ctrl)

	mock.EXPECT().
		FindStacksMap(gomock.Any(), false).
		Return(map[string]any{
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
		}, map[string]map[string]any{}, nil)

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

	// Verify file was created.
	content, err := os.ReadFile(filepath.Join(componentDir, "output.json"))
	require.NoError(t, err)
	assert.Contains(t, string(content), "stack")
}

func TestService_ExecuteForAll_WithFilters(t *testing.T) {
	ctrl := gomock.NewController(t)
	mock := NewMockStackProcessor(ctrl)

	mock.EXPECT().
		FindStacksMap(gomock.Any(), false).
		Return(map[string]any{
			"dev-us-west-2":  map[string]any{},
			"prod-us-west-2": map[string]any{},
		}, map[string]map[string]any{}, nil)

	service := NewService(mock)

	atmosConfig := &schema.AtmosConfiguration{
		BasePath: t.TempDir(),
	}

	// Filter to only dev stacks.
	err := service.ExecuteForAll(atmosConfig, []string{"dev-*"}, nil, false, false)
	require.NoError(t, err)
}

func TestService_GenerateFilesForComponent_Disabled(t *testing.T) {
	ctrl := gomock.NewController(t)
	mock := NewMockStackProcessor(ctrl)
	// No expectations - mock should not be called.

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
	ctrl := gomock.NewController(t)
	mock := NewMockStackProcessor(ctrl)
	// No expectations - mock should not be called.

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

func TestService_GenerateFilesForComponent_Success(t *testing.T) {
	tempDir := t.TempDir()
	ctrl := gomock.NewController(t)
	mock := NewMockStackProcessor(ctrl)
	// No expectations - mock should not be called for this method.

	service := NewService(mock)

	atmosConfig := &schema.AtmosConfiguration{
		Components: schema.Components{
			Terraform: schema.Terraform{
				AutoGenerateFiles: true,
			},
		},
	}

	info := &schema.ConfigAndStacksInfo{
		ComponentFromArg: "vpc",
		Stack:            "dev-us-west-2",
		ComponentSection: map[string]any{
			"generate": map[string]any{
				"auto-output.json": map[string]any{
					"generated": true,
				},
			},
		},
		ComponentVarsSection: map[string]any{
			"name": "test-vpc",
		},
	}

	// Should successfully generate files.
	err := service.GenerateFilesForComponent(atmosConfig, info, tempDir)
	require.NoError(t, err)

	// Verify file was created.
	content, err := os.ReadFile(filepath.Join(tempDir, "auto-output.json"))
	require.NoError(t, err)
	assert.Contains(t, string(content), "generated")
}

func TestService_ExecuteForAll_WithComponentFilter(t *testing.T) {
	tempDir := t.TempDir()
	ctrl := gomock.NewController(t)
	mock := NewMockStackProcessor(ctrl)

	mock.EXPECT().
		FindStacksMap(gomock.Any(), false).
		Return(map[string]any{
			"dev-us-west-2": map[string]any{
				"components": map[string]any{
					"terraform": map[string]any{
						"vpc": map[string]any{
							"generate": map[string]any{
								"output.json": map[string]any{"stack": "dev"},
							},
						},
						"rds": map[string]any{
							"generate": map[string]any{
								"output.json": map[string]any{"stack": "dev"},
							},
						},
					},
				},
			},
		}, map[string]map[string]any{}, nil)

	service := NewService(mock)

	atmosConfig := &schema.AtmosConfiguration{
		BasePath: tempDir,
		Components: schema.Components{
			Terraform: schema.Terraform{
				BasePath: "components/terraform",
			},
		},
	}

	// Create the component directories.
	vpcDir := filepath.Join(tempDir, "components", "terraform", "vpc")
	rdsDir := filepath.Join(tempDir, "components", "terraform", "rds")
	require.NoError(t, os.MkdirAll(vpcDir, 0o755))
	require.NoError(t, os.MkdirAll(rdsDir, 0o755))

	// Filter to only vpc component.
	err := service.ExecuteForAll(atmosConfig, nil, []string{"vpc"}, false, false)
	require.NoError(t, err)

	// Verify vpc file was created.
	_, err = os.Stat(filepath.Join(vpcDir, "output.json"))
	assert.NoError(t, err)

	// Verify rds file was NOT created (filtered out).
	_, err = os.Stat(filepath.Join(rdsDir, "output.json"))
	assert.True(t, os.IsNotExist(err))
}

func TestService_ExecuteForAll_AbstractComponentSkipped(t *testing.T) {
	tempDir := t.TempDir()
	ctrl := gomock.NewController(t)
	mock := NewMockStackProcessor(ctrl)

	mock.EXPECT().
		FindStacksMap(gomock.Any(), false).
		Return(map[string]any{
			"dev": map[string]any{
				"components": map[string]any{
					"terraform": map[string]any{
						"abstract-vpc": map[string]any{
							"metadata": map[string]any{
								"type": "abstract",
							},
							"generate": map[string]any{
								"output.json": map[string]any{"stack": "dev"},
							},
						},
					},
				},
			},
		}, map[string]map[string]any{}, nil)

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
	componentDir := filepath.Join(tempDir, "components", "terraform", "abstract-vpc")
	require.NoError(t, os.MkdirAll(componentDir, 0o755))

	err := service.ExecuteForAll(atmosConfig, nil, nil, false, false)
	require.NoError(t, err)

	// Verify file was NOT created (abstract component skipped).
	_, err = os.Stat(filepath.Join(componentDir, "output.json"))
	assert.True(t, os.IsNotExist(err))
}

func TestService_ExecuteForAll_ComponentWithMetadataPath(t *testing.T) {
	tempDir := t.TempDir()
	ctrl := gomock.NewController(t)
	mock := NewMockStackProcessor(ctrl)

	mock.EXPECT().
		FindStacksMap(gomock.Any(), false).
		Return(map[string]any{
			"dev": map[string]any{
				"components": map[string]any{
					"terraform": map[string]any{
						"vpc-dev": map[string]any{
							"metadata": map[string]any{
								"component": "shared/vpc",
							},
							"generate": map[string]any{
								"output.json": map[string]any{"stack": "dev"},
							},
						},
					},
				},
			},
		}, map[string]map[string]any{}, nil)

	service := NewService(mock)

	atmosConfig := &schema.AtmosConfiguration{
		BasePath: tempDir,
		Components: schema.Components{
			Terraform: schema.Terraform{
				BasePath: "components/terraform",
			},
		},
	}

	// Create the component directory using metadata component path.
	componentDir := filepath.Join(tempDir, "components", "terraform", "shared", "vpc")
	require.NoError(t, os.MkdirAll(componentDir, 0o755))

	err := service.ExecuteForAll(atmosConfig, nil, nil, false, false)
	require.NoError(t, err)

	// Verify file was created in the correct path.
	_, err = os.Stat(filepath.Join(componentDir, "output.json"))
	assert.NoError(t, err)
}

func TestService_ExecuteForAll_NoTerraformSection(t *testing.T) {
	ctrl := gomock.NewController(t)
	mock := NewMockStackProcessor(ctrl)

	mock.EXPECT().
		FindStacksMap(gomock.Any(), false).
		Return(map[string]any{
			"dev": map[string]any{
				"components": map[string]any{
					"helmfile": map[string]any{
						"chart": map[string]any{},
					},
				},
			},
		}, map[string]map[string]any{}, nil)

	service := NewService(mock)

	atmosConfig := &schema.AtmosConfiguration{
		BasePath: t.TempDir(),
	}

	// Should not error when there's no terraform section.
	err := service.ExecuteForAll(atmosConfig, nil, nil, false, false)
	require.NoError(t, err)
}

func TestService_ExecuteForAll_InvalidStackSection(t *testing.T) {
	ctrl := gomock.NewController(t)
	mock := NewMockStackProcessor(ctrl)

	mock.EXPECT().
		FindStacksMap(gomock.Any(), false).
		Return(map[string]any{
			"dev": "not a map",
		}, map[string]map[string]any{}, nil)

	service := NewService(mock)

	atmosConfig := &schema.AtmosConfiguration{
		BasePath: t.TempDir(),
	}

	// Should not error when stack section is invalid.
	err := service.ExecuteForAll(atmosConfig, nil, nil, false, false)
	require.NoError(t, err)
}

func TestService_ExecuteForAll_InvalidComponentSection(t *testing.T) {
	ctrl := gomock.NewController(t)
	mock := NewMockStackProcessor(ctrl)

	mock.EXPECT().
		FindStacksMap(gomock.Any(), false).
		Return(map[string]any{
			"dev": map[string]any{
				"components": map[string]any{
					"terraform": map[string]any{
						"vpc": "not a map", // Invalid component section.
					},
				},
			},
		}, map[string]map[string]any{}, nil)

	service := NewService(mock)

	atmosConfig := &schema.AtmosConfiguration{
		BasePath: t.TempDir(),
	}

	// Should not error when component section is invalid.
	err := service.ExecuteForAll(atmosConfig, nil, nil, false, false)
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
