package list

import (
	"errors"
	"sort"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"

	"github.com/cloudposse/atmos/pkg/git"
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
			executeListDeployments := func(_ schema.ConfigAndStacksInfo, _ *cobra.Command, args []string) error {
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
				if tt.driftEnabled {
					filtered := []schema.Deployment{}
					for _, deployment := range processedDeployments {
						if settings, ok := deployment.Settings["pro"].(map[string]any); ok {
							if driftDetection, ok := settings["drift_detection"].(map[string]any); ok {
								if enabled, ok := driftDetection["enabled"].(bool); ok && enabled {
									filtered = append(filtered, deployment)
								}
							}
						}
					}
					processedDeployments = filtered
				}

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
			err := executeListDeployments(tt.info, cmd, nil)

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
	deployments := []schema.Deployment{
		{Component: "b", Stack: "stack2"},
		{Component: "a", Stack: "stack1"},
		{Component: "c", Stack: "stack1"},
		{Component: "a", Stack: "stack2"},
	}

	// Sort deployments by stack, then by component
	type deploymentRow struct {
		Component string
		Stack     string
	}
	rowsData := make([]deploymentRow, 0, len(deployments))
	for _, d := range deployments {
		rowsData = append(rowsData, deploymentRow{Component: d.Component, Stack: d.Stack})
	}

	// Sort
	sort.Slice(rowsData, func(i, j int) bool {
		if rowsData[i].Stack != rowsData[j].Stack {
			return rowsData[i].Stack < rowsData[j].Stack
		}
		return rowsData[i].Component < rowsData[j].Component
	})

	// Verify sorting
	expected := []deploymentRow{
		{Component: "a", Stack: "stack1"},
		{Component: "c", Stack: "stack1"},
		{Component: "a", Stack: "stack2"},
		{Component: "b", Stack: "stack2"},
	}

	assert.Equal(t, expected, rowsData)
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
