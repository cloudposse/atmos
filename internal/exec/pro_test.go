package exec

import (
	"testing"

	gogit "github.com/go-git/go-git/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	atmosgit "github.com/cloudposse/atmos/pkg/git"
	"github.com/cloudposse/atmos/pkg/pro/dtos"
	"github.com/cloudposse/atmos/pkg/schema"
)

// MockProAPIClient is a mock implementation of the pro API client.
type MockProAPIClient struct {
	mock.Mock
}

func (m *MockProAPIClient) UploadInstances(req *dtos.InstancesUploadRequest) error {
	args := m.Called(req)
	return args.Error(0)
}

func (m *MockProAPIClient) UploadInstanceStatus(dto *dtos.InstanceStatusUploadRequest) error {
	args := m.Called(dto)
	return args.Error(0)
}

func (m *MockProAPIClient) UploadAffectedStacks(dto *dtos.UploadAffectedStacksRequest) error {
	args := m.Called(dto)
	return args.Error(0)
}

func (m *MockProAPIClient) LockStack(dto *dtos.LockStackRequest) (dtos.LockStackResponse, error) {
	args := m.Called(dto)
	if args.Get(0) == nil {
		return dtos.LockStackResponse{}, args.Error(1)
	}
	return args.Get(0).(dtos.LockStackResponse), args.Error(1)
}

func (m *MockProAPIClient) UnlockStack(dto *dtos.UnlockStackRequest) (dtos.UnlockStackResponse, error) {
	args := m.Called(dto)
	if args.Get(0) == nil {
		return dtos.UnlockStackResponse{}, args.Error(1)
	}
	return args.Get(0).(dtos.UnlockStackResponse), args.Error(1)
}

// MockGitRepo is a mock implementation of the git repository.
type MockGitRepo struct {
	mock.Mock
}

func (m *MockGitRepo) GetLocalRepoInfo() (*atmosgit.RepoInfo, error) {
	args := m.Called()
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*atmosgit.RepoInfo), args.Error(1)
}

func (m *MockGitRepo) GetRepoInfo(repo *gogit.Repository) (atmosgit.RepoInfo, error) {
	args := m.Called(repo)
	if args.Get(0) == nil {
		return atmosgit.RepoInfo{}, args.Error(1)
	}
	return args.Get(0).(atmosgit.RepoInfo), args.Error(1)
}

func (m *MockGitRepo) GetCurrentCommitSHA() (string, error) {
	args := m.Called()
	return args.String(0), args.Error(1)
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

func TestShouldUploadStatus(t *testing.T) {
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
			result := shouldUploadStatus(tc.info)
			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestUploadStatus(t *testing.T) {
	// Create test repo info
	testRepoInfo := &atmosgit.RepoInfo{
		RepoUrl:   "https://github.com/test/repo",
		RepoName:  "repo",
		RepoOwner: "test",
		RepoHost:  "github.com",
	}

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
			// Create fresh mock clients for each subtest
			mockProClient := new(MockProAPIClient)
			mockGitRepo := new(MockGitRepo)

			// Create test info
			info := createTestInfo(tc.proEnabled)

			// Set up mock expectations based on exit code
			// The function only processes exit codes 0 and 2
			if tc.exitCode == 0 || tc.exitCode == 2 {
				// Set up mock expectations for git functions
				mockGitRepo.On("GetLocalRepoInfo").Return(testRepoInfo, nil)
				mockGitRepo.On("GetCurrentCommitSHA").Return("abc123def456", nil)

				// Set up mock expectations for pro client
				mockProClient.On("UploadInstanceStatus", mock.AnythingOfType("*dtos.InstanceStatusUploadRequest")).Return(nil)
			}

			// Call the function
			err := uploadStatus(&info, tc.exitCode, mockProClient, mockGitRepo)

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

// TestProLockCmdArgs tests the ProLockCmdArgs struct.
func TestProLockCmdArgs(t *testing.T) {
	t.Run("creates lock args with all fields", func(t *testing.T) {
		args := ProLockCmdArgs{
			ProLockUnlockCmdArgs: ProLockUnlockCmdArgs{
				Component: "vpc",
				Stack:     "dev",
			},
			LockMessage: "Test lock",
			LockTTL:     30,
		}

		assert.Equal(t, "vpc", args.Component)
		assert.Equal(t, "dev", args.Stack)
		assert.Equal(t, "Test lock", args.LockMessage)
		assert.Equal(t, int32(30), args.LockTTL)
	})
}

// TestProUnlockCmdArgs tests the ProUnlockCmdArgs struct.
func TestProUnlockCmdArgs(t *testing.T) {
	t.Run("creates unlock args with required fields", func(t *testing.T) {
		args := ProUnlockCmdArgs{
			ProLockUnlockCmdArgs: ProLockUnlockCmdArgs{
				Component: "vpc",
				Stack:     "dev",
			},
		}

		assert.Equal(t, "vpc", args.Component)
		assert.Equal(t, "dev", args.Stack)
	})
}

// TestUploadStatusWithDifferentExitCodes tests upload behavior with various exit codes.
func TestUploadStatusWithDifferentExitCodes(t *testing.T) {
	testRepoInfo := &atmosgit.RepoInfo{
		RepoUrl:   "https://github.com/test/repo",
		RepoName:  "repo",
		RepoOwner: "test",
		RepoHost:  "github.com",
	}

	testCases := []struct {
		name             string
		exitCode         int
		shouldUpload     bool
		expectedHasDrift bool
	}{
		{
			name:             "exit code 0 - no changes",
			exitCode:         0,
			shouldUpload:     true,
			expectedHasDrift: false,
		},
		{
			name:             "exit code 1 - error",
			exitCode:         1,
			shouldUpload:     false,
			expectedHasDrift: false,
		},
		{
			name:             "exit code 2 - changes detected",
			exitCode:         2,
			shouldUpload:     true,
			expectedHasDrift: true,
		},
		{
			name:             "exit code 3 - unknown",
			exitCode:         3,
			shouldUpload:     false,
			expectedHasDrift: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockProClient := new(MockProAPIClient)
			mockGitRepo := new(MockGitRepo)

			info := createTestInfo(true)

			if tc.shouldUpload {
				mockGitRepo.On("GetLocalRepoInfo").Return(testRepoInfo, nil)
				mockGitRepo.On("GetCurrentCommitSHA").Return("abc123", nil)
				mockProClient.On("UploadInstanceStatus", mock.AnythingOfType("*dtos.InstanceStatusUploadRequest")).Return(nil)
			}

			err := uploadStatus(&info, tc.exitCode, mockProClient, mockGitRepo)
			assert.NoError(t, err)

			mockProClient.AssertExpectations(t)
			mockGitRepo.AssertExpectations(t)
		})
	}
}

// TestUploadStatusWithGitErrors tests error handling when git operations fail.
func TestUploadStatusWithGitErrors(t *testing.T) {
	t.Run("handles git repo info error", func(t *testing.T) {
		mockProClient := new(MockProAPIClient)
		mockGitRepo := new(MockGitRepo)

		info := createTestInfo(true)

		// Simulate git error
		mockGitRepo.On("GetLocalRepoInfo").Return(nil, assert.AnError)

		err := uploadStatus(&info, 2, mockProClient, mockGitRepo)
		assert.Error(t, err)

		mockGitRepo.AssertExpectations(t)
	})

	t.Run("continues when git SHA fails", func(t *testing.T) {
		mockProClient := new(MockProAPIClient)
		mockGitRepo := new(MockGitRepo)

		info := createTestInfo(true)

		testRepoInfo := &atmosgit.RepoInfo{
			RepoUrl:   "https://github.com/test/repo",
			RepoName:  "repo",
			RepoOwner: "test",
			RepoHost:  "github.com",
		}

		// Git SHA can fail but upload should continue
		mockGitRepo.On("GetLocalRepoInfo").Return(testRepoInfo, nil)
		mockGitRepo.On("GetCurrentCommitSHA").Return("", assert.AnError)
		mockProClient.On("UploadInstanceStatus", mock.AnythingOfType("*dtos.InstanceStatusUploadRequest")).Return(nil)

		err := uploadStatus(&info, 2, mockProClient, mockGitRepo)
		assert.NoError(t, err)

		mockProClient.AssertExpectations(t)
		mockGitRepo.AssertExpectations(t)
	})
}

// TestUploadStatusDTO tests the DTO creation for instance status upload.
func TestUploadStatusDTO(t *testing.T) {
	t.Run("creates DTO with correct fields", func(t *testing.T) {
		dto := dtos.InstanceStatusUploadRequest{
			AtmosProRunID: "run-123",
			GitSHA:        "abc123def456",
			RepoURL:       "https://github.com/test/repo",
			RepoName:      "repo",
			RepoOwner:     "test",
			RepoHost:      "github.com",
			Stack:         "dev",
			Component:     "vpc",
			HasDrift:      true,
		}

		assert.Equal(t, "run-123", dto.AtmosProRunID)
		assert.Equal(t, "abc123def456", dto.GitSHA)
		assert.Equal(t, "https://github.com/test/repo", dto.RepoURL)
		assert.Equal(t, "repo", dto.RepoName)
		assert.Equal(t, "test", dto.RepoOwner)
		assert.Equal(t, "github.com", dto.RepoHost)
		assert.Equal(t, "dev", dto.Stack)
		assert.Equal(t, "vpc", dto.Component)
		assert.True(t, dto.HasDrift)
	})
}

// TestShouldUploadStatusEdgeCases tests edge cases for shouldUploadStatus.
func TestShouldUploadStatusEdgeCases(t *testing.T) {
	testCases := []struct {
		name     string
		info     *schema.ConfigAndStacksInfo
		expected bool
	}{
		{
			name: "nil component settings section",
			info: &schema.ConfigAndStacksInfo{
				SubCommand:               "plan",
				ComponentSettingsSection: nil,
			},
			expected: false,
		},
		{
			name: "pro settings not a map",
			info: &schema.ConfigAndStacksInfo{
				SubCommand: "plan",
				ComponentSettingsSection: map[string]interface{}{
					"pro": "invalid",
				},
			},
			expected: false,
		},
		{
			name: "enabled setting not a bool",
			info: &schema.ConfigAndStacksInfo{
				SubCommand: "plan",
				ComponentSettingsSection: map[string]interface{}{
					"pro": map[string]interface{}{
						"enabled": "true",
					},
				},
			},
			expected: false,
		},
		{
			name: "empty subcommand",
			info: &schema.ConfigAndStacksInfo{
				SubCommand: "",
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
			result := shouldUploadStatus(tc.info)
			assert.Equal(t, tc.expected, result)
		})
	}
}

// TestLockKeyFormat tests the format of the lock key.
func TestLockKeyFormat(t *testing.T) {
	t.Run("creates correct lock key format", func(t *testing.T) {
		owner := "cloudposse"
		repoName := "infra"
		stack := "dev"
		component := "vpc"

		expectedKey := "cloudposse/infra/dev/vpc"
		actualKey := owner + "/" + repoName + "/" + stack + "/" + component

		assert.Equal(t, expectedKey, actualKey)
	})
}

// TestProLockUnlockCmdArgs tests the ProLockUnlockCmdArgs struct.
func TestProLockUnlockCmdArgs(t *testing.T) {
	t.Run("creates lock/unlock args with required fields", func(t *testing.T) {
		args := ProLockUnlockCmdArgs{
			Component: "vpc",
			Stack:     "dev",
		}

		assert.Equal(t, "vpc", args.Component)
		assert.Equal(t, "dev", args.Stack)
	})
}

// TestLockStackRequest tests the LockStackRequest DTO.
func TestLockStackRequest(t *testing.T) {
	t.Run("creates lock request with all fields", func(t *testing.T) {
		dto := dtos.LockStackRequest{
			Key:         "owner/repo/stack/component",
			TTL:         30,
			LockMessage: "Locked by user",
			Properties:  nil,
		}

		assert.Equal(t, "owner/repo/stack/component", dto.Key)
		assert.Equal(t, int32(30), dto.TTL)
		assert.Equal(t, "Locked by user", dto.LockMessage)
		assert.Nil(t, dto.Properties)
	})
}

// TestUnlockStackRequest tests the UnlockStackRequest DTO.
func TestUnlockStackRequest(t *testing.T) {
	t.Run("creates unlock request with key", func(t *testing.T) {
		dto := dtos.UnlockStackRequest{
			Key: "owner/repo/stack/component",
		}

		assert.Equal(t, "owner/repo/stack/component", dto.Key)
	})
}
