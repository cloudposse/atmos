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
)

func TestManager_Exec(t *testing.T) {
	tests := []struct {
		name          string
		params        ExecParams
		setupMocks    func(*MockConfigLoader, *MockRuntimeDetector, *MockRuntime)
		expectError   bool
		errorIs       error
		errorContains string
	}{
		{
			name: "exec command in running container successfully",
			params: ExecParams{
				Name:        "test",
				Instance:    "default",
				Interactive: false,
				UsePTY:      false,
				Command:     []string{"ls", "-la"},
			},
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
					Exec(gomock.Any(), "running-id", []string{"ls", "-la"}, gomock.Any()).
					Return(nil)
			},
			expectError: false,
		},
		{
			name: "exec in stopped container - starts and executes",
			params: ExecParams{
				Name:        "test",
				Instance:    "default",
				Interactive: false,
				UsePTY:      false,
				Command:     []string{"pwd"},
			},
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
					Exec(gomock.Any(), "stopped-id", []string{"pwd"}, gomock.Any()).
					Return(nil)
			},
			expectError: false,
		},
		{
			name: "interactive exec",
			params: ExecParams{
				Name:        "test",
				Instance:    "default",
				Interactive: true,
				UsePTY:      false,
				Command:     []string{"bash"},
			},
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
					Exec(gomock.Any(), "running-id", []string{"bash"}, gomock.Any()).
					Do(func(_ context.Context, _ string, _ []string, opts *container.ExecOptions) {
						assert.True(t, opts.Tty)
						assert.True(t, opts.AttachStdin)
						assert.True(t, opts.AttachStdout)
						assert.True(t, opts.AttachStderr)
					}).
					Return(nil)
			},
			expectError: false,
		},
		{
			name: "config load fails",
			params: ExecParams{
				Name:     "test",
				Instance: "default",
				Command:  []string{"ls"},
			},
			setupMocks: func(loader *MockConfigLoader, detector *MockRuntimeDetector, runtime *MockRuntime) {
				loader.EXPECT().
					LoadConfig(gomock.Any(), "test").
					Return(nil, nil, errors.New("config not found"))
			},
			expectError:   true,
			errorContains: "config not found",
		},
		{
			name: "runtime detection fails",
			params: ExecParams{
				Name:     "test",
				Instance: "default",
				Command:  []string{"ls"},
			},
			setupMocks: func(loader *MockConfigLoader, detector *MockRuntimeDetector, runtime *MockRuntime) {
				loader.EXPECT().
					LoadConfig(gomock.Any(), "test").
					Return(&Config{Name: "test", Image: "ubuntu:22.04"}, &Settings{}, nil)
				detector.EXPECT().
					DetectRuntime("").
					Return(nil, errors.New("docker not found"))
			},
			expectError:   true,
			errorContains: "docker not found",
		},
		{
			name: "container not found",
			params: ExecParams{
				Name:     "test",
				Instance: "default",
				Command:  []string{"ls"},
			},
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
			expectError:   true,
			errorIs:       errUtils.ErrDevcontainerNotFound,
			errorContains: "not found",
		},
		{
			name: "container list fails",
			params: ExecParams{
				Name:     "test",
				Instance: "default",
				Command:  []string{"ls"},
			},
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
			expectError:   true,
			errorIs:       errUtils.ErrContainerRuntimeOperation,
			errorContains: "failed to list containers",
		},
		{
			name: "container start fails",
			params: ExecParams{
				Name:     "test",
				Instance: "default",
				Command:  []string{"ls"},
			},
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
			expectError:   true,
			errorIs:       errUtils.ErrContainerRuntimeOperation,
			errorContains: "failed to start container",
		},
		{
			name: "exec fails",
			params: ExecParams{
				Name:     "test",
				Instance: "default",
				Command:  []string{"ls"},
			},
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
					Exec(gomock.Any(), "running-id", []string{"ls"}, gomock.Any()).
					Return(errors.New("exec failed"))
			},
			expectError:   true,
			errorContains: "exec failed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
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

			err := mgr.Exec(&schema.AtmosConfiguration{}, tt.params)

			if tt.expectError {
				require.Error(t, err)
				if tt.errorIs != nil {
					assert.ErrorIs(t, err, tt.errorIs)
				}
				if tt.errorContains != "" {
					assert.Contains(t, err.Error(), tt.errorContains)
				}
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestExecInContainer_RegularMode(t *testing.T) {
	// Save original viper value and restore after test.
	originalMask := viper.GetBool("mask")
	defer viper.Set("mask", originalMask)

	tests := []struct {
		name           string
		interactive    bool
		usePTY         bool
		maskingEnabled bool
		command        []string
		setupMocks     func(*MockRuntime)
		expectError    bool
	}{
		{
			name:           "non-interactive exec",
			interactive:    false,
			usePTY:         false,
			maskingEnabled: false,
			command:        []string{"ls", "-la"},
			setupMocks: func(runtime *MockRuntime) {
				runtime.EXPECT().
					Exec(gomock.Any(), "container-id", []string{"ls", "-la"}, gomock.Any()).
					Do(func(_ context.Context, _ string, _ []string, opts *container.ExecOptions) {
						assert.False(t, opts.Tty)
						assert.False(t, opts.AttachStdin)
						assert.True(t, opts.AttachStdout)
						assert.True(t, opts.AttachStderr)
					}).
					Return(nil)
			},
			expectError: false,
		},
		{
			name:           "interactive exec with TTY",
			interactive:    true,
			usePTY:         false,
			maskingEnabled: false,
			command:        []string{"bash"},
			setupMocks: func(runtime *MockRuntime) {
				runtime.EXPECT().
					Exec(gomock.Any(), "container-id", []string{"bash"}, gomock.Any()).
					Do(func(_ context.Context, _ string, _ []string, opts *container.ExecOptions) {
						assert.True(t, opts.Tty)
						assert.True(t, opts.AttachStdin)
						assert.True(t, opts.AttachStdout)
						assert.True(t, opts.AttachStderr)
					}).
					Return(nil)
			},
			expectError: false,
		},
		{
			name:           "interactive with masking enabled - logs warning",
			interactive:    true,
			usePTY:         false,
			maskingEnabled: true,
			command:        []string{"bash"},
			setupMocks: func(runtime *MockRuntime) {
				runtime.EXPECT().
					Exec(gomock.Any(), "container-id", []string{"bash"}, gomock.Any()).
					Return(nil)
			},
			expectError: false,
		},
		{
			name:           "exec fails",
			interactive:    false,
			usePTY:         false,
			maskingEnabled: false,
			command:        []string{"ls"},
			setupMocks: func(runtime *MockRuntime) {
				runtime.EXPECT().
					Exec(gomock.Any(), "container-id", []string{"ls"}, gomock.Any()).
					Return(errors.New("exec failed"))
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

			params := &execParams{
				ctx:         context.Background(),
				runtime:     mockRuntime,
				containerID: "container-id",
				interactive: tt.interactive,
				usePTY:      tt.usePTY,
				command:     tt.command,
			}

			err := execInContainer(params)

			if tt.expectError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestExecParams(t *testing.T) {
	t.Run("ExecParams struct fields", func(t *testing.T) {
		params := ExecParams{
			Name:        "test",
			Instance:    "default",
			Interactive: true,
			UsePTY:      false,
			Command:     []string{"ls", "-la"},
		}

		assert.Equal(t, "test", params.Name)
		assert.Equal(t, "default", params.Instance)
		assert.True(t, params.Interactive)
		assert.False(t, params.UsePTY)
		assert.Equal(t, []string{"ls", "-la"}, params.Command)
	})
}

func TestExecParamsInternal(t *testing.T) {
	t.Run("execParams struct fields", func(t *testing.T) {
		ctx := context.Background()
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockRuntime := NewMockRuntime(ctrl)

		params := &execParams{
			ctx:         ctx,
			runtime:     mockRuntime,
			containerID: "container-id",
			interactive: true,
			usePTY:      false,
			command:     []string{"bash"},
		}

		assert.NotNil(t, params.ctx)
		assert.Equal(t, mockRuntime, params.runtime)
		assert.Equal(t, "container-id", params.containerID)
		assert.True(t, params.interactive)
		assert.False(t, params.usePTY)
		assert.Equal(t, []string{"bash"}, params.command)
	})
}

func TestExecInContainer_WithMasking(t *testing.T) {
	// Save original viper value and restore after test.
	originalMask := viper.GetBool("mask")
	defer viper.Set("mask", originalMask)

	tests := []struct {
		name        string
		maskEnabled bool
	}{
		{
			name:        "masking enabled",
			maskEnabled: true,
		},
		{
			name:        "masking disabled",
			maskEnabled: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockRuntime := NewMockRuntime(ctrl)
			mockRuntime.EXPECT().
				Exec(gomock.Any(), "container-id", []string{"echo", "test"}, gomock.Any()).
				Return(nil)

			viper.Set("mask", tt.maskEnabled)

			params := &execParams{
				ctx:         context.Background(),
				runtime:     mockRuntime,
				containerID: "container-id",
				interactive: false,
				usePTY:      false,
				command:     []string{"echo", "test"},
			}

			err := execInContainer(params)
			require.NoError(t, err)
		})
	}
}

func TestExecInContainer_ExecOptionsConfiguration(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRuntime := NewMockRuntime(ctrl)

	// Capture the ExecOptions passed to the runtime.
	var capturedOpts *container.ExecOptions

	mockRuntime.EXPECT().
		Exec(gomock.Any(), "container-id", []string{"ls"}, gomock.Any()).
		Do(func(_ context.Context, _ string, _ []string, opts *container.ExecOptions) {
			capturedOpts = opts
		}).
		Return(nil)

	params := &execParams{
		ctx:         context.Background(),
		runtime:     mockRuntime,
		containerID: "container-id",
		interactive: true,
		usePTY:      false,
		command:     []string{"ls"},
	}

	err := execInContainer(params)
	require.NoError(t, err)

	// Verify ExecOptions were configured correctly.
	require.NotNil(t, capturedOpts)
	assert.True(t, capturedOpts.Tty, "Tty should be true for interactive mode")
	assert.True(t, capturedOpts.AttachStdin, "AttachStdin should be true for interactive mode")
	assert.True(t, capturedOpts.AttachStdout, "AttachStdout should always be true")
	assert.True(t, capturedOpts.AttachStderr, "AttachStderr should always be true")
}
