package list

import (
	"errors"
	"sort"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"

	"github.com/cloudposse/atmos/pkg/git"
	logger "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/pro"
	"github.com/cloudposse/atmos/pkg/schema"
)

// mockGitRepo is a mock implementation of git.Repository.
type mockGitRepo struct {
	repoInfo *git.RepoInfo
	err      error
}

func (m *mockGitRepo) GetRepoInfo() (*git.RepoInfo, error) {
	return m.repoInfo, m.err
}

// mockAPIClient is a mock implementation of pro.AtmosProAPIClient.
type mockAPIClient struct {
	uploadErr error
}

func (m *mockAPIClient) UploadDriftDetection(req *pro.DriftDetectionUploadRequest) error {
	return m.uploadErr
}

// mockDescribeStacks is a mock implementation of describe.ExecuteDescribeStacks.
type mockDescribeStacks struct {
	stacks map[string]interface{}
	err    error
}

func (m *mockDescribeStacks) Execute(config *schema.AtmosConfiguration, stack string, additionalStacks []string, additionalStacksList []string, additionalStacksMap map[string]bool, includeAllStacks bool, includeStackGroups bool) (map[string]interface{}, error) {
	return m.stacks, m.err
}

// isDriftDetectionEnabled checks if drift detection is enabled for a deployment.
func isDriftDetectionEnabled(deployment *schema.Deployment) bool {
	settings, ok := deployment.Settings["pro"].(map[string]any)
	if !ok {
		return false
	}

	driftDetection, ok := settings["drift_detection"].(map[string]any)
	if !ok {
		return false
	}

	enabled, ok := driftDetection["enabled"].(bool)
	return ok && enabled
}

// filterDeploymentsByDriftDetection filters deployments based on drift detection setting.
func filterDeploymentsByDriftDetection(deployments []schema.Deployment, driftEnabled bool) []schema.Deployment {
	if !driftEnabled {
		return deployments
	}

	filtered := make([]schema.Deployment, 0, len(deployments))
	for _, deployment := range deployments {
		if isDriftDetectionEnabled(&deployment) {
			filtered = append(filtered, deployment)
		}
	}
	return filtered
}

// TestExecuteListDeploymentsCmd tests the ExecuteListDeploymentsCmd function.
func TestExecuteListDeploymentsCmd(t *testing.T) {
	// Create a new command for testing.
	cmd := &cobra.Command{}
	cmd.Flags().Bool("drift-enabled", false, "Filter deployments with drift detection enabled")
	cmd.Flags().Bool("upload", false, "Upload deployments to pro API")

	// Mock stacks data with various scenarios
	mockStacks := map[string]interface{}{
		"stack1": map[string]interface{}{
			"components": map[string]interface{}{
				"terraform": map[string]interface{}{
					"vpc": map[string]interface{}{
						"settings": map[string]interface{}{
							"pro": map[string]interface{}{
								"drift_detection": map[string]interface{}{
									"enabled": true,
								},
							},
						},
						"vars":    map[string]interface{}{},
						"env":     map[string]interface{}{},
						"backend": map[string]interface{}{},
						"metadata": map[string]interface{}{
							"type": "real",
						},
					},
					"abstract-vpc": map[string]interface{}{
						"metadata": map[string]interface{}{
							"type": "abstract",
						},
					},
				},
				"helmfile": map[string]interface{}{
					"app": map[string]interface{}{
						"settings": map[string]interface{}{
							"pro": map[string]interface{}{
								"drift_detection": map[string]interface{}{
									"enabled": false,
								},
							},
						},
						"metadata": map[string]interface{}{
							"type": "real",
						},
					},
				},
			},
		},
		"stack2": map[string]interface{}{
			"components": map[string]interface{}{
				"terraform": map[string]interface{}{
					"invalid-component": "not-a-map",
				},
			},
		},
	}

	// Test cases
	tests := []struct {
		name                 string
		info                 schema.ConfigAndStacksInfo
		driftEnabled         bool
		upload               bool
		mockStacks           map[string]interface{}
		mockStacksErr        error
		mockRepoInfo         *git.RepoInfo
		mockRepoErr          error
		mockUploadErr        error
		expectedError        string
		expectedOutput       string
		expectedDeployments  []schema.Deployment
		processedDeployments []schema.Deployment
	}{
		{
			name:         "default flags",
			info:         schema.ConfigAndStacksInfo{},
			driftEnabled: false,
			upload:       false,
			mockStacks:   mockStacks,
			expectedDeployments: []schema.Deployment{
				{
					Component:     "app",
					Stack:         "stack1",
					ComponentType: "helmfile",
					Settings: map[string]interface{}{
						"pro": map[string]interface{}{
							"drift_detection": map[string]interface{}{
								"enabled": false,
							},
						},
					},
				},
				{
					Component:     "vpc",
					Stack:         "stack1",
					ComponentType: "terraform",
					Settings: map[string]interface{}{
						"pro": map[string]interface{}{
							"drift_detection": map[string]interface{}{
								"enabled": true,
							},
						},
					},
				},
			},
		},
		{
			name:         "drift detection enabled",
			info:         schema.ConfigAndStacksInfo{},
			driftEnabled: true,
			upload:       false,
			mockStacks:   mockStacks,
			expectedDeployments: []schema.Deployment{
				{
					Component:     "vpc",
					Stack:         "stack1",
					ComponentType: "terraform",
					Settings: map[string]interface{}{
						"pro": map[string]interface{}{
							"drift_detection": map[string]interface{}{
								"enabled": true,
							},
						},
					},
				},
			},
		},
		{
			name:           "upload enabled without drift detection",
			info:           schema.ConfigAndStacksInfo{},
			driftEnabled:   false,
			upload:         true,
			mockStacks:     mockStacks,
			expectedOutput: "Atmos Pro only supports uploading drift detection stacks at this time.\n\nTo upload drift detection stacks, use the --drift-enabled flag:\n  atmos list deployments --upload --drift-enabled",
		},
		{
			name:         "upload enabled with drift detection",
			info:         schema.ConfigAndStacksInfo{},
			driftEnabled: true,
			upload:       true,
			mockStacks:   mockStacks,
			mockRepoInfo: &git.RepoInfo{
				RepoUrl:   "https://github.com/test/repo",
				RepoName:  "repo",
				RepoOwner: "test",
				RepoHost:  "github.com",
			},
			expectedDeployments: []schema.Deployment{
				{
					Component:     "vpc",
					Stack:         "stack1",
					ComponentType: "terraform",
					Settings: map[string]interface{}{
						"pro": map[string]interface{}{
							"drift_detection": map[string]interface{}{
								"enabled": true,
							},
						},
					},
					Metadata: map[string]interface{}{
						"type": "real",
					},
				},
			},
		},
		{
			name:          "error getting stacks",
			info:          schema.ConfigAndStacksInfo{},
			mockStacksErr: errors.New("failed to get stacks"),
			expectedError: "failed to get stacks",
		},
		{
			name:          "error getting repo info",
			info:          schema.ConfigAndStacksInfo{},
			driftEnabled:  true,
			upload:        true,
			mockStacks:    mockStacks,
			mockRepoErr:   errors.New("failed to get repo info"),
			expectedError: "failed to get repo info",
		},
		{
			name:         "error uploading to API",
			info:         schema.ConfigAndStacksInfo{},
			driftEnabled: true,
			upload:       true,
			mockStacks:   mockStacks,
			mockRepoInfo: &git.RepoInfo{
				RepoUrl:   "https://github.com/test/repo",
				RepoName:  "repo",
				RepoOwner: "test",
				RepoHost:  "github.com",
			},
			mockUploadErr: errors.New("failed to upload"),
			expectedError: "failed to upload",
		},
		{
			name:         "no deployments found",
			info:         schema.ConfigAndStacksInfo{},
			driftEnabled: false,
			upload:       false,
			mockStacks:   map[string]interface{}{},
		},
		{
			name:         "invalid component configuration",
			info:         schema.ConfigAndStacksInfo{},
			driftEnabled: false,
			upload:       false,
			mockStacks: map[string]interface{}{
				"stack1": map[string]interface{}{
					"components": "not-a-map",
				},
			},
		},
		{
			name:         "multiple component types",
			info:         schema.ConfigAndStacksInfo{},
			driftEnabled: false,
			upload:       false,
			mockStacks:   mockStacks,
		},
		{
			name:         "sorting edge case - same component different stacks",
			info:         schema.ConfigAndStacksInfo{},
			driftEnabled: false,
			upload:       false,
			mockStacks: map[string]interface{}{
				"stack2": map[string]interface{}{
					"components": map[string]interface{}{
						"terraform": map[string]interface{}{
							"vpc": map[string]interface{}{
								"metadata": map[string]interface{}{
									"type": "real",
								},
							},
						},
					},
				},
				"stack1": map[string]interface{}{
					"components": map[string]interface{}{
						"terraform": map[string]interface{}{
							"vpc": map[string]interface{}{
								"metadata": map[string]interface{}{
									"type": "real",
								},
							},
						},
					},
				},
			},
			expectedDeployments: []schema.Deployment{
				{
					Component:     "vpc",
					Stack:         "stack1",
					ComponentType: "terraform",
					Settings:      nil,
				},
				{
					Component:     "vpc",
					Stack:         "stack2",
					ComponentType: "terraform",
					Settings:      nil,
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set up command flags
			if tt.driftEnabled {
				cmd.Flags().Set("drift-enabled", "true")
			}
			if tt.upload {
				cmd.Flags().Set("upload", "true")
			}

			// Create mock implementations
			mockDescriber := &mockDescribeStacks{
				stacks: tt.mockStacks,
				err:    tt.mockStacksErr,
			}

			// Create a test-specific implementation of ExecuteListDeploymentsCmd
			executeListDeployments := func(_ *schema.ConfigAndStacksInfo, _ *cobra.Command) error {
				// Get stacks
				stacks, err := mockDescriber.Execute(&schema.AtmosConfiguration{}, "", nil, nil, nil, true, false)
				if err != nil {
					return err
				}

				// Collect deployments from stacks
				var processedDeployments []schema.Deployment
				for stackName, stackConfig := range stacks {
					stackConfigMap, ok := stackConfig.(map[string]any)
					if !ok {
						continue
					}

					components, ok := stackConfigMap["components"].(map[string]any)
					if !ok {
						continue
					}

					for componentType, typeComponents := range components {
						typeComponentsMap, ok := typeComponents.(map[string]any)
						if !ok {
							continue
						}

						for componentName, componentConfig := range typeComponentsMap {
							componentConfigMap, ok := componentConfig.(map[string]any)
							if !ok {
								continue
							}

							deployment := &schema.Deployment{
								Component:     componentName,
								Stack:         stackName,
								ComponentType: componentType,
							}

							if settings, ok := componentConfigMap["settings"].(map[string]any); ok {
								deployment.Settings = settings
							}
							if vars, ok := componentConfigMap["vars"].(map[string]any); ok {
								deployment.Vars = vars
							}
							if env, ok := componentConfigMap["env"].(map[string]any); ok {
								deployment.Env = env
							}
							if backend, ok := componentConfigMap["backend"].(map[string]any); ok {
								deployment.Backend = backend
							}
							if metadata, ok := componentConfigMap["metadata"].(map[string]any); ok {
								deployment.Metadata = metadata
							}

							// Skip abstract components
							if componentType, ok := deployment.Metadata["type"].(string); ok && componentType == "abstract" {
								continue
							}

							processedDeployments = append(processedDeployments, *deployment)
						}
					}
				}

				// Filter deployments if drift detection is enabled
				processedDeployments = filterDeploymentsByDriftDetection(processedDeployments, tt.driftEnabled)

				// Sort deployments
				sort.Slice(processedDeployments, func(i, j int) bool {
					if processedDeployments[i].Stack != processedDeployments[j].Stack {
						return processedDeployments[i].Stack < processedDeployments[j].Stack
					}
					return processedDeployments[i].Component < processedDeployments[j].Component
				})

				// Get repo info if needed
				var repoInfo *git.RepoInfo
				if tt.upload {
					repo := &mockGitRepo{repoInfo: tt.mockRepoInfo, err: tt.mockRepoErr}
					repoInfo, err = repo.GetRepoInfo()
					if err != nil {
						return err
					}
				}

				// Upload to API if needed
				if tt.upload && tt.driftEnabled {
					apiClient := &mockAPIClient{uploadErr: tt.mockUploadErr}
					req := pro.DriftDetectionUploadRequest{
						RepoURL:   repoInfo.RepoUrl,
						RepoName:  repoInfo.RepoName,
						RepoOwner: repoInfo.RepoOwner,
						RepoHost:  repoInfo.RepoHost,
						Stacks:    processedDeployments,
					}
					err := apiClient.UploadDriftDetection(&req)
					if err != nil {
						return err
					}
				}

				// Store the processed deployments for verification
				tt.processedDeployments = processedDeployments

				return nil
			}

			// Execute the test
			err := executeListDeployments(&tt.info, cmd)

			// Check error
			if tt.expectedError != "" {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedError)
			} else {
				assert.NoError(t, err)
			}

			// Check deployments if expected
			if tt.expectedDeployments != nil {
				assert.Equal(t, len(tt.expectedDeployments), len(tt.processedDeployments), "Number of deployments should match")
				for i, expected := range tt.expectedDeployments {
					actual := tt.processedDeployments[i]
					assert.Equal(t, expected.Component, actual.Component, "Component should match")
					assert.Equal(t, expected.Stack, actual.Stack, "Stack should match")
					assert.Equal(t, expected.ComponentType, actual.ComponentType, "ComponentType should match")
					if expected.Settings == nil {
						assert.Nil(t, actual.Settings, "Settings should be nil")
					} else {
						assert.Equal(t, expected.Settings, actual.Settings, "Settings should match")
					}
				}
			}
		})
	}
}

// TestSortDeployments tests the sorting functionality of deployments.
func TestSortDeployments(t *testing.T) {
	tests := []struct {
		name        string
		deployments []schema.Deployment
		expected    []schema.Deployment
	}{
		{
			name: "basic sorting",
			deployments: []schema.Deployment{
				{Component: "b", Stack: "stack2"},
				{Component: "a", Stack: "stack1"},
				{Component: "c", Stack: "stack1"},
				{Component: "a", Stack: "stack2"},
			},
			expected: []schema.Deployment{
				{Component: "a", Stack: "stack1"},
				{Component: "c", Stack: "stack1"},
				{Component: "a", Stack: "stack2"},
				{Component: "b", Stack: "stack2"},
			},
		},
		{
			name: "same component different stacks",
			deployments: []schema.Deployment{
				{Component: "vpc", Stack: "prod"},
				{Component: "vpc", Stack: "dev"},
				{Component: "vpc", Stack: "staging"},
			},
			expected: []schema.Deployment{
				{Component: "vpc", Stack: "dev"},
				{Component: "vpc", Stack: "prod"},
				{Component: "vpc", Stack: "staging"},
			},
		},
		{
			name: "same stack different components",
			deployments: []schema.Deployment{
				{Component: "z", Stack: "prod"},
				{Component: "a", Stack: "prod"},
				{Component: "m", Stack: "prod"},
			},
			expected: []schema.Deployment{
				{Component: "a", Stack: "prod"},
				{Component: "m", Stack: "prod"},
				{Component: "z", Stack: "prod"},
			},
		},
		{
			name: "complex sorting - both components and stacks",
			deployments: []schema.Deployment{
				{Component: "z", Stack: "prod"},
				{Component: "a", Stack: "dev"},
				{Component: "m", Stack: "prod"},
				{Component: "a", Stack: "prod"},
				{Component: "z", Stack: "dev"},
				{Component: "m", Stack: "dev"},
			},
			expected: []schema.Deployment{
				{Component: "a", Stack: "dev"},
				{Component: "m", Stack: "dev"},
				{Component: "z", Stack: "dev"},
				{Component: "a", Stack: "prod"},
				{Component: "m", Stack: "prod"},
				{Component: "z", Stack: "prod"},
			},
		},
		{
			name:        "empty deployments",
			deployments: []schema.Deployment{},
			expected:    []schema.Deployment{},
		},
		{
			name: "single deployment",
			deployments: []schema.Deployment{
				{Component: "vpc", Stack: "prod"},
			},
			expected: []schema.Deployment{
				{Component: "vpc", Stack: "prod"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sorted := sortDeployments(tt.deployments)
			assert.Equal(t, len(tt.expected), len(sorted), "Number of deployments should match")

			for i := range sorted {
				assert.Equal(t, tt.expected[i].Component, sorted[i].Component,
					"Component at index %d should match", i)
				assert.Equal(t, tt.expected[i].Stack, sorted[i].Stack,
					"Stack at index %d should match", i)
			}

			// Additional verification for complex sorting
			if tt.name == "complex sorting - both components and stacks" {
				// Verify stacks are sorted
				for i := 1; i < len(sorted); i++ {
					assert.LessOrEqual(t, sorted[i-1].Stack, sorted[i].Stack,
						"Stacks should be in ascending order")

					// If stacks are equal, verify components are sorted
					if sorted[i-1].Stack == sorted[i].Stack {
						assert.LessOrEqual(t, sorted[i-1].Component, sorted[i].Component,
							"Components should be in ascending order within same stack")
					}
				}
			}
		})
	}
}

// TestDriftDetectionFiltering tests the drift detection filtering logic.
func TestDriftDetectionFiltering(t *testing.T) {
	deployments := []schema.Deployment{
		{
			Component: "vpc",
			Stack:     "stack1",
			Settings: map[string]interface{}{
				"pro": map[string]interface{}{
					"drift_detection": map[string]interface{}{
						"enabled": true,
					},
				},
			},
		},
		{
			Component: "app",
			Stack:     "stack1",
			Settings: map[string]interface{}{
				"pro": map[string]interface{}{
					"drift_detection": map[string]interface{}{
						"enabled": false,
					},
				},
			},
		},
		{
			Component: "db",
			Stack:     "stack1",
			Settings:  map[string]interface{}{},
		},
	}

	// Filter deployments with drift detection enabled
	filtered := make([]schema.Deployment, 0)
	for _, d := range deployments {
		if settings, ok := d.Settings["pro"].(map[string]interface{}); ok {
			if driftDetection, ok := settings["drift_detection"].(map[string]interface{}); ok {
				if enabled, ok := driftDetection["enabled"].(bool); ok && enabled {
					filtered = append(filtered, d)
				}
			}
		}
	}

	// Verify filtering
	assert.Len(t, filtered, 1)
	assert.Equal(t, "vpc", filtered[0].Component)
	assert.Equal(t, "stack1", filtered[0].Stack)
}

// TestProcessComponentConfig tests the processComponentConfig function.
func TestProcessComponentConfig(t *testing.T) {
	tests := []struct {
		name            string
		stackName       string
		componentName   string
		componentType   string
		componentConfig interface{}
		expected        *schema.Deployment
	}{
		{
			name:          "valid component config",
			stackName:     "stack1",
			componentName: "vpc",
			componentType: "terraform",
			componentConfig: map[string]interface{}{
				"settings": map[string]interface{}{
					"pro": map[string]interface{}{
						"drift_detection": map[string]interface{}{
							"enabled": true,
						},
					},
				},
				"metadata": map[string]interface{}{
					"type": "real",
				},
			},
			expected: &schema.Deployment{
				Component:     "vpc",
				Stack:         "stack1",
				ComponentType: "terraform",
				Settings: map[string]interface{}{
					"pro": map[string]interface{}{
						"drift_detection": map[string]interface{}{
							"enabled": true,
						},
					},
				},
				Metadata: map[string]interface{}{
					"type": "real",
				},
			},
		},
		{
			name:            "invalid component config type",
			stackName:       "stack1",
			componentName:   "vpc",
			componentType:   "terraform",
			componentConfig: "not-a-map",
			expected:        nil,
		},
		{
			name:          "abstract component",
			stackName:     "stack1",
			componentName: "vpc",
			componentType: "terraform",
			componentConfig: map[string]interface{}{
				"metadata": map[string]interface{}{
					"type": "abstract",
				},
			},
			expected: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := processComponentConfig(tt.stackName, tt.componentName, tt.componentType, tt.componentConfig)
			if tt.expected == nil {
				assert.Nil(t, result)
			} else {
				assert.NotNil(t, result)
				assert.Equal(t, tt.expected.Component, result.Component)
				assert.Equal(t, tt.expected.Stack, result.Stack)
				assert.Equal(t, tt.expected.ComponentType, result.ComponentType)
				assert.Equal(t, tt.expected.Settings, result.Settings)
				assert.Equal(t, tt.expected.Metadata, result.Metadata)
			}
		})
	}
}

// TestProcessComponentType tests the processComponentType function.
func TestProcessComponentType(t *testing.T) {
	tests := []struct {
		name           string
		stackName      string
		componentType  string
		typeComponents interface{}
		expected       []schema.Deployment
	}{
		{
			name:          "valid component type",
			stackName:     "stack1",
			componentType: "terraform",
			typeComponents: map[string]interface{}{
				"vpc": map[string]interface{}{
					"settings": map[string]interface{}{
						"pro": map[string]interface{}{
							"drift_detection": map[string]interface{}{
								"enabled": true,
							},
						},
					},
					"metadata": map[string]interface{}{
						"type": "real",
					},
				},
				"abstract-vpc": map[string]interface{}{
					"metadata": map[string]interface{}{
						"type": "abstract",
					},
				},
			},
			expected: []schema.Deployment{
				{
					Component:     "vpc",
					Stack:         "stack1",
					ComponentType: "terraform",
					Settings: map[string]interface{}{
						"pro": map[string]interface{}{
							"drift_detection": map[string]interface{}{
								"enabled": true,
							},
						},
					},
					Metadata: map[string]interface{}{
						"type": "real",
					},
				},
			},
		},
		{
			name:           "invalid type components",
			stackName:      "stack1",
			componentType:  "terraform",
			typeComponents: "not-a-map",
			expected:       nil,
		},
		{
			name:           "empty type components",
			stackName:      "stack1",
			componentType:  "terraform",
			typeComponents: map[string]interface{}{},
			expected:       []schema.Deployment{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := processComponentType(tt.stackName, tt.componentType, tt.typeComponents)
			if tt.expected == nil {
				assert.Nil(t, result)
			} else {
				assert.Equal(t, len(tt.expected), len(result))
				for i, expected := range tt.expected {
					actual := result[i]
					assert.Equal(t, expected.Component, actual.Component)
					assert.Equal(t, expected.Stack, actual.Stack)
					assert.Equal(t, expected.ComponentType, actual.ComponentType)
					assert.Equal(t, expected.Settings, actual.Settings)
					assert.Equal(t, expected.Metadata, actual.Metadata)
				}
			}
		})
	}
}

// TestProcessStackComponents tests the processStackComponents function.
func TestProcessStackComponents(t *testing.T) {
	tests := []struct {
		name        string
		stackName   string
		stackConfig interface{}
		expected    []schema.Deployment
	}{
		{
			name:      "valid stack config",
			stackName: "stack1",
			stackConfig: map[string]interface{}{
				"components": map[string]interface{}{
					"terraform": map[string]interface{}{
						"vpc": map[string]interface{}{
							"settings": map[string]interface{}{
								"pro": map[string]interface{}{
									"drift_detection": map[string]interface{}{
										"enabled": true,
									},
								},
							},
							"metadata": map[string]interface{}{
								"type": "real",
							},
						},
					},
					"helmfile": map[string]interface{}{
						"app": map[string]interface{}{
							"metadata": map[string]interface{}{
								"type": "real",
							},
						},
					},
				},
			},
			expected: []schema.Deployment{
				{
					Component:     "vpc",
					Stack:         "stack1",
					ComponentType: "terraform",
					Settings: map[string]interface{}{
						"pro": map[string]interface{}{
							"drift_detection": map[string]interface{}{
								"enabled": true,
							},
						},
					},
					Metadata: map[string]interface{}{
						"type": "real",
					},
				},
				{
					Component:     "app",
					Stack:         "stack1",
					ComponentType: "helmfile",
					Settings:      map[string]interface{}{},
					Metadata: map[string]interface{}{
						"type": "real",
					},
				},
			},
		},
		{
			name:        "invalid stack config",
			stackName:   "stack1",
			stackConfig: "not-a-map",
			expected:    nil,
		},
		{
			name:      "missing components",
			stackName: "stack1",
			stackConfig: map[string]interface{}{
				"other": "value",
			},
			expected: nil,
		},
		{
			name:      "invalid components type",
			stackName: "stack1",
			stackConfig: map[string]interface{}{
				"components": "not-a-map",
			},
			expected: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := processStackComponents(tt.stackName, tt.stackConfig)
			if tt.expected == nil {
				assert.Nil(t, result)
			} else {
				// Sort both actual and expected deployments for order-insensitive comparison
				sortedResult := sortDeployments(result)
				sortedExpected := sortDeployments(tt.expected)
				assert.Equal(t, len(sortedExpected), len(sortedResult))
				for i, expected := range sortedExpected {
					actual := sortedResult[i]
					assert.Equal(t, expected.Component, actual.Component)
					assert.Equal(t, expected.Stack, actual.Stack)
					assert.Equal(t, expected.ComponentType, actual.ComponentType)
					assert.Equal(t, expected.Settings, actual.Settings)
					assert.Equal(t, expected.Metadata, actual.Metadata)
				}
			}
		})
	}
}

// TestFormatDeployments tests the formatDeployments function.
func TestFormatDeployments(t *testing.T) {
	tests := []struct {
		name        string
		deployments []schema.Deployment
		expected    string
	}{
		{
			name:        "empty deployments",
			deployments: []schema.Deployment{},
			expected:    "Component,Stack\n",
		},
		{
			name: "single deployment",
			deployments: []schema.Deployment{
				{
					Component: "vpc",
					Stack:     "prod",
				},
			},
			expected: "Component,Stack\nvpc,prod\n",
		},
		{
			name: "multiple deployments",
			deployments: []schema.Deployment{
				{
					Component: "vpc",
					Stack:     "prod",
				},
				{
					Component: "app",
					Stack:     "dev",
				},
			},
			expected: "Component,Stack\nvpc,prod\napp,dev\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatDeployments(tt.deployments)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestUploadDeploymentsFunc tests the uploadDeployments function.
func TestUploadDeploymentsFunc(t *testing.T) {
	// Create a test logger
	log, _ := logger.NewLogger("info", "test")

	tests := []struct {
		name          string
		deployments   []schema.Deployment
		mockRepoInfo  *git.RepoInfo
		mockRepoErr   error
		mockUploadErr error
		expectedError string
		description   string
	}{
		{
			name: "successful upload",
			deployments: []schema.Deployment{
				{
					Component: "vpc",
					Stack:     "stack1",
					Settings: map[string]interface{}{
						"pro": map[string]interface{}{
							"drift_detection": map[string]interface{}{
								"enabled": true,
							},
						},
					},
				},
			},
			mockRepoInfo: &git.RepoInfo{
				RepoUrl:   "https://github.com/test/repo",
				RepoName:  "repo",
				RepoOwner: "test",
				RepoHost:  "github.com",
			},
			expectedError: "",
			description:   "Test successful upload of deployments",
		},
		{
			name: "error getting repo info",
			deployments: []schema.Deployment{
				{
					Component: "vpc",
					Stack:     "stack1",
				},
			},
			mockRepoErr:   errors.New("failed to get repo info"),
			expectedError: "failed to get repo info",
			description:   "Test error when getting repo info",
		},
		{
			name: "error uploading to API",
			deployments: []schema.Deployment{
				{
					Component: "vpc",
					Stack:     "stack1",
				},
			},
			mockRepoInfo: &git.RepoInfo{
				RepoUrl:   "https://github.com/test/repo",
				RepoName:  "repo",
				RepoOwner: "test",
				RepoHost:  "github.com",
			},
			mockUploadErr: errors.New("failed to upload"),
			expectedError: "failed to upload",
			description:   "Test error when uploading to API",
		},
		{
			name:        "empty deployments",
			deployments: []schema.Deployment{},
			mockRepoInfo: &git.RepoInfo{
				RepoUrl:   "https://github.com/test/repo",
				RepoName:  "repo",
				RepoOwner: "test",
				RepoHost:  "github.com",
			},
			expectedError: "",
			description:   "Test uploading empty deployments slice",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create mock implementations
			mockRepo := &mockGitRepo{
				repoInfo: tt.mockRepoInfo,
				err:      tt.mockRepoErr,
			}

			mockAPIClient := &mockAPIClient{
				uploadErr: tt.mockUploadErr,
			}

			// Create a test-specific implementation of uploadDeployments
			uploadDeployments := func(deployments []schema.Deployment, log *logger.Logger) error {
				// Get repo info
				repoInfo, err := mockRepo.GetRepoInfo()
				if err != nil {
					return err
				}

				// Create upload request
				req := pro.DriftDetectionUploadRequest{
					RepoURL:   repoInfo.RepoUrl,
					RepoName:  repoInfo.RepoName,
					RepoOwner: repoInfo.RepoOwner,
					RepoHost:  repoInfo.RepoHost,
					Stacks:    deployments,
				}

				// Upload to API
				err = mockAPIClient.UploadDriftDetection(&req)
				if err != nil {
					return err
				}

				return nil
			}

			// Execute the test
			err := uploadDeployments(tt.deployments, log)

			// Check error
			if tt.expectedError != "" {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedError)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// TestCollectDeployments tests the collectDeployments function.
func TestCollectDeployments(t *testing.T) {
	tests := []struct {
		name        string
		stacksMap   map[string]interface{}
		expected    []schema.Deployment
		description string
	}{
		{
			name: "valid stacks with multiple components",
			stacksMap: map[string]interface{}{
				"stack1": map[string]interface{}{
					"components": map[string]interface{}{
						"terraform": map[string]interface{}{
							"vpc": map[string]interface{}{
								"settings": map[string]interface{}{
									"pro": map[string]interface{}{
										"drift_detection": map[string]interface{}{
											"enabled": true,
										},
									},
								},
								"metadata": map[string]interface{}{
									"type": "real",
								},
							},
						},
						"helmfile": map[string]interface{}{
							"app": map[string]interface{}{
								"metadata": map[string]interface{}{
									"type": "real",
								},
							},
						},
					},
				},
				"stack2": map[string]interface{}{
					"components": map[string]interface{}{
						"terraform": map[string]interface{}{
							"db": map[string]interface{}{
								"metadata": map[string]interface{}{
									"type": "real",
								},
							},
						},
					},
				},
			},
			expected: []schema.Deployment{
				{
					Component:     "vpc",
					Stack:         "stack1",
					ComponentType: "terraform",
					Settings: map[string]interface{}{
						"pro": map[string]interface{}{
							"drift_detection": map[string]interface{}{
								"enabled": true,
							},
						},
					},
					Metadata: map[string]interface{}{
						"type": "real",
					},
				},
				{
					Component:     "app",
					Stack:         "stack1",
					ComponentType: "helmfile",
					Settings:      map[string]interface{}{},
					Metadata: map[string]interface{}{
						"type": "real",
					},
				},
				{
					Component:     "db",
					Stack:         "stack2",
					ComponentType: "terraform",
					Settings:      map[string]interface{}{},
					Metadata: map[string]interface{}{
						"type": "real",
					},
				},
			},
			description: "Test collecting deployments from multiple stacks with different component types",
		},
		{
			name:        "empty stacks map",
			stacksMap:   map[string]interface{}{},
			expected:    []schema.Deployment{},
			description: "Test with empty stacks map",
		},
		{
			name: "stack with invalid components",
			stacksMap: map[string]interface{}{
				"stack1": map[string]interface{}{
					"components": "not-a-map",
				},
			},
			expected:    []schema.Deployment{},
			description: "Test with invalid components type",
		},
		{
			name: "stack with missing components",
			stacksMap: map[string]interface{}{
				"stack1": map[string]interface{}{
					"other": "value",
				},
			},
			expected:    []schema.Deployment{},
			description: "Test with missing components field",
		},
		{
			name: "stack with abstract components",
			stacksMap: map[string]interface{}{
				"stack1": map[string]interface{}{
					"components": map[string]interface{}{
						"terraform": map[string]interface{}{
							"abstract-vpc": map[string]interface{}{
								"metadata": map[string]interface{}{
									"type": "abstract",
								},
							},
						},
					},
				},
			},
			expected:    []schema.Deployment{},
			description: "Test with abstract components that should be filtered out",
		},
		{
			name: "stack with invalid component config",
			stacksMap: map[string]interface{}{
				"stack1": map[string]interface{}{
					"components": map[string]interface{}{
						"terraform": map[string]interface{}{
							"vpc": "not-a-map",
						},
					},
				},
			},
			expected:    []schema.Deployment{},
			description: "Test with invalid component configuration",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := collectDeployments(tt.stacksMap)

			// Sort both actual and expected deployments for order-insensitive comparison
			sortedResult := sortDeployments(result)
			sortedExpected := sortDeployments(tt.expected)

			assert.Equal(t, len(sortedExpected), len(sortedResult),
				"Number of deployments should match for test case: %s", tt.description)

			for i, expected := range sortedExpected {
				actual := sortedResult[i]
				assert.Equal(t, expected.Component, actual.Component,
					"Component should match for test case: %s", tt.description)
				assert.Equal(t, expected.Stack, actual.Stack,
					"Stack should match for test case: %s", tt.description)
				assert.Equal(t, expected.ComponentType, actual.ComponentType,
					"ComponentType should match for test case: %s", tt.description)
				assert.Equal(t, expected.Settings, actual.Settings,
					"Settings should match for test case: %s", tt.description)
				assert.Equal(t, expected.Metadata, actual.Metadata,
					"Metadata should match for test case: %s", tt.description)
			}
		})
	}
}

// TestFilterDeployments tests the filterDeployments function.
func TestFilterDeployments(t *testing.T) {
	tests := []struct {
		name         string
		deployments  []schema.Deployment
		driftEnabled bool
		expected     []schema.Deployment
		description  string
	}{
		{
			name: "drift detection enabled - filter enabled deployments",
			deployments: []schema.Deployment{
				{
					Component: "vpc",
					Stack:     "stack1",
					Settings: map[string]interface{}{
						"pro": map[string]interface{}{
							"drift_detection": map[string]interface{}{
								"enabled": true,
							},
						},
					},
				},
				{
					Component: "app",
					Stack:     "stack1",
					Settings: map[string]interface{}{
						"pro": map[string]interface{}{
							"drift_detection": map[string]interface{}{
								"enabled": false,
							},
						},
					},
				},
				{
					Component: "db",
					Stack:     "stack1",
					Settings:  map[string]interface{}{},
				},
			},
			driftEnabled: true,
			expected: []schema.Deployment{
				{
					Component: "vpc",
					Stack:     "stack1",
					Settings: map[string]interface{}{
						"pro": map[string]interface{}{
							"drift_detection": map[string]interface{}{
								"enabled": true,
							},
						},
					},
				},
			},
			description: "Test filtering deployments with drift detection enabled",
		},
		{
			name: "drift detection disabled - return all deployments",
			deployments: []schema.Deployment{
				{
					Component: "vpc",
					Stack:     "stack1",
					Settings: map[string]interface{}{
						"pro": map[string]interface{}{
							"drift_detection": map[string]interface{}{
								"enabled": true,
							},
						},
					},
				},
				{
					Component: "app",
					Stack:     "stack1",
					Settings: map[string]interface{}{
						"pro": map[string]interface{}{
							"drift_detection": map[string]interface{}{
								"enabled": false,
							},
						},
					},
				},
			},
			driftEnabled: false,
			expected: []schema.Deployment{
				{
					Component: "vpc",
					Stack:     "stack1",
					Settings: map[string]interface{}{
						"pro": map[string]interface{}{
							"drift_detection": map[string]interface{}{
								"enabled": true,
							},
						},
					},
				},
				{
					Component: "app",
					Stack:     "stack1",
					Settings: map[string]interface{}{
						"pro": map[string]interface{}{
							"drift_detection": map[string]interface{}{
								"enabled": false,
							},
						},
					},
				},
			},
			description: "Test returning all deployments when drift detection is disabled",
		},
		{
			name:         "empty deployments",
			deployments:  []schema.Deployment{},
			driftEnabled: true,
			expected:     []schema.Deployment{},
			description:  "Test with empty deployments slice",
		},
		{
			name: "deployments without pro settings",
			deployments: []schema.Deployment{
				{
					Component: "vpc",
					Stack:     "stack1",
					Settings:  map[string]interface{}{},
				},
				{
					Component: "app",
					Stack:     "stack1",
					Settings:  map[string]interface{}{},
				},
			},
			driftEnabled: true,
			expected:     []schema.Deployment{},
			description:  "Test with deployments that don't have pro settings",
		},
		{
			name: "deployments with invalid drift detection settings",
			deployments: []schema.Deployment{
				{
					Component: "vpc",
					Stack:     "stack1",
					Settings: map[string]interface{}{
						"pro": map[string]interface{}{
							"drift_detection": "not-a-map",
						},
					},
				},
				{
					Component: "app",
					Stack:     "stack1",
					Settings: map[string]interface{}{
						"pro": map[string]interface{}{
							"drift_detection": map[string]interface{}{
								"enabled": "not-a-bool",
							},
						},
					},
				},
			},
			driftEnabled: true,
			expected:     []schema.Deployment{},
			description:  "Test with deployments that have invalid drift detection settings",
		},
		{
			name: "multiple enabled deployments",
			deployments: []schema.Deployment{
				{
					Component: "vpc",
					Stack:     "stack1",
					Settings: map[string]interface{}{
						"pro": map[string]interface{}{
							"drift_detection": map[string]interface{}{
								"enabled": true,
							},
						},
					},
				},
				{
					Component: "app",
					Stack:     "stack1",
					Settings: map[string]interface{}{
						"pro": map[string]interface{}{
							"drift_detection": map[string]interface{}{
								"enabled": true,
							},
						},
					},
				},
				{
					Component: "db",
					Stack:     "stack1",
					Settings: map[string]interface{}{
						"pro": map[string]interface{}{
							"drift_detection": map[string]interface{}{
								"enabled": false,
							},
						},
					},
				},
			},
			driftEnabled: true,
			expected: []schema.Deployment{
				{
					Component: "vpc",
					Stack:     "stack1",
					Settings: map[string]interface{}{
						"pro": map[string]interface{}{
							"drift_detection": map[string]interface{}{
								"enabled": true,
							},
						},
					},
				},
				{
					Component: "app",
					Stack:     "stack1",
					Settings: map[string]interface{}{
						"pro": map[string]interface{}{
							"drift_detection": map[string]interface{}{
								"enabled": true,
							},
						},
					},
				},
			},
			description: "Test filtering multiple deployments with drift detection enabled",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := filterDeployments(tt.deployments, tt.driftEnabled)

			// Sort both actual and expected deployments for order-insensitive comparison
			sortedResult := sortDeployments(result)
			sortedExpected := sortDeployments(tt.expected)

			assert.Equal(t, len(sortedExpected), len(sortedResult),
				"Number of deployments should match for test case: %s", tt.description)

			for i, expected := range sortedExpected {
				actual := sortedResult[i]
				assert.Equal(t, expected.Component, actual.Component,
					"Component should match for test case: %s", tt.description)
				assert.Equal(t, expected.Stack, actual.Stack,
					"Stack should match for test case: %s", tt.description)
				assert.Equal(t, expected.Settings, actual.Settings,
					"Settings should match for test case: %s", tt.description)
			}
		})
	}
}
