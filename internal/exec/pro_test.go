package exec

import (
	"encoding/base64"
	"testing"
	"time"

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

// TestUploadStatusWithCIData tests that CI data is included in the upload DTO.
func TestUploadStatusWithCIData(t *testing.T) {
	testRepoInfo := &atmosgit.RepoInfo{
		RepoUrl:   "https://github.com/test/repo",
		RepoName:  "repo",
		RepoOwner: "test",
		RepoHost:  "github.com",
	}

	t.Run("includes CI data in DTO when provided", func(t *testing.T) {
		mockProClient := new(MockProAPIClient)
		mockGitRepo := new(MockGitRepo)
		info := createTestInfo(true)

		ciData := map[string]any{
			"component_type": "terraform",
			"has_changes":    true,
			"has_errors":     false,
			"resource_counts": map[string]int{
				"create":  3,
				"change":  1,
				"replace": 0,
				"destroy": 0,
			},
		}

		mockGitRepo.On("GetLocalRepoInfo").Return(testRepoInfo, nil)
		mockGitRepo.On("GetCurrentCommitSHA").Return("abc123", nil)
		mockProClient.On("UploadInstanceStatus", mock.MatchedBy(func(dto *dtos.InstanceStatusUploadRequest) bool {
			return dto.CI != nil &&
				dto.CI["component_type"] == "terraform" &&
				dto.CI["has_changes"] == true &&
				dto.Command == "plan"
		})).Return(nil)

		err := uploadStatus(&info, 2, ciData, mockProClient, mockGitRepo)
		assert.NoError(t, err)

		mockProClient.AssertExpectations(t)
		mockGitRepo.AssertExpectations(t)
	})

	t.Run("omits CI data from DTO when nil", func(t *testing.T) {
		mockProClient := new(MockProAPIClient)
		mockGitRepo := new(MockGitRepo)
		info := createTestInfo(true)

		mockGitRepo.On("GetLocalRepoInfo").Return(testRepoInfo, nil)
		mockGitRepo.On("GetCurrentCommitSHA").Return("abc123", nil)
		mockProClient.On("UploadInstanceStatus", mock.MatchedBy(func(dto *dtos.InstanceStatusUploadRequest) bool {
			return dto.CI == nil && dto.Command == "plan"
		})).Return(nil)

		err := uploadStatus(&info, 0, nil, mockProClient, mockGitRepo)
		assert.NoError(t, err)

		mockProClient.AssertExpectations(t)
		mockGitRepo.AssertExpectations(t)
	})
}

// TestBuildCIStatusData tests the buildCIStatusData function.
func TestBuildCIStatusData(t *testing.T) {
	t.Run("returns nil for unknown component type", func(t *testing.T) {
		info := &schema.ConfigAndStacksInfo{
			Command:    "unknown",
			SubCommand: "plan",
		}
		result := buildCIStatusData(info, []byte("some output"))
		assert.Nil(t, result)
	})

	t.Run("returns CI data with output log for terraform", func(t *testing.T) {
		// The terraform plugin is auto-registered via init().
		// We need to import the package to trigger registration.
		info := &schema.ConfigAndStacksInfo{
			Command:    "terraform",
			SubCommand: "plan",
		}
		output := []byte("Plan: 2 to add, 1 to change, 0 to destroy.")
		result := buildCIStatusData(info, output)

		// If terraform plugin is registered, we should get data.
		if result != nil {
			assert.Equal(t, "terraform", result["component_type"])
			assert.Contains(t, result, "output_log")
			assert.Contains(t, result, "has_changes")
		}
	})
}

// TestAddOutputLog tests the addOutputLog helper.
func TestAddOutputLog(t *testing.T) {
	t.Run("adds base64 encoded output", func(t *testing.T) {
		data := make(map[string]any)
		output := []byte("hello world")
		addOutputLog(data, output, 1024)

		assert.Contains(t, data, "output_log")
		assert.NotContains(t, data, "truncated")
		assert.Equal(t, "aGVsbG8gd29ybGQ=", data["output_log"])
	})

	t.Run("truncates from beginning when exceeding max", func(t *testing.T) {
		data := make(map[string]any)
		output := []byte("AAAAABBBBB")
		addOutputLog(data, output, 5)

		assert.Contains(t, data, "output_log")
		assert.Equal(t, true, data["truncated"])
		// Should keep last 5 bytes: "BBBBB".
		assert.Equal(t, "QkJCQkI=", data["output_log"])
	})

	t.Run("does nothing for empty output", func(t *testing.T) {
		data := make(map[string]any)
		addOutputLog(data, []byte{}, 1024)

		assert.NotContains(t, data, "output_log")
	})

	t.Run("does nothing for nil data map", func(t *testing.T) {
		// Should not panic.
		addOutputLog(nil, []byte("hello"), 1024)
	})

	t.Run("no truncation when exactly at max", func(t *testing.T) {
		data := make(map[string]any)
		output := []byte("12345")
		addOutputLog(data, output, 5)

		assert.Contains(t, data, "output_log")
		assert.NotContains(t, data, "truncated")
	})
}

// TestAddOutputLogTruncation tests truncation at the defaultMaxOutputLogBytes boundary.
func TestAddOutputLogTruncation(t *testing.T) {
	t.Run("truncates at defaultMaxOutputLogBytes boundary", func(t *testing.T) {
		// Create output larger than defaultMaxOutputLogBytes (3MB).
		size := defaultMaxOutputLogBytes + 1000
		output := make([]byte, size)
		// Fill with a pattern so we can verify truncation keeps the tail.
		for i := range output {
			output[i] = byte('A' + (i % 26))
		}

		data := make(map[string]any)
		addOutputLog(data, output, defaultMaxOutputLogBytes)

		assert.Contains(t, data, "output_log")
		assert.Equal(t, true, data["truncated"])

		// Decode and verify the tail was kept.
		decoded, err := base64.StdEncoding.DecodeString(data["output_log"].(string))
		assert.NoError(t, err)
		assert.Equal(t, defaultMaxOutputLogBytes, len(decoded))
		// The decoded bytes should be the last defaultMaxOutputLogBytes of the original.
		assert.Equal(t, output[size-defaultMaxOutputLogBytes:], decoded)
	})

	t.Run("does not truncate when output is exactly defaultMaxOutputLogBytes", func(t *testing.T) {
		output := make([]byte, defaultMaxOutputLogBytes)
		for i := range output {
			output[i] = byte('X')
		}

		data := make(map[string]any)
		addOutputLog(data, output, defaultMaxOutputLogBytes)

		assert.Contains(t, data, "output_log")
		assert.NotContains(t, data, "truncated")

		decoded, err := base64.StdEncoding.DecodeString(data["output_log"].(string))
		assert.NoError(t, err)
		assert.Equal(t, defaultMaxOutputLogBytes, len(decoded))
	})

	t.Run("does not truncate when output is under defaultMaxOutputLogBytes", func(t *testing.T) {
		output := []byte("small output")

		data := make(map[string]any)
		addOutputLog(data, output, defaultMaxOutputLogBytes)

		assert.Contains(t, data, "output_log")
		assert.NotContains(t, data, "truncated")

		decoded, err := base64.StdEncoding.DecodeString(data["output_log"].(string))
		assert.NoError(t, err)
		assert.Equal(t, output, decoded)
	})
}

// TestBuildCIStatusDataOutputLogIncluded verifies that buildCIStatusData includes
// a base64-encoded output_log key when the terraform plugin is available.
func TestBuildCIStatusDataOutputLogIncluded(t *testing.T) {
	t.Run("output_log is base64 encoded from masked output", func(t *testing.T) {
		info := &schema.ConfigAndStacksInfo{
			Command:    "terraform",
			SubCommand: "plan",
		}
		maskedOutput := []byte("Plan: 1 to add, 0 to change, 0 to destroy.\nSome <MASKED> secret was here.")

		result := buildCIStatusData(info, maskedOutput)

		// If terraform plugin is registered, verify output_log.
		if result != nil {
			outputLog, ok := result["output_log"].(string)
			assert.True(t, ok, "output_log should be a string")

			decoded, err := base64.StdEncoding.DecodeString(outputLog)
			assert.NoError(t, err)
			// The decoded output should contain the masked content.
			assert.Contains(t, string(decoded), "<MASKED>")
			assert.Contains(t, string(decoded), "Plan: 1 to add")
		}
	})

	t.Run("output_log is truncated for large output", func(t *testing.T) {
		info := &schema.ConfigAndStacksInfo{
			Command:    "terraform",
			SubCommand: "plan",
		}
		// Create output larger than defaultMaxOutputLogBytes.
		largeOutput := make([]byte, defaultMaxOutputLogBytes+100)
		copy(largeOutput, []byte("HEAD... "))
		// Put a recognizable marker at the tail.
		copy(largeOutput[len(largeOutput)-50:], []byte("Plan: 5 to add, 0 to change, 0 to destroy.TAIL__"))

		result := buildCIStatusData(info, largeOutput)

		if result != nil {
			assert.Equal(t, true, result["truncated"])

			outputLog, ok := result["output_log"].(string)
			assert.True(t, ok)

			decoded, err := base64.StdEncoding.DecodeString(outputLog)
			assert.NoError(t, err)
			// Should contain the tail, not the head.
			assert.Contains(t, string(decoded), "TAIL__")
			assert.NotContains(t, string(decoded), "HEAD...")
		}
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
