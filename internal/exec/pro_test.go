package exec

import (
	"testing"

	atmosgit "github.com/cloudposse/atmos/pkg/git"
	"github.com/cloudposse/atmos/pkg/pro/dtos"
	"github.com/cloudposse/atmos/pkg/schema"
	gogit "github.com/go-git/go-git/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// MockProAPIClient is a mock implementation of the pro API client.
type MockProAPIClient struct {
	mock.Mock
}

func (m *MockProAPIClient) UploadDeploymentStatus(dto *dtos.DeploymentStatusUploadRequest) error {
	args := m.Called(dto)
	return args.Error(0)
}

// MockGitRepo is a mock implementation of the git repository.
type MockGitRepo struct {
	mock.Mock
}

func (m *MockGitRepo) GetLocalRepo() (*atmosgit.RepoInfo, error) {
	args := m.Called()
	return args.Get(0).(*atmosgit.RepoInfo), args.Error(1)
}

func (m *MockGitRepo) GetRepoInfo(repo *gogit.Repository) (atmosgit.RepoInfo, error) {
	args := m.Called(repo)
	return args.Get(0).(atmosgit.RepoInfo), args.Error(1)
}

// Test helper function to create a test info with pro settings.
func createTestInfo(proEnabled bool) schema.ConfigAndStacksInfo {
	info := schema.ConfigAndStacksInfo{
		Stack:            "test-stack",
		Component:        "test-component",
		ComponentType:    "terraform",
		ComponentFromArg: "test-component",
		SubCommand:       "plan",
	}

	if proEnabled {
		info.ComponentSettingsSection = map[string]interface{}{
			"pro": map[string]interface{}{
				"enabled": true,
			},
		}
	}

	return info
}

func TestShouldUploadDeploymentStatus(t *testing.T) {
	testCases := []struct {
		name     string
		info     *schema.ConfigAndStacksInfo
		expected bool
	}{
		{
			name: "should return true for plan command with pro enabled",
			info: &schema.ConfigAndStacksInfo{
				SubCommand: "plan",
				ComponentSettingsSection: map[string]interface{}{
					"pro": map[string]interface{}{
						"enabled": true,
					},
				},
			},
			expected: true,
		},
		{
			name: "should return false for plan command with pro disabled",
			info: &schema.ConfigAndStacksInfo{
				SubCommand: "plan",
				ComponentSettingsSection: map[string]interface{}{
					"pro": map[string]interface{}{
						"enabled": false,
					},
				},
			},
			expected: false,
		},
		{
			name: "should return false for plan command with no pro settings",
			info: &schema.ConfigAndStacksInfo{
				SubCommand:               "plan",
				ComponentSettingsSection: map[string]interface{}{},
			},
			expected: false,
		},
		{
			name: "should return false for non-plan command",
			info: &schema.ConfigAndStacksInfo{
				SubCommand: "apply",
				ComponentSettingsSection: map[string]interface{}{
					"pro": map[string]interface{}{
						"enabled": true,
					},
				},
			},
			expected: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := shouldUploadDeploymentStatus(tc.info)
			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestUploadDeploymentStatus(t *testing.T) {
	// Create mock clients
	mockProClient := new(MockProAPIClient)
	mockGitRepo := new(MockGitRepo)

	// Create test repo info
	testRepoInfo := &atmosgit.RepoInfo{
		RepoUrl:   "https://github.com/test/repo",
		RepoName:  "repo",
		RepoOwner: "test",
		RepoHost:  "github.com",
	}

	// Set up mock expectations for git functions
	mockGitRepo.On("GetLocalRepo").Return(testRepoInfo, nil)

	// Test cases
	testCases := []struct {
		name          string
		exitCode      int
		proEnabled    bool
		expectedError bool
		expectedDrift bool
	}{
		{
			name:          "drift detected",
			exitCode:      2,
			proEnabled:    true,
			expectedError: false,
			expectedDrift: true,
		},
		{
			name:          "no drift",
			exitCode:      0,
			proEnabled:    true,
			expectedError: false,
			expectedDrift: false,
		},
		{
			name:          "error exit code",
			exitCode:      1,
			proEnabled:    true,
			expectedError: false,
			expectedDrift: false,
		},
		{
			name:          "pro disabled",
			exitCode:      2,
			proEnabled:    false,
			expectedError: false,
			expectedDrift: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Create test info
			info := createTestInfo(tc.proEnabled)

			// Set up mock expectations for pro client
			if tc.proEnabled && (tc.exitCode == 0 || tc.exitCode == 2) {
				mockProClient.On("UploadDeploymentStatus", mock.AnythingOfType("*dtos.DeploymentStatusUploadRequest")).Return(nil)
			}

			// Call the function
			err := uploadDeploymentStatus(&info, tc.exitCode, mockProClient, mockGitRepo)

			// Check results
			if tc.expectedError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}

			// Verify mock expectations
			mockProClient.AssertExpectations(t)
			mockGitRepo.AssertExpectations(t)
		})
	}
}
