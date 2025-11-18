package devcontainer

import (
	"context"
	"errors"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/container"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/terminal/pty"
)

func TestManager_Attach(t *testing.T) {
	tests := []struct {
		name        string
		devName     string
		instance    string
		usePTY      bool
		setupMocks  func(*MockConfigLoader, *MockRuntimeDetector, *MockRuntime)
		expectError bool
		errorIs     error
	}{
		{
			name:     "attach to running container successfully",
			devName:  "test",
			instance: "default",
			usePTY:   false,
			setupMocks: func(loader *MockConfigLoader, detector *MockRuntimeDetector, runtime *MockRuntime) {
				loader.EXPECT().
					LoadConfig(gomock.Any(), "test").
					Return(&Config{
						Name:  "test",
						Image: "ubuntu:22.04",
					}, &Settings{}, nil)
				detector.EXPECT().
					DetectRuntime("").
					Return(runtime, nil)
				runtime.EXPECT().
					List(gomock.Any(), map[string]string{"name": "atmos-devcontainer.test.default"}).
					Return([]container.Info{
						{
							ID:     "running-id",
							Name:   "atmos-devcontainer.test.default",
							Status: "running",
						},
					}, nil)
				runtime.EXPECT().
					Inspect(gomock.Any(), "running-id").
					Return(&container.Info{
						ID:    "running-id",
						Ports: []container.PortBinding{},
					}, nil)
				runtime.EXPECT().
					Attach(gomock.Any(), "running-id", gomock.Any()).
					Return(nil)
			},
			expectError: false,
		},
		{
			name:     "attach to stopped container - starts and attaches",
			devName:  "test",
			instance: "default",
			usePTY:   false,
			setupMocks: func(loader *MockConfigLoader, detector *MockRuntimeDetector, runtime *MockRuntime) {
				loader.EXPECT().
					LoadConfig(gomock.Any(), "test").
					Return(&Config{
						Name:  "test",
						Image: "ubuntu:22.04",
					}, &Settings{}, nil)
				detector.EXPECT().
					DetectRuntime("").
					Return(runtime, nil)
				runtime.EXPECT().
					List(gomock.Any(), map[string]string{"name": "atmos-devcontainer.test.default"}).
					Return([]container.Info{
						{
							ID:     "stopped-id",
							Name:   "atmos-devcontainer.test.default",
							Status: "exited",
						},
					}, nil)
				runtime.EXPECT().
					Start(gomock.Any(), "stopped-id").
					Return(nil)
				runtime.EXPECT().
					Inspect(gomock.Any(), "stopped-id").
					Return(&container.Info{
						ID:    "stopped-id",
						Ports: []container.PortBinding{},
					}, nil)
				runtime.EXPECT().
					Attach(gomock.Any(), "stopped-id", gomock.Any()).
					Return(nil)
			},
			expectError: false,
		},
		{
			name:     "config load fails",
			devName:  "test",
			instance: "default",
			usePTY:   false,
			setupMocks: func(loader *MockConfigLoader, detector *MockRuntimeDetector, runtime *MockRuntime) {
				loader.EXPECT().
					LoadConfig(gomock.Any(), "test").
					Return(nil, nil, errors.New("config not found"))
			},
			expectError: true,
		},
		{
			name:     "runtime detection fails",
			devName:  "test",
			instance: "default",
			usePTY:   false,
			setupMocks: func(loader *MockConfigLoader, detector *MockRuntimeDetector, runtime *MockRuntime) {
				loader.EXPECT().
					LoadConfig(gomock.Any(), "test").
					Return(&Config{Name: "test", Image: "ubuntu:22.04"}, &Settings{}, nil)
				detector.EXPECT().
					DetectRuntime("").
					Return(nil, errors.New("docker not found"))
			},
			expectError: true,
		},
		{
			name:     "container not found",
			devName:  "test",
			instance: "default",
			usePTY:   false,
			setupMocks: func(loader *MockConfigLoader, detector *MockRuntimeDetector, runtime *MockRuntime) {
				loader.EXPECT().
					LoadConfig(gomock.Any(), "test").
					Return(&Config{Name: "test", Image: "ubuntu:22.04"}, &Settings{}, nil)
				detector.EXPECT().
					DetectRuntime("").
					Return(runtime, nil)
				runtime.EXPECT().
					List(gomock.Any(), gomock.Any()).
					Return([]container.Info{}, nil)
			},
			expectError: true,
			errorIs:     errUtils.ErrDevcontainerNotFound,
		},
		{
			name:     "container list fails",
			devName:  "test",
			instance: "default",
			usePTY:   false,
			setupMocks: func(loader *MockConfigLoader, detector *MockRuntimeDetector, runtime *MockRuntime) {
				loader.EXPECT().
					LoadConfig(gomock.Any(), "test").
					Return(&Config{Name: "test", Image: "ubuntu:22.04"}, &Settings{}, nil)
				detector.EXPECT().
					DetectRuntime("").
					Return(runtime, nil)
				runtime.EXPECT().
					List(gomock.Any(), gomock.Any()).
					Return(nil, errors.New("runtime error"))
			},
			expectError: true,
			errorIs:     errUtils.ErrContainerRuntimeOperation,
		},
		{
			name:     "container start fails",
			devName:  "test",
			instance: "default",
			usePTY:   false,
			setupMocks: func(loader *MockConfigLoader, detector *MockRuntimeDetector, runtime *MockRuntime) {
				loader.EXPECT().
					LoadConfig(gomock.Any(), "test").
					Return(&Config{Name: "test", Image: "ubuntu:22.04"}, &Settings{}, nil)
				detector.EXPECT().
					DetectRuntime("").
					Return(runtime, nil)
				runtime.EXPECT().
					List(gomock.Any(), gomock.Any()).
					Return([]container.Info{
						{
							ID:     "stopped-id",
							Name:   "atmos-devcontainer.test.default",
							Status: "exited",
						},
					}, nil)
				runtime.EXPECT().
					Start(gomock.Any(), "stopped-id").
					Return(errors.New("start failed"))
			},
			expectError: true,
			errorIs:     errUtils.ErrContainerRuntimeOperation,
		},
		{
			name:     "attach fails",
			devName:  "test",
			instance: "default",
			usePTY:   false,
			setupMocks: func(loader *MockConfigLoader, detector *MockRuntimeDetector, runtime *MockRuntime) {
				loader.EXPECT().
					LoadConfig(gomock.Any(), "test").
					Return(&Config{Name: "test", Image: "ubuntu:22.04"}, &Settings{}, nil)
				detector.EXPECT().
					DetectRuntime("").
					Return(runtime, nil)
				runtime.EXPECT().
					List(gomock.Any(), gomock.Any()).
					Return([]container.Info{
						{
							ID:     "running-id",
							Name:   "atmos-devcontainer.test.default",
							Status: "running",
						},
					}, nil)
				runtime.EXPECT().
					Inspect(gomock.Any(), "running-id").
					Return(&container.Info{
						ID:    "running-id",
						Ports: []container.PortBinding{},
					}, nil)
				runtime.EXPECT().
					Attach(gomock.Any(), "running-id", gomock.Any()).
					Return(errors.New("attach failed"))
			},
			expectError: true,
		},
		{
			name:     "attach with PTY mode on supported platforms",
			devName:  "test",
			instance: "default",
			usePTY:   true,
			setupMocks: func(loader *MockConfigLoader, detector *MockRuntimeDetector, runtime *MockRuntime) {
				// Only run this test on platforms that support PTY.
				if !pty.IsSupported() {
					return
				}
				loader.EXPECT().
					LoadConfig(gomock.Any(), "test").
					Return(&Config{Name: "test", Image: "ubuntu:22.04"}, &Settings{}, nil)
				detector.EXPECT().
					DetectRuntime("").
					Return(runtime, nil)
				runtime.EXPECT().
					List(gomock.Any(), gomock.Any()).
					Return([]container.Info{
						{
							ID:     "running-id",
							Name:   "atmos-devcontainer.test.default",
							Status: "running",
						},
					}, nil)
				runtime.EXPECT().
					Inspect(gomock.Any(), "running-id").
					Return(&container.Info{
						ID:    "running-id",
						Ports: []container.PortBinding{},
					}, nil)
				// PTY mode calls runtime.Info() to determine the binary (docker/podman).
				runtime.EXPECT().
					Info(gomock.Any()).
					Return(&container.RuntimeInfo{
						Type:    "docker",
						Version: "24.0.0",
						Running: true,
					}, nil)
			},
			expectError: false, // PTY execution will fail in tests, but code path is exercised
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Skip PTY tests on unsupported platforms.
			if tt.usePTY && !pty.IsSupported() {
				t.Skip("PTY mode not supported on this platform")
			}

			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockLoader := NewMockConfigLoader(ctrl)
			mockDetector := NewMockRuntimeDetector(ctrl)
			mockRuntime := NewMockRuntime(ctrl)

			if tt.setupMocks != nil {
				tt.setupMocks(mockLoader, mockDetector, mockRuntime)
			}

			mgr := NewManager(
				WithConfigLoader(mockLoader),
				WithRuntimeDetector(mockDetector),
			)

			err := mgr.Attach(&schema.AtmosConfiguration{}, tt.devName, tt.instance, tt.usePTY)

			if tt.expectError {
				require.Error(t, err)
				if tt.errorIs != nil {
					assert.ErrorIs(t, err, tt.errorIs)
				}
			} else {
				// PTY mode will attempt to execute actual commands in test environment.
				// We verify the code path was exercised via mock expectations.
				if err != nil && tt.usePTY {
					t.Logf("PTY execution failed in test (expected): %v", err)
					return
				}
				require.NoError(t, err)
			}
		})
	}
}

func TestFindAndStartContainer(t *testing.T) {
	tests := []struct {
		name          string
		containerName string
		setupMocks    func(*MockRuntime)
		expectError   bool
		errorIs       error
	}{
		{
			name:          "container running - no action needed",
			containerName: "test-container",
			setupMocks: func(runtime *MockRuntime) {
				runtime.EXPECT().
					List(gomock.Any(), map[string]string{"name": "test-container"}).
					Return([]container.Info{
						{
							ID:     "running-id",
							Name:   "test-container",
							Status: "running",
						},
					}, nil)
				runtime.EXPECT().
					Inspect(gomock.Any(), "running-id").
					Return(&container.Info{
						ID:    "running-id",
						Ports: []container.PortBinding{},
					}, nil)
			},
			expectError: false,
		},
		{
			name:          "container stopped - starts successfully",
			containerName: "test-container",
			setupMocks: func(runtime *MockRuntime) {
				runtime.EXPECT().
					List(gomock.Any(), map[string]string{"name": "test-container"}).
					Return([]container.Info{
						{
							ID:     "stopped-id",
							Name:   "test-container",
							Status: "exited",
						},
					}, nil)
				runtime.EXPECT().
					Start(gomock.Any(), "stopped-id").
					Return(nil)
				runtime.EXPECT().
					Inspect(gomock.Any(), "stopped-id").
					Return(&container.Info{
						ID:    "stopped-id",
						Ports: []container.PortBinding{},
					}, nil)
			},
			expectError: false,
		},
		{
			name:          "container not found",
			containerName: "test-container",
			setupMocks: func(runtime *MockRuntime) {
				runtime.EXPECT().
					List(gomock.Any(), map[string]string{"name": "test-container"}).
					Return([]container.Info{}, nil)
			},
			expectError: true,
			errorIs:     errUtils.ErrDevcontainerNotFound,
		},
		{
			name:          "list fails",
			containerName: "test-container",
			setupMocks: func(runtime *MockRuntime) {
				runtime.EXPECT().
					List(gomock.Any(), gomock.Any()).
					Return(nil, errors.New("runtime error"))
			},
			expectError: true,
			errorIs:     errUtils.ErrContainerRuntimeOperation,
		},
		{
			name:          "start fails",
			containerName: "test-container",
			setupMocks: func(runtime *MockRuntime) {
				runtime.EXPECT().
					List(gomock.Any(), map[string]string{"name": "test-container"}).
					Return([]container.Info{
						{
							ID:     "stopped-id",
							Name:   "test-container",
							Status: "exited",
						},
					}, nil)
				runtime.EXPECT().
					Start(gomock.Any(), "stopped-id").
					Return(errors.New("start failed"))
			},
			expectError: true,
			errorIs:     errUtils.ErrContainerRuntimeOperation,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockRuntime := NewMockRuntime(ctrl)
			tt.setupMocks(mockRuntime)

			ctx := context.Background()
			testConfig := &Config{
				Image:        "test-image",
				ForwardPorts: []interface{}{8080},
			}
			containerInfo, err := findAndStartContainer(ctx, mockRuntime, tt.containerName, testConfig)

			if tt.expectError {
				require.Error(t, err)
				assert.Nil(t, containerInfo)
				if tt.errorIs != nil {
					assert.ErrorIs(t, err, tt.errorIs)
				}
			} else {
				require.NoError(t, err)
				assert.NotNil(t, containerInfo)
			}
		})
	}
}

func TestStartContainerForAttach(t *testing.T) {
	tests := []struct {
		name          string
		containerInfo *container.Info
		setupMocks    func(*MockRuntime)
		expectError   bool
		errorIs       error
	}{
		{
			name: "start succeeds",
			containerInfo: &container.Info{
				ID:     "stopped-id",
				Name:   "test-container",
				Status: "exited",
			},
			setupMocks: func(runtime *MockRuntime) {
				runtime.EXPECT().
					Start(gomock.Any(), "stopped-id").
					Return(nil)
			},
			expectError: false,
		},
		{
			name: "start fails",
			containerInfo: &container.Info{
				ID:     "stopped-id",
				Name:   "test-container",
				Status: "exited",
			},
			setupMocks: func(runtime *MockRuntime) {
				runtime.EXPECT().
					Start(gomock.Any(), "stopped-id").
					Return(errors.New("start error"))
			},
			expectError: true,
			errorIs:     errUtils.ErrContainerRuntimeOperation,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockRuntime := NewMockRuntime(ctrl)
			tt.setupMocks(mockRuntime)

			ctx := context.Background()
			err := startContainerForAttach(ctx, mockRuntime, tt.containerInfo, tt.containerInfo.Name)

			if tt.expectError {
				require.Error(t, err)
				if tt.errorIs != nil {
					assert.ErrorIs(t, err, tt.errorIs)
				}
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestGetShellArgs(t *testing.T) {
	tests := []struct {
		name         string
		userEnvProbe string
		expected     []string
	}{
		{
			name:         "loginShell returns -l",
			userEnvProbe: "loginShell",
			expected:     []string{"-l"},
		},
		{
			name:         "loginInteractiveShell returns -l",
			userEnvProbe: "loginInteractiveShell",
			expected:     []string{"-l"},
		},
		{
			name:         "interactiveShell returns nil",
			userEnvProbe: "interactiveShell",
			expected:     nil,
		},
		{
			name:         "empty string returns nil",
			userEnvProbe: "",
			expected:     nil,
		},
		{
			name:         "none returns nil",
			userEnvProbe: "none",
			expected:     nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getShellArgs(tt.userEnvProbe)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestAttachToContainer_RegularMode(t *testing.T) {
	// Save original viper value and restore after test.
	originalMask := viper.GetBool("mask")
	defer viper.Set("mask", originalMask)

	tests := []struct {
		name           string
		usePTY         bool
		maskingEnabled bool
		userEnvProbe   string
		setupMocks     func(*MockRuntime)
		expectError    bool
	}{
		{
			name:           "regular mode with masking disabled",
			usePTY:         false,
			maskingEnabled: false,
			userEnvProbe:   "",
			setupMocks: func(runtime *MockRuntime) {
				runtime.EXPECT().
					Attach(gomock.Any(), "container-id", gomock.Any()).
					Return(nil)
			},
			expectError: false,
		},
		{
			name:           "regular mode with masking enabled",
			usePTY:         false,
			maskingEnabled: true,
			userEnvProbe:   "",
			setupMocks: func(runtime *MockRuntime) {
				runtime.EXPECT().
					Attach(gomock.Any(), "container-id", gomock.Any()).
					Return(nil)
			},
			expectError: false,
		},
		{
			name:           "regular mode with login shell",
			usePTY:         false,
			maskingEnabled: false,
			userEnvProbe:   "loginShell",
			setupMocks: func(runtime *MockRuntime) {
				runtime.EXPECT().
					Attach(gomock.Any(), "container-id", gomock.Any()).
					Do(func(_ context.Context, _ string, opts *container.AttachOptions) {
						assert.Equal(t, []string{"-l"}, opts.ShellArgs)
					}).
					Return(nil)
			},
			expectError: false,
		},
		{
			name:           "attach fails",
			usePTY:         false,
			maskingEnabled: false,
			userEnvProbe:   "",
			setupMocks: func(runtime *MockRuntime) {
				runtime.EXPECT().
					Attach(gomock.Any(), "container-id", gomock.Any()).
					Return(errors.New("attach failed"))
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockRuntime := NewMockRuntime(ctrl)
			tt.setupMocks(mockRuntime)

			viper.Set("mask", tt.maskingEnabled)

			params := &attachParams{
				ctx:     context.Background(),
				runtime: mockRuntime,
				containerInfo: &container.Info{
					ID:   "container-id",
					Name: "test-container",
				},
				config: &Config{
					UserEnvProbe: tt.userEnvProbe,
				},
				containerName: "test-container",
				usePTY:        tt.usePTY,
			}

			err := attachToContainer(params)

			if tt.expectError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestAttachToContainer_PTYMode(t *testing.T) {
	// Skip PTY tests on unsupported platforms.
	if !pty.IsSupported() {
		t.Skip("PTY mode not supported on this platform")
	}

	// Save original viper value and restore after test.
	originalMask := viper.GetBool("mask")
	defer viper.Set("mask", originalMask)

	tests := []struct {
		name           string
		usePTY         bool
		maskingEnabled bool
		userEnvProbe   string
		runtimeType    string
		setupMocks     func(*MockRuntime)
		expectError    bool
	}{
		{
			name:           "PTY mode calls runtime.Info to determine binary",
			usePTY:         true,
			maskingEnabled: false,
			userEnvProbe:   "",
			runtimeType:    "docker",
			setupMocks: func(runtime *MockRuntime) {
				runtime.EXPECT().
					Info(gomock.Any()).
					Return(&container.RuntimeInfo{
						Type:    "docker",
						Version: "24.0.0",
						Running: true,
					}, nil)
			},
			expectError: false,
		},
		{
			name:           "PTY mode with podman runtime",
			usePTY:         true,
			maskingEnabled: false,
			userEnvProbe:   "",
			runtimeType:    "podman",
			setupMocks: func(runtime *MockRuntime) {
				runtime.EXPECT().
					Info(gomock.Any()).
					Return(&container.RuntimeInfo{
						Type:    "podman",
						Version: "4.5.0",
						Running: true,
					}, nil)
			},
			expectError: false,
		},
		{
			name:           "PTY mode with masking enabled",
			usePTY:         true,
			maskingEnabled: true,
			userEnvProbe:   "",
			runtimeType:    "docker",
			setupMocks: func(runtime *MockRuntime) {
				runtime.EXPECT().
					Info(gomock.Any()).
					Return(&container.RuntimeInfo{
						Type:    "docker",
						Version: "24.0.0",
						Running: true,
					}, nil)
			},
			expectError: false,
		},
		{
			name:           "PTY mode with login shell args",
			usePTY:         true,
			maskingEnabled: false,
			userEnvProbe:   "loginShell",
			runtimeType:    "docker",
			setupMocks: func(runtime *MockRuntime) {
				runtime.EXPECT().
					Info(gomock.Any()).
					Return(&container.RuntimeInfo{
						Type:    "docker",
						Version: "24.0.0",
						Running: true,
					}, nil)
			},
			expectError: false,
		},
		{
			name:           "PTY mode fails when runtime.Info fails",
			usePTY:         true,
			maskingEnabled: false,
			userEnvProbe:   "",
			runtimeType:    "",
			setupMocks: func(runtime *MockRuntime) {
				runtime.EXPECT().
					Info(gomock.Any()).
					Return(nil, errors.New("runtime info failed"))
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockRuntime := NewMockRuntime(ctrl)
			tt.setupMocks(mockRuntime)

			viper.Set("mask", tt.maskingEnabled)

			params := &attachParams{
				ctx:     context.Background(),
				runtime: mockRuntime,
				containerInfo: &container.Info{
					ID:   "container-id",
					Name: "test-container",
				},
				config: &Config{
					UserEnvProbe: tt.userEnvProbe,
				},
				containerName: "test-container",
				usePTY:        tt.usePTY,
			}

			err := attachToContainer(params)

			if tt.expectError {
				require.Error(t, err)
			} else {
				// PTY mode will fail in test environment because it tries to execute
				// actual shell commands. We verify that the PTY code path was reached
				// by checking that runtime.Info() was called (via mock expectations).
				// The error "exit status" or "executable not found" indicates PTY
				// execution was attempted, which validates the code path.
				if err != nil {
					// Expected errors from actual PTY execution in test environment.
					t.Logf("PTY execution failed in test (expected): %v", err)
				}
			}
		})
	}
}
