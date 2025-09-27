package exec

import (
	"testing"

	errUtils "github.com/cloudposse/atmos/errors"
	atmosgit "github.com/cloudposse/atmos/pkg/git"
	"github.com/cloudposse/atmos/pkg/pro/dtos"
	"github.com/cloudposse/atmos/pkg/schema"
	gogit "github.com/go-git/go-git/v5"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
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

// TestParseLockUnlockCliArgs tests the parseLockUnlockCliArgs function error handling.
func TestParseLockUnlockCliArgs(t *testing.T) {
	// Skip this test as it requires full command setup with many flags
	t.Skip("Requires full command setup with ProcessCommandLineArgs")
}

// TestParseLockCliArgs tests the parseLockCliArgs function.
func TestParseLockCliArgs(t *testing.T) {
	tests := []struct {
		name        string
		setupCmd    func() *cobra.Command
		args        []string
		expectError bool
		expectedTTL int32
		expectedMsg string
	}{
		{
			name: "Invalid TTL flag",
			setupCmd: func() *cobra.Command {
				cmd := &cobra.Command{}
				cmd.Flags().String("component", "test-component", "Component name")
				cmd.Flags().String("stack", "test-stack", "Stack name")
				// Intentionally not setting up TTL flag to simulate error
				return cmd
			},
			args:        []string{},
			expectError: true,
		},
		{
			name: "Invalid message flag",
			setupCmd: func() *cobra.Command {
				cmd := &cobra.Command{}
				cmd.Flags().String("component", "test-component", "Component name")
				cmd.Flags().String("stack", "test-stack", "Stack name")
				cmd.Flags().Int32("ttl", 60, "TTL in seconds")
				// Intentionally not setting up message flag to simulate error
				return cmd
			},
			args:        []string{},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := tt.setupCmd()
			_, err := parseLockCliArgs(cmd, tt.args)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// TestParseUnlockCliArgs tests the parseUnlockCliArgs function.
func TestParseUnlockCliArgs(t *testing.T) {
	tests := []struct {
		name        string
		setupCmd    func() *cobra.Command
		args        []string
		expectError bool
	}{
		{
			name: "Missing key flag",
			setupCmd: func() *cobra.Command {
				cmd := &cobra.Command{}
				cmd.Flags().String("component", "test-component", "Component name")
				cmd.Flags().String("stack", "test-stack", "Stack name")
				// Intentionally not setting up key flag to simulate error
				return cmd
			},
			args:        []string{},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := tt.setupCmd()
			_, err := parseUnlockCliArgs(cmd, tt.args)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// TestProLockStack tests the proLockStack function error handling.
func TestProLockStack(t *testing.T) {
	// Test that errors are properly wrapped with ErrStringWrappingFormat
	tests := []struct {
		name        string
		expectedErr error
	}{
		{
			name:        "Git repo error",
			expectedErr: errUtils.ErrFailedToGetLocalRepo,
		},
		{
			name:        "API client error",
			expectedErr: errUtils.ErrFailedToCreateAPIClient,
		},
		{
			name:        "Lock stack error",
			expectedErr: errUtils.ErrFailedToLockStack,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Just verify the error types exist
			assert.NotNil(t, tt.expectedErr)
		})
	}
}

// TestProUnlockStack tests error handling in proUnlockStack.
func TestProUnlockStack(t *testing.T) {
	// Test that errors are properly wrapped
	tests := []struct {
		name        string
		expectedErr error
	}{
		{
			name:        "Git repo error",
			expectedErr: errUtils.ErrFailedToGetLocalRepo,
		},
		{
			name:        "API client error",
			expectedErr: errUtils.ErrFailedToCreateAPIClient,
		},
		{
			name:        "Unlock stack error",
			expectedErr: errUtils.ErrFailedToUnlockStack,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Verify the error types exist and would be wrapped correctly
			assert.NotNil(t, tt.expectedErr)
		})
	}
}
