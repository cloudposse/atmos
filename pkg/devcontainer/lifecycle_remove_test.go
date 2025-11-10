package devcontainer

import (
	"errors"
	"testing"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/container"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestManager_Remove(t *testing.T) {
	tests := []struct {
		name          string
		devName       string
		instance      string
		force         bool
		setupMocks    func(*MockConfigLoader, *MockRuntimeDetector, *MockRuntime)
		expectError   bool
		errorIs       error
		errorContains string
	}{
		{
			name:     "remove stopped container",
			devName:  "test",
			instance: "default",
			force:    false,
			setupMocks: func(loader *MockConfigLoader, detector *MockRuntimeDetector, runtime *MockRuntime) {
				loader.EXPECT().
					LoadConfig(gomock.Any(), "test").
					Return(&Config{}, &Settings{}, nil)
				detector.EXPECT().
					DetectRuntime("").
					Return(runtime, nil)
				runtime.EXPECT().
					Inspect(gomock.Any(), "atmos-devcontainer-test-default").
					Return(&container.Info{
						ID:     "stopped-id",
						Name:   "atmos-devcontainer-test-default",
						Status: "exited",
					}, nil)
				runtime.EXPECT().
					Remove(gomock.Any(), "stopped-id", true).
					Return(nil)
			},
			expectError: false,
		},
		{
			name:     "remove running container with force",
			devName:  "test",
			instance: "default",
			force:    true,
			setupMocks: func(loader *MockConfigLoader, detector *MockRuntimeDetector, runtime *MockRuntime) {
				loader.EXPECT().
					LoadConfig(gomock.Any(), "test").
					Return(&Config{}, &Settings{}, nil)
				detector.EXPECT().
					DetectRuntime("").
					Return(runtime, nil)
				runtime.EXPECT().
					Inspect(gomock.Any(), "atmos-devcontainer-test-default").
					Return(&container.Info{
						ID:     "running-id",
						Name:   "atmos-devcontainer-test-default",
						Status: "running",
					}, nil)
				runtime.EXPECT().
					Stop(gomock.Any(), "running-id", defaultContainerStopTimeout).
					Return(nil)
				runtime.EXPECT().
					Remove(gomock.Any(), "running-id", true).
					Return(nil)
			},
			expectError: false,
		},
		{
			name:     "remove running container without force - fails",
			devName:  "test",
			instance: "default",
			force:    false,
			setupMocks: func(loader *MockConfigLoader, detector *MockRuntimeDetector, runtime *MockRuntime) {
				loader.EXPECT().
					LoadConfig(gomock.Any(), "test").
					Return(&Config{}, &Settings{}, nil)
				detector.EXPECT().
					DetectRuntime("").
					Return(runtime, nil)
				runtime.EXPECT().
					Inspect(gomock.Any(), "atmos-devcontainer-test-default").
					Return(&container.Info{
						ID:     "running-id",
						Name:   "atmos-devcontainer-test-default",
						Status: "running",
					}, nil)
			},
			expectError:   true,
			errorIs:       errUtils.ErrContainerRunning,
			errorContains: "use --force to remove",
		},
		{
			name:     "container doesn't exist - idempotent success",
			devName:  "test",
			instance: "default",
			force:    false,
			setupMocks: func(loader *MockConfigLoader, detector *MockRuntimeDetector, runtime *MockRuntime) {
				loader.EXPECT().
					LoadConfig(gomock.Any(), "test").
					Return(&Config{}, &Settings{}, nil)
				detector.EXPECT().
					DetectRuntime("").
					Return(runtime, nil)
				runtime.EXPECT().
					Inspect(gomock.Any(), "atmos-devcontainer-test-default").
					Return(nil, errUtils.ErrContainerNotFound)
			},
			expectError: false, // Idempotent - no error if container doesn't exist.
		},
		{
			name:     "config load fails",
			devName:  "test",
			instance: "default",
			force:    false,
			setupMocks: func(loader *MockConfigLoader, detector *MockRuntimeDetector, runtime *MockRuntime) {
				loader.EXPECT().
					LoadConfig(gomock.Any(), "test").
					Return(nil, nil, errors.New("config error"))
			},
			expectError:   true,
			errorContains: "config error",
		},
		{
			name:     "runtime detection fails",
			devName:  "test",
			instance: "default",
			force:    false,
			setupMocks: func(loader *MockConfigLoader, detector *MockRuntimeDetector, runtime *MockRuntime) {
				loader.EXPECT().
					LoadConfig(gomock.Any(), "test").
					Return(&Config{}, &Settings{}, nil)
				detector.EXPECT().
					DetectRuntime("").
					Return(nil, errors.New("runtime not found"))
			},
			expectError:   true,
			errorIs:       errUtils.ErrContainerRuntimeOperation,
			errorContains: "failed to initialize container runtime",
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

			err := mgr.Remove(&schema.AtmosConfiguration{}, tt.devName, tt.instance, tt.force)

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
