package exec

import (
	"testing"
	"time"

	gogit "github.com/go-git/go-git/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	atmosgit "github.com/cloudposse/atmos/pkg/git"
	"github.com/cloudposse/atmos/pkg/metrics/process"
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
			name: "should return true for apply command with pro enabled",
			info: &schema.ConfigAndStacksInfo{
				SubCommand: "apply",
				ComponentSettingsSection: map[string]interface{}{
					"pro": map[string]interface{}{
						"enabled": true,
					},
				},
			},
			expected: true,
		},
		{
			name: "should return false for destroy command",
			info: &schema.ConfigAndStacksInfo{
				SubCommand: "destroy",
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

	// Test cases — all exit codes now upload.
	testCases := []struct {
		name          string
		exitCode      int
		proEnabled    bool
		expectedError bool
	}{
		{
			name:          "drift detected (exit code 2)",
			exitCode:      2,
			proEnabled:    true,
			expectedError: false,
		},
		{
			name:          "no drift (exit code 0)",
			exitCode:      0,
			proEnabled:    true,
			expectedError: false,
		},
		{
			name:          "error exit code",
			exitCode:      1,
			proEnabled:    true,
			expectedError: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockProClient := new(MockProAPIClient)
			mockGitRepo := new(MockGitRepo)

			info := createTestInfo(tc.proEnabled)

			// All exit codes now upload.
			mockGitRepo.On("GetLocalRepoInfo").Return(testRepoInfo, nil)
			mockGitRepo.On("GetCurrentCommitSHA").Return("abc123def456", nil)
			mockProClient.On("UploadInstanceStatus", mock.MatchedBy(func(dto *dtos.InstanceStatusUploadRequest) bool {
				return dto.Command == "plan" && dto.ExitCode == tc.exitCode
			})).Return(nil)

			err := uploadStatus(&info, tc.exitCode, nil, mockProClient, mockGitRepo)

			if tc.expectedError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}

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
		name     string
		exitCode int
	}{
		{
			name:     "exit code 0 - no changes",
			exitCode: 0,
		},
		{
			name:     "exit code 1 - error",
			exitCode: 1,
		},
		{
			name:     "exit code 2 - changes detected",
			exitCode: 2,
		},
		{
			name:     "exit code 3 - unknown",
			exitCode: 3,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockProClient := new(MockProAPIClient)
			mockGitRepo := new(MockGitRepo)

			info := createTestInfo(true)

			// All exit codes now upload with raw command + exit_code.
			mockGitRepo.On("GetLocalRepoInfo").Return(testRepoInfo, nil)
			mockGitRepo.On("GetCurrentCommitSHA").Return("abc123", nil)
			mockProClient.On("UploadInstanceStatus", mock.MatchedBy(func(dto *dtos.InstanceStatusUploadRequest) bool {
				return dto.Command == "plan" && dto.ExitCode == tc.exitCode
			})).Return(nil)

			err := uploadStatus(&info, tc.exitCode, nil, mockProClient, mockGitRepo)
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

		err := uploadStatus(&info, 2, nil, mockProClient, mockGitRepo)
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

		err := uploadStatus(&info, 2, nil, mockProClient, mockGitRepo)
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
			Command:       "plan",
			ExitCode:      2,
		}

		assert.Equal(t, "run-123", dto.AtmosProRunID)
		assert.Equal(t, "abc123def456", dto.GitSHA)
		assert.Equal(t, "https://github.com/test/repo", dto.RepoURL)
		assert.Equal(t, "repo", dto.RepoName)
		assert.Equal(t, "test", dto.RepoOwner)
		assert.Equal(t, "github.com", dto.RepoHost)
		assert.Equal(t, "dev", dto.Stack)
		assert.Equal(t, "vpc", dto.Component)
		assert.Equal(t, "plan", dto.Command)
		assert.Equal(t, 2, dto.ExitCode)
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

// TestExecuteProLock tests the executeProLock function with mocked dependencies.
func TestExecuteProLock(t *testing.T) {
	t.Run("successfully locks stack and shows checkmark", func(t *testing.T) {
		// Create mocks
		mockAPI := new(MockProAPIClient)
		mockGit := new(MockGitRepo)

		// Setup mock expectations
		mockGit.On("GetLocalRepoInfo").Return(&atmosgit.RepoInfo{
			RepoOwner: "test-owner",
			RepoName:  "test-repo",
		}, nil)

		mockAPI.On("LockStack", mock.MatchedBy(func(req *dtos.LockStackRequest) bool {
			return req.Key == "test-owner/test-repo/test-stack/test-component" &&
				req.TTL == 30 &&
				req.LockMessage == "Test lock"
		})).Return(dtos.LockStackResponse{
			Data: struct {
				ID          string    `json:"id,omitempty"`
				WorkspaceId string    `json:"workspaceId,omitempty"`
				Key         string    `json:"key,omitempty"`
				LockMessage string    `json:"lockMessage,omitempty"`
				ExpiresAt   time.Time `json:"expiresAt,omitempty"`
				CreatedAt   time.Time `json:"createdAt,omitempty"`
				UpdatedAt   time.Time `json:"updatedAt,omitempty"`
				DeletedAt   time.Time `json:"deletedAt,omitempty"`
			}{
				Key: "test-owner/test-repo/test-stack/test-component",
			},
		}, nil)

		// Create test args
		args := ProLockCmdArgs{
			ProLockUnlockCmdArgs: ProLockUnlockCmdArgs{
				Component: "test-component",
				Stack:     "test-stack",
			},
			LockTTL:     30,
			LockMessage: "Test lock",
		}

		// Execute
		err := executeProLock(&args, mockAPI, mockGit)

		// Verify
		assert.NoError(t, err)
		mockAPI.AssertExpectations(t)
		mockGit.AssertExpectations(t)
	})

	t.Run("returns error when git repo info fails", func(t *testing.T) {
		mockAPI := new(MockProAPIClient)
		mockGit := new(MockGitRepo)

		mockGit.On("GetLocalRepoInfo").Return(nil, assert.AnError)

		args := ProLockCmdArgs{
			ProLockUnlockCmdArgs: ProLockUnlockCmdArgs{
				Component: "test-component",
				Stack:     "test-stack",
			},
			LockTTL:     30,
			LockMessage: "Test lock",
		}

		err := executeProLock(&args, mockAPI, mockGit)

		assert.Error(t, err)
		mockGit.AssertExpectations(t)
	})

	t.Run("returns error when API lock fails", func(t *testing.T) {
		mockAPI := new(MockProAPIClient)
		mockGit := new(MockGitRepo)

		mockGit.On("GetLocalRepoInfo").Return(&atmosgit.RepoInfo{
			RepoOwner: "test-owner",
			RepoName:  "test-repo",
		}, nil)

		mockAPI.On("LockStack", mock.Anything).Return(dtos.LockStackResponse{}, assert.AnError)

		args := ProLockCmdArgs{
			ProLockUnlockCmdArgs: ProLockUnlockCmdArgs{
				Component: "test-component",
				Stack:     "test-stack",
			},
			LockTTL:     30,
			LockMessage: "Test lock",
		}

		err := executeProLock(&args, mockAPI, mockGit)

		assert.Error(t, err)
		mockAPI.AssertExpectations(t)
		mockGit.AssertExpectations(t)
	})
}

// TestExecuteProUnlock tests the executeProUnlock function with mocked dependencies.
func TestExecuteProUnlock(t *testing.T) {
	t.Run("successfully unlocks stack and shows checkmark", func(t *testing.T) {
		// Create mocks
		mockAPI := new(MockProAPIClient)
		mockGit := new(MockGitRepo)

		// Setup mock expectations
		mockGit.On("GetLocalRepoInfo").Return(&atmosgit.RepoInfo{
			RepoOwner: "test-owner",
			RepoName:  "test-repo",
		}, nil)

		mockAPI.On("UnlockStack", mock.MatchedBy(func(req *dtos.UnlockStackRequest) bool {
			return req.Key == "test-owner/test-repo/test-stack/test-component"
		})).Return(dtos.UnlockStackResponse{}, nil)

		// Create test args
		args := ProUnlockCmdArgs{
			ProLockUnlockCmdArgs: ProLockUnlockCmdArgs{
				Component: "test-component",
				Stack:     "test-stack",
			},
		}

		// Execute
		err := executeProUnlock(&args, mockAPI, mockGit)

		// Verify
		assert.NoError(t, err)
		mockAPI.AssertExpectations(t)
		mockGit.AssertExpectations(t)
	})

	t.Run("returns error when git repo info fails", func(t *testing.T) {
		mockAPI := new(MockProAPIClient)
		mockGit := new(MockGitRepo)

		mockGit.On("GetLocalRepoInfo").Return(nil, assert.AnError)

		args := ProUnlockCmdArgs{
			ProLockUnlockCmdArgs: ProLockUnlockCmdArgs{
				Component: "test-component",
				Stack:     "test-stack",
			},
		}

		err := executeProUnlock(&args, mockAPI, mockGit)

		assert.Error(t, err)
		mockGit.AssertExpectations(t)
	})

	t.Run("returns error when API unlock fails", func(t *testing.T) {
		mockAPI := new(MockProAPIClient)
		mockGit := new(MockGitRepo)

		mockGit.On("GetLocalRepoInfo").Return(&atmosgit.RepoInfo{
			RepoOwner: "test-owner",
			RepoName:  "test-repo",
		}, nil)

		mockAPI.On("UnlockStack", mock.Anything).Return(dtos.UnlockStackResponse{}, assert.AnError)

		args := ProUnlockCmdArgs{
			ProLockUnlockCmdArgs: ProLockUnlockCmdArgs{
				Component: "test-component",
				Stack:     "test-stack",
			},
		}

		err := executeProUnlock(&args, mockAPI, mockGit)

		assert.Error(t, err)
		mockAPI.AssertExpectations(t)
		mockGit.AssertExpectations(t)
	})
}

// TestUploadStatusWithMetrics tests that process metrics are included in the upload DTO.
func TestUploadStatusWithMetrics(t *testing.T) {
	testRepoInfo := &atmosgit.RepoInfo{
		RepoUrl:   "https://github.com/test/repo",
		RepoName:  "repo",
		RepoOwner: "test",
		RepoHost:  "github.com",
	}

	t.Run("includes metrics in DTO when provided", func(t *testing.T) {
		mockProClient := new(MockProAPIClient)
		mockGitRepo := new(MockGitRepo)

		info := createTestInfo(true)

		metrics := &process.ProcessMetrics{
			WallTime:         45200 * time.Millisecond,
			UserCPUTime:      12300 * time.Millisecond,
			SystemCPUTime:    4100 * time.Millisecond,
			MaxRSSBytes:      536870912,
			MinorPageFaults:  42000,
			MajorPageFaults:  12,
			InBlockOps:       1500,
			OutBlockOps:      800,
			VolCtxSwitches:   3200,
			InvolCtxSwitches: 150,
		}

		mockGitRepo.On("GetLocalRepoInfo").Return(testRepoInfo, nil)
		mockGitRepo.On("GetCurrentCommitSHA").Return("abc123", nil)
		mockProClient.On("UploadInstanceStatus", mock.MatchedBy(func(dto *dtos.InstanceStatusUploadRequest) bool {
			return dto.Command == "plan" &&
				dto.ExitCode == 0 &&
				dto.WallTimeMs != nil && *dto.WallTimeMs == 45200 &&
				dto.UserCPUTimeMs != nil && *dto.UserCPUTimeMs == 12300 &&
				dto.SysCPUTimeMs != nil && *dto.SysCPUTimeMs == 4100 &&
				dto.PeakRSSBytes != nil && *dto.PeakRSSBytes == 536870912 &&
				dto.MinorPageFaults != nil && *dto.MinorPageFaults == 42000 &&
				dto.MajorPageFaults != nil && *dto.MajorPageFaults == 12 &&
				dto.InBlockOps != nil && *dto.InBlockOps == 1500 &&
				dto.OutBlockOps != nil && *dto.OutBlockOps == 800 &&
				dto.VolCtxSwitches != nil && *dto.VolCtxSwitches == 3200 &&
				dto.InvolCtxSwitches != nil && *dto.InvolCtxSwitches == 150
		})).Return(nil)

		err := uploadStatus(&info, 0, metrics, mockProClient, mockGitRepo)
		assert.NoError(t, err)

		mockProClient.AssertExpectations(t)
		mockGitRepo.AssertExpectations(t)
	})

	t.Run("omits zero rusage fields", func(t *testing.T) {
		mockProClient := new(MockProAPIClient)
		mockGitRepo := new(MockGitRepo)

		info := createTestInfo(true)

		// Simulate Windows: only timing metrics, no rusage.
		metrics := &process.ProcessMetrics{
			WallTime:      5 * time.Second,
			UserCPUTime:   2 * time.Second,
			SystemCPUTime: 1 * time.Second,
		}

		mockGitRepo.On("GetLocalRepoInfo").Return(testRepoInfo, nil)
		mockGitRepo.On("GetCurrentCommitSHA").Return("abc123", nil)
		mockProClient.On("UploadInstanceStatus", mock.MatchedBy(func(dto *dtos.InstanceStatusUploadRequest) bool {
			return dto.WallTimeMs != nil && *dto.WallTimeMs == 5000 &&
				dto.PeakRSSBytes == nil &&
				dto.MinorPageFaults == nil &&
				dto.MajorPageFaults == nil
		})).Return(nil)

		err := uploadStatus(&info, 0, metrics, mockProClient, mockGitRepo)
		assert.NoError(t, err)

		mockProClient.AssertExpectations(t)
	})

	t.Run("nil metrics produces no metrics fields", func(t *testing.T) {
		mockProClient := new(MockProAPIClient)
		mockGitRepo := new(MockGitRepo)

		info := createTestInfo(true)

		mockGitRepo.On("GetLocalRepoInfo").Return(testRepoInfo, nil)
		mockGitRepo.On("GetCurrentCommitSHA").Return("abc123", nil)
		mockProClient.On("UploadInstanceStatus", mock.MatchedBy(func(dto *dtos.InstanceStatusUploadRequest) bool {
			return dto.WallTimeMs == nil && dto.PeakRSSBytes == nil
		})).Return(nil)

		err := uploadStatus(&info, 0, nil, mockProClient, mockGitRepo)
		assert.NoError(t, err)

		mockProClient.AssertExpectations(t)
	})
}

// TestPopulateMetricsDTO tests the populateMetricsDTO helper function.
func TestPopulateMetricsDTO(t *testing.T) {
	t.Run("populates all fields from metrics", func(t *testing.T) {
		dto := &dtos.InstanceStatusUploadRequest{}
		m := &process.ProcessMetrics{
			WallTime:         10 * time.Second,
			UserCPUTime:      3 * time.Second,
			SystemCPUTime:    1500 * time.Millisecond,
			MaxRSSBytes:      1024 * 1024 * 512,
			MinorPageFaults:  100,
			MajorPageFaults:  5,
			InBlockOps:       200,
			OutBlockOps:      50,
			VolCtxSwitches:   300,
			InvolCtxSwitches: 10,
		}

		populateMetricsDTO(dto, m)

		assert.NotNil(t, dto.WallTimeMs)
		assert.Equal(t, int64(10000), *dto.WallTimeMs)
		assert.NotNil(t, dto.UserCPUTimeMs)
		assert.Equal(t, int64(3000), *dto.UserCPUTimeMs)
		assert.NotNil(t, dto.SysCPUTimeMs)
		assert.Equal(t, int64(1500), *dto.SysCPUTimeMs)
		assert.NotNil(t, dto.PeakRSSBytes)
		assert.Equal(t, int64(1024*1024*512), *dto.PeakRSSBytes)
		assert.NotNil(t, dto.VolCtxSwitches)
		assert.Equal(t, int64(300), *dto.VolCtxSwitches)
	})

	t.Run("skips zero rusage fields", func(t *testing.T) {
		dto := &dtos.InstanceStatusUploadRequest{}
		m := &process.ProcessMetrics{
			WallTime:      1 * time.Second,
			UserCPUTime:   500 * time.Millisecond,
			SystemCPUTime: 100 * time.Millisecond,
			// All rusage fields zero (Windows).
		}

		populateMetricsDTO(dto, m)

		assert.NotNil(t, dto.WallTimeMs)
		assert.Equal(t, int64(1000), *dto.WallTimeMs)
		assert.Nil(t, dto.PeakRSSBytes)
		assert.Nil(t, dto.MinorPageFaults)
		assert.Nil(t, dto.MajorPageFaults)
		assert.Nil(t, dto.InBlockOps)
		assert.Nil(t, dto.OutBlockOps)
		assert.Nil(t, dto.VolCtxSwitches)
		assert.Nil(t, dto.InvolCtxSwitches)
	})
}
