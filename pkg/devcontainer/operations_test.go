package devcontainer

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/container"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

// TestIsContainerRunning validates the isContainerRunning status checking logic.
// This is a critical decision point used throughout devcontainer lifecycle management
// to determine whether containers need to be started, stopped, or are already in the
// desired state. The function must match exact status strings ("running", "Running", "Up")
// to avoid false positives with similar-looking statuses like "Up 5 minutes".
//
// The duplication is by design: each test suite validates different domain-specific status logic
// with its own set of expected values and edge cases. Consolidating these would reduce readability
// and make it harder to maintain domain-specific test scenarios independently.
//
//nolint:dupl // This table-driven test intentionally shares structure with other status validation tests.
func TestIsContainerRunning(t *testing.T) {
	tests := []struct {
		name     string
		status   string
		expected bool
	}{
		{
			name:     "lowercase running",
			status:   "running",
			expected: true,
		},
		{
			name:     "capitalized Running",
			status:   "Running",
			expected: true,
		},
		{
			name:     "status Up",
			status:   "Up",
			expected: true,
		},
		{
			name:     "status exited",
			status:   "exited",
			expected: false,
		},
		{
			name:     "status stopped",
			status:   "stopped",
			expected: false,
		},
		{
			name:     "status paused",
			status:   "paused",
			expected: false,
		},
		{
			name:     "status created",
			status:   "created",
			expected: false,
		},
		{
			name:     "empty status",
			status:   "",
			expected: false,
		},
		{
			name:     "uppercase RUNNING",
			status:   "RUNNING",
			expected: false,
		},
		{
			name:     "status with spaces",
			status:   " running ",
			expected: false,
		},
		{
			name:     "Docker status Up 5 minutes",
			status:   "Up 5 minutes",
			expected: false,
		},
		{
			name:     "exact match Up only",
			status:   "Up",
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isContainerRunning(tt.status)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestFormatPortsInfo validates the formatPortsInfo formatting helper.
func TestFormatPortsInfo(t *testing.T) {
	tests := []struct {
		name         string
		forwardPorts []interface{}
		expected     string
	}{
		{
			name:         "empty ports",
			forwardPorts: []interface{}{},
			expected:     "",
		},
		{
			name:         "nil ports",
			forwardPorts: nil,
			expected:     "",
		},
		{
			name:         "single int port",
			forwardPorts: []interface{}{8080},
			expected:     "Ports: 8080",
		},
		{
			name:         "single float64 port",
			forwardPorts: []interface{}{8080.0},
			expected:     "Ports: 8080",
		},
		{
			name:         "single string port",
			forwardPorts: []interface{}{"8080:8080"},
			expected:     "Ports: 8080:8080",
		},
		{
			name:         "multiple int ports",
			forwardPorts: []interface{}{8080, 3000, 5432},
			expected:     "Ports: 8080, 3000, 5432",
		},
		{
			name:         "mixed types",
			forwardPorts: []interface{}{8080, 3000.0, "5432:5432"},
			expected:     "Ports: 8080, 3000, 5432:5432",
		},
		{
			name:         "float64 with decimals gets truncated",
			forwardPorts: []interface{}{8080.9},
			expected:     "Ports: 8080",
		},
		{
			name:         "unsupported type ignored",
			forwardPorts: []interface{}{8080, true, "3000"}, // bool is ignored.
			expected:     "Ports: 8080, 3000",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatPortsInfo(tt.forwardPorts)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestStopContainerIfRunning validates the stopContainerIfRunning operation.
func TestStopContainerIfRunning(t *testing.T) {
	tests := []struct {
		name        string
		status      string
		setupMock   func(*MockRuntime)
		expectError bool
	}{
		{
			name:   "container not running - no-op",
			status: "exited",
			setupMock: func(m *MockRuntime) {
				// Should not call Stop when container is not running.
			},
			expectError: false,
		},
		{
			name:   "container stopped - no-op",
			status: "stopped",
			setupMock: func(m *MockRuntime) {
				// Should not call Stop when container is stopped.
			},
			expectError: false,
		},
		{
			name:   "container running - calls Stop",
			status: "running",
			setupMock: func(m *MockRuntime) {
				m.EXPECT().
					Stop(gomock.Any(), "test-id", defaultContainerStopTimeout).
					Return(nil)
			},
			expectError: false,
		},
		{
			name:   "container Up - calls Stop",
			status: "Up",
			setupMock: func(m *MockRuntime) {
				m.EXPECT().
					Stop(gomock.Any(), "test-id", defaultContainerStopTimeout).
					Return(nil)
			},
			expectError: false,
		},
		{
			name:   "stop fails - wraps error",
			status: "running",
			setupMock: func(m *MockRuntime) {
				m.EXPECT().
					Stop(gomock.Any(), "test-id", defaultContainerStopTimeout).
					Return(errors.New("runtime error"))
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockRuntime := NewMockRuntime(ctrl)
			tt.setupMock(mockRuntime)

			containerInfo := &container.Info{
				ID:     "test-id",
				Name:   "test-container",
				Status: tt.status,
			}

			err := stopContainerIfRunning(context.Background(), mockRuntime, containerInfo)

			if tt.expectError {
				require.Error(t, err)
				assert.ErrorIs(t, err, errUtils.ErrContainerRuntimeOperation)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// TestBuildImageIfNeeded validates the buildImageIfNeeded operation.
func TestBuildImageIfNeeded(t *testing.T) {
	tests := []struct {
		name        string
		config      *Config
		setupMock   func(*MockRuntime)
		expectBuild bool
		expectImage string
		expectError bool
	}{
		{
			name: "no build config - no-op",
			config: &Config{
				Image: "existing-image",
				Build: nil,
			},
			setupMock: func(m *MockRuntime) {
				// Should not call Build when Build is nil.
			},
			expectBuild: false,
			expectImage: "existing-image",
			expectError: false,
		},
		{
			name: "build config provided - builds and updates image",
			config: &Config{
				Image: "",
				Build: &Build{
					Context:    "./context",
					Dockerfile: "Dockerfile",
					Args: map[string]string{
						"VERSION": "1.0",
					},
				},
			},
			setupMock: func(m *MockRuntime) {
				m.EXPECT().
					Build(gomock.Any(), gomock.Any()).
					DoAndReturn(func(ctx context.Context, cfg *container.BuildConfig) error {
						// Verify build config.
						assert.Equal(t, "./context", cfg.Context)
						assert.Equal(t, "Dockerfile", cfg.Dockerfile)
						assert.Equal(t, []string{"atmos-devcontainer-test-dev"}, cfg.Tags)
						assert.Equal(t, map[string]string{"VERSION": "1.0"}, cfg.Args)
						return nil
					})
			},
			expectBuild: true,
			expectImage: "atmos-devcontainer-test-dev",
			expectError: false,
		},
		{
			name: "build fails - wraps error",
			config: &Config{
				Build: &Build{
					Context:    "./context",
					Dockerfile: "Dockerfile",
				},
			},
			setupMock: func(m *MockRuntime) {
				m.EXPECT().
					Build(gomock.Any(), gomock.Any()).
					Return(errors.New("build failed"))
			},
			expectBuild: true,
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockRuntime := NewMockRuntime(ctrl)
			tt.setupMock(mockRuntime)

			err := buildImageIfNeeded(context.Background(), mockRuntime, tt.config, "test-dev")

			if tt.expectError {
				require.Error(t, err)
				assert.ErrorIs(t, err, errUtils.ErrContainerRuntimeOperation)
			} else {
				require.NoError(t, err)
				if tt.expectBuild {
					assert.Equal(t, tt.expectImage, tt.config.Image)
				}
			}
		})
	}
}

// TestPullImageIfNeeded validates the pullImageIfNeeded operation.
func TestPullImageIfNeeded(t *testing.T) {
	tests := []struct {
		name        string
		image       string
		noPull      bool
		setupMock   func(*MockRuntime)
		expectError bool
	}{
		{
			name:   "noPull is true - no-op",
			image:  "test-image:latest",
			noPull: true,
			setupMock: func(m *MockRuntime) {
				// Should not call Pull when noPull is true.
			},
			expectError: false,
		},
		{
			name:   "empty image - no-op",
			image:  "",
			noPull: false,
			setupMock: func(m *MockRuntime) {
				// Should not call Pull when image is empty.
			},
			expectError: false,
		},
		{
			name:   "pull image success",
			image:  "test-image:latest",
			noPull: false,
			setupMock: func(m *MockRuntime) {
				m.EXPECT().
					Pull(gomock.Any(), "test-image:latest").
					Return(nil)
			},
			expectError: false,
		},
		{
			name:   "pull fails - wraps error",
			image:  "test-image:latest",
			noPull: false,
			setupMock: func(m *MockRuntime) {
				m.EXPECT().
					Pull(gomock.Any(), "test-image:latest").
					Return(errors.New("network error"))
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockRuntime := NewMockRuntime(ctrl)
			tt.setupMock(mockRuntime)

			err := pullImageIfNeeded(context.Background(), mockRuntime, tt.image, tt.noPull)

			if tt.expectError {
				require.Error(t, err)
				assert.ErrorIs(t, err, errUtils.ErrContainerRuntimeOperation)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// TestCreateContainer validates the createContainer operation.
func TestCreateContainer(t *testing.T) {
	tests := []struct {
		name          string
		containerName string
		setupMock     func(*MockRuntime)
		expectedID    string
		expectError   bool
	}{
		{
			name:          "create success",
			containerName: "test-container",
			setupMock: func(m *MockRuntime) {
				m.EXPECT().
					Create(gomock.Any(), gomock.Any()).
					DoAndReturn(func(ctx context.Context, cfg *container.CreateConfig) (string, error) {
						// Verify container name is passed correctly.
						assert.Equal(t, "test-container", cfg.Name)
						return "container-id-123", nil
					})
			},
			expectedID:  "container-id-123",
			expectError: false,
		},
		{
			name:          "create fails - wraps error",
			containerName: "test-container",
			setupMock: func(m *MockRuntime) {
				m.EXPECT().
					Create(gomock.Any(), gomock.Any()).
					Return("", errors.New("container name conflict"))
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockRuntime := NewMockRuntime(ctrl)
			tt.setupMock(mockRuntime)

			params := &containerParams{
				ctx:           context.Background(),
				runtime:       mockRuntime,
				config:        &Config{Image: "test-image"},
				containerName: tt.containerName,
				name:          "test",
				instance:      "default",
			}

			containerID, err := createContainer(params)

			if tt.expectError {
				require.Error(t, err)
				assert.ErrorIs(t, err, errUtils.ErrContainerRuntimeOperation)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expectedID, containerID)
			}
		})
	}
}

// TestStartContainer validates the startContainer operation.
func TestStartContainer(t *testing.T) {
	tests := []struct {
		name          string
		containerID   string
		containerName string
		setupMock     func(*MockRuntime)
		expectError   bool
	}{
		{
			name:          "start success",
			containerID:   "container-id-123",
			containerName: "test-container",
			setupMock: func(m *MockRuntime) {
				m.EXPECT().
					Start(gomock.Any(), "container-id-123").
					Return(nil)
			},
			expectError: false,
		},
		{
			name:          "start fails - wraps error",
			containerID:   "container-id-123",
			containerName: "test-container",
			setupMock: func(m *MockRuntime) {
				m.EXPECT().
					Start(gomock.Any(), "container-id-123").
					Return(errors.New("container not found"))
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockRuntime := NewMockRuntime(ctrl)
			tt.setupMock(mockRuntime)

			err := startContainer(context.Background(), mockRuntime, tt.containerID, tt.containerName)

			if tt.expectError {
				require.Error(t, err)
				assert.ErrorIs(t, err, errUtils.ErrContainerRuntimeOperation)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// TestRemoveContainer validates the removeContainer operation.
func TestRemoveContainer(t *testing.T) {
	tests := []struct {
		name          string
		containerInfo *container.Info
		containerName string
		setupMock     func(*MockRuntime)
		expectError   bool
	}{
		{
			name: "remove success",
			containerInfo: &container.Info{
				ID:   "container-id-123",
				Name: "test-container",
			},
			containerName: "test-container",
			setupMock: func(m *MockRuntime) {
				m.EXPECT().
					Remove(gomock.Any(), "container-id-123", true).
					Return(nil)
			},
			expectError: false,
		},
		{
			name: "remove fails - wraps error",
			containerInfo: &container.Info{
				ID:   "container-id-123",
				Name: "test-container",
			},
			containerName: "test-container",
			setupMock: func(m *MockRuntime) {
				m.EXPECT().
					Remove(gomock.Any(), "container-id-123", true).
					Return(errors.New("container is running"))
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockRuntime := NewMockRuntime(ctrl)
			tt.setupMock(mockRuntime)

			err := removeContainer(context.Background(), mockRuntime, tt.containerInfo, tt.containerName)

			if tt.expectError {
				require.Error(t, err)
				assert.ErrorIs(t, err, errUtils.ErrContainerRuntimeOperation)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// TestStartExistingContainer validates the startExistingContainer operation.
func TestStopAndRemoveContainer(t *testing.T) {
	tests := []struct {
		name        string
		setupMock   func(*MockRuntime)
		expectError bool
	}{
		{
			name: "container doesn't exist - no-op",
			setupMock: func(m *MockRuntime) {
				m.EXPECT().
					Inspect(gomock.Any(), "test-container").
					Return(nil, errors.New("container not found"))
			},
			expectError: false,
		},
		{
			name: "running container - stops and removes",
			setupMock: func(m *MockRuntime) {
				m.EXPECT().
					Inspect(gomock.Any(), "test-container").
					Return(&container.Info{
						ID:     "test-id",
						Name:   "test-container",
						Status: "running",
					}, nil)
				m.EXPECT().
					Stop(gomock.Any(), "test-id", defaultContainerStopTimeout).
					Return(nil)
				m.EXPECT().
					Remove(gomock.Any(), "test-id", true).
					Return(nil)
			},
			expectError: false,
		},
		{
			name: "stopped container - removes only",
			setupMock: func(m *MockRuntime) {
				m.EXPECT().
					Inspect(gomock.Any(), "test-container").
					Return(&container.Info{
						ID:     "test-id",
						Name:   "test-container",
						Status: "exited",
					}, nil)
				m.EXPECT().
					Remove(gomock.Any(), "test-id", true).
					Return(nil)
			},
			expectError: false,
		},
		{
			name: "stop fails - propagates error",
			setupMock: func(m *MockRuntime) {
				m.EXPECT().
					Inspect(gomock.Any(), "test-container").
					Return(&container.Info{
						ID:     "test-id",
						Name:   "test-container",
						Status: "running",
					}, nil)
				m.EXPECT().
					Stop(gomock.Any(), "test-id", defaultContainerStopTimeout).
					Return(errors.New("stop failed"))
			},
			expectError: true,
		},
		{
			name: "remove fails - propagates error",
			setupMock: func(m *MockRuntime) {
				m.EXPECT().
					Inspect(gomock.Any(), "test-container").
					Return(&container.Info{
						ID:     "test-id",
						Name:   "test-container",
						Status: "exited",
					}, nil)
				m.EXPECT().
					Remove(gomock.Any(), "test-id", true).
					Return(errors.New("remove failed"))
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockRuntime := NewMockRuntime(ctrl)
			tt.setupMock(mockRuntime)

			err := stopAndRemoveContainer(context.Background(), mockRuntime, "test-container")

			if tt.expectError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// TestCreateAndStartNewContainer validates the createAndStartNewContainer orchestration.
func TestCreateAndStartNewContainer(t *testing.T) {
	tests := []struct {
		name        string
		config      *Config
		setupMock   func(*MockRuntime)
		expectError bool
	}{
		{
			name: "full orchestration success - no build",
			config: &Config{
				Image:           "test-image:latest",
				Build:           nil,
				WorkspaceFolder: "/workspace",
				ForwardPorts:    []interface{}{8080},
			},
			setupMock: func(m *MockRuntime) {
				// Should create and start container.
				m.EXPECT().
					Create(gomock.Any(), gomock.Any()).
					Return("container-id-123", nil)
				m.EXPECT().
					Start(gomock.Any(), "container-id-123").
					Return(nil)
			},
			expectError: false,
		},
		{
			name: "full orchestration success - with build",
			config: &Config{
				Image: "",
				Build: &Build{
					Context:    "./context",
					Dockerfile: "Dockerfile",
				},
				WorkspaceFolder: "/workspace",
			},
			setupMock: func(m *MockRuntime) {
				// Should build, create, and start.
				m.EXPECT().
					Build(gomock.Any(), gomock.Any()).
					Return(nil)
				m.EXPECT().
					Create(gomock.Any(), gomock.Any()).
					Return("container-id-123", nil)
				m.EXPECT().
					Start(gomock.Any(), "container-id-123").
					Return(nil)
			},
			expectError: false,
		},
		{
			name: "build fails - aborts before create",
			config: &Config{
				Build: &Build{
					Context:    "./context",
					Dockerfile: "Dockerfile",
				},
			},
			setupMock: func(m *MockRuntime) {
				m.EXPECT().
					Build(gomock.Any(), gomock.Any()).
					Return(errors.New("build failed"))
				// Should NOT call Create or Start.
			},
			expectError: true,
		},
		{
			name: "create fails - aborts before start",
			config: &Config{
				Image: "test-image:latest",
				Build: nil,
			},
			setupMock: func(m *MockRuntime) {
				m.EXPECT().
					Create(gomock.Any(), gomock.Any()).
					Return("", errors.New("create failed"))
				// Should NOT call Start.
			},
			expectError: true,
		},
		{
			name: "start fails - propagates error",
			config: &Config{
				Image: "test-image:latest",
				Build: nil,
			},
			setupMock: func(m *MockRuntime) {
				m.EXPECT().
					Create(gomock.Any(), gomock.Any()).
					Return("container-id-123", nil)
				m.EXPECT().
					Start(gomock.Any(), "container-id-123").
					Return(errors.New("start failed"))
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockRuntime := NewMockRuntime(ctrl)
			tt.setupMock(mockRuntime)

			params := &containerParams{
				ctx:           context.Background(),
				runtime:       mockRuntime,
				config:        tt.config,
				containerName: "test-container",
				name:          "test",
				instance:      "default",
			}

			err := createAndStartNewContainer(params)

			if tt.expectError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// TestFormatWorkspaceInfo validates the formatWorkspaceInfo formatting helper.
func TestFormatWorkspaceInfo(t *testing.T) {
	tests := []struct {
		name            string
		workspaceFolder string
		expectedPrefix  string
	}{
		{
			name:            "empty workspace folder",
			workspaceFolder: "",
			expectedPrefix:  "",
		},
		{
			name:            "workspace folder set",
			workspaceFolder: "/workspace",
			expectedPrefix:  "Workspace",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatWorkspaceInfo(tt.workspaceFolder)
			if tt.expectedPrefix == "" {
				assert.Empty(t, result)
			} else {
				assert.Contains(t, result, tt.expectedPrefix)
				assert.Contains(t, result, tt.workspaceFolder)
			}
		})
	}
}

// TestDefaultContainerStopTimeout validates the timeout constant.
func TestDefaultContainerStopTimeout(t *testing.T) {
	assert.Equal(t, 10*time.Second, defaultContainerStopTimeout)
}

// TestRunWithSpinner validates the spinner wrapper is a passthrough.
func TestRunWithSpinner(t *testing.T) {
	tests := []struct {
		name        string
		operation   func() error
		expectError bool
	}{
		{
			name: "operation succeeds",
			operation: func() error {
				return nil
			},
			expectError: false,
		},
		{
			name: "operation fails",
			operation: func() error {
				return fmt.Errorf("test error")
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := runWithSpinner("progress", "completed", tt.operation)
			if tt.expectError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// TestExtractDevcontainerName validates the extractDevcontainerName helper function.
func TestExtractDevcontainerName(t *testing.T) {
	tests := []struct {
		name          string
		containerName string
		expected      string
	}{
		{
			name:          "standard format with default instance",
			containerName: "atmos-devcontainer.geodesic.default",
			expected:      "geodesic",
		},
		{
			name:          "standard format with custom instance",
			containerName: "atmos-devcontainer.terraform.project-a",
			expected:      "terraform",
		},
		{
			name:          "format with suffix",
			containerName: "atmos-devcontainer.geodesic.default-2",
			expected:      "geodesic",
		},
		{
			name:          "format with multiple dots",
			containerName: "atmos-devcontainer.my-dev.instance-1",
			expected:      "my-dev",
		},
		{
			name:          "no prefix - returns as is",
			containerName: "geodesic.default",
			expected:      "geodesic.default",
		},
		{
			name:          "empty string",
			containerName: "",
			expected:      "",
		},
		{
			name:          "only prefix",
			containerName: "atmos-devcontainer.",
			expected:      "",
		},
		{
			name:          "prefix with single name",
			containerName: "atmos-devcontainer.test",
			expected:      "test",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractDevcontainerName(tt.containerName)
			assert.Equal(t, tt.expected, result)
		})
	}
}
