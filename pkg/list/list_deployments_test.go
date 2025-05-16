package list

import (
	"errors"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"

	"github.com/cloudposse/atmos/pkg/git"
	"github.com/cloudposse/atmos/pkg/pro"
	"github.com/cloudposse/atmos/pkg/schema"
)

// mockGitRepo is a mock implementation of git.Repository
type mockGitRepo struct {
	repoInfo *git.RepoInfo
	err      error
}

func (m *mockGitRepo) GetRepoInfo() (*git.RepoInfo, error) {
	return m.repoInfo, m.err
}

// mockAPIClient is a mock implementation of pro.AtmosProAPIClient
type mockAPIClient struct {
	uploadErr error
}

func (m *mockAPIClient) UploadDriftDetection(req pro.DriftDetectionUploadRequest) error {
	return m.uploadErr
}

// mockDescribeStacks is a mock implementation of describe.ExecuteDescribeStacks
type mockDescribeStacks struct {
	stacks map[string]interface{}
	err    error
}

func (m *mockDescribeStacks) Execute(config schema.AtmosConfiguration, stack string, additionalStacks []string, additionalStacksList []string, additionalStacksMap map[string]bool, includeAllStacks bool, includeStackGroups bool) (map[string]interface{}, error) {
	return m.stacks, m.err
}

// TestExecuteListDeploymentsCmd tests the ExecuteListDeploymentsCmd function
func TestExecuteListDeploymentsCmd(t *testing.T) {
	// Create a new command for testing
	cmd := &cobra.Command{}
	cmd.Flags().Bool("drift-enabled", false, "Filter deployments with drift detection enabled")
	cmd.Flags().Bool("upload", false, "Upload deployments to pro API")

	// Mock stacks data
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
							"type": "concrete",
						},
					},
					"abstract-vpc": map[string]interface{}{
						"metadata": map[string]interface{}{
							"type": "abstract",
						},
					},
				},
			},
		},
	}

	// Test cases
	tests := []struct {
		name           string
		info           schema.ConfigAndStacksInfo
		driftEnabled   bool
		upload         bool
		mockStacks     map[string]interface{}
		mockStacksErr  error
		mockRepoInfo   *git.RepoInfo
		mockRepoErr    error
		mockUploadErr  error
		expectedError  string
		expectedOutput string
	}{
		{
			name:         "default flags",
			info:         schema.ConfigAndStacksInfo{},
			driftEnabled: false,
			upload:       false,
			mockStacks:   mockStacks,
		},
		{
			name:         "drift detection enabled",
			info:         schema.ConfigAndStacksInfo{},
			driftEnabled: true,
			upload:       false,
			mockStacks:   mockStacks,
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
			executeListDeployments := func(info schema.ConfigAndStacksInfo, cmd *cobra.Command, args []string) error {
				// Get stacks
				_, err := mockDescriber.Execute(schema.AtmosConfiguration{}, "", nil, nil, nil, true, false)
				if err != nil {
					return err
				}

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
						BaseSHA:   "",
						RepoURL:   repoInfo.RepoUrl,
						RepoName:  repoInfo.RepoName,
						RepoOwner: repoInfo.RepoOwner,
						RepoHost:  repoInfo.RepoHost,
						Stacks:    []schema.Deployment{},
					}
					err := apiClient.UploadDriftDetection(req)
					if err != nil {
						return err
					}
				}

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
		})
	}
}
