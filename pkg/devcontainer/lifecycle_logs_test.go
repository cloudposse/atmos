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

func TestManager_Logs(t *testing.T) {
	tests := []struct {
		name          string
		devName       string
		instance      string
		follow        bool
		tail          string
		setupMocks    func(*MockConfigLoader, *MockRuntimeDetector, *MockRuntime)
		expectError   bool
		errorIs       error
		errorContains string
	}{
		{
			name:     "show logs for running container",
			devName:  "test",
			instance: "default",
			follow:   false,
			tail:     "100",
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
						ID:     "container-id",
						Name:   "atmos-devcontainer-test-default",
						Status: "running",
					}, nil)
				runtime.EXPECT().
					Logs(gomock.Any(), "atmos-devcontainer-test-default", false, "100", nil, nil).
					Return(nil)
			},
			expectError: false,
		},
		{
			name:     "follow logs from container",
			devName:  "test",
			instance: "default",
			follow:   true,
			tail:     "all",
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
						ID:     "container-id",
						Name:   "atmos-devcontainer-test-default",
						Status: "running",
					}, nil)
				runtime.EXPECT().
					Logs(gomock.Any(), "atmos-devcontainer-test-default", true, "all", nil, nil).
					Return(nil)
			},
			expectError: false,
		},
		{
			name:     "container not found",
			devName:  "test",
			instance: "default",
			follow:   false,
			tail:     "100",
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
			expectError:   true,
			errorIs:       errUtils.ErrContainerNotFound,
			errorContains: "container atmos-devcontainer-test-default not found",
		},
		{
			name:     "config load fails",
			devName:  "test",
			instance: "default",
			follow:   false,
			tail:     "100",
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
			follow:   false,
			tail:     "100",
			setupMocks: func(loader *MockConfigLoader, detector *MockRuntimeDetector, runtime *MockRuntime) {
				loader.EXPECT().
					LoadConfig(gomock.Any(), "test").
					Return(&Config{}, &Settings{}, nil)
				detector.EXPECT().
					DetectRuntime("").
					Return(nil, errors.New("runtime not found"))
			},
			expectError:   true,
			errorContains: "runtime not found",
		},
		{
			name:     "logs operation fails",
			devName:  "test",
			instance: "default",
			follow:   false,
			tail:     "100",
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
						ID:     "container-id",
						Name:   "atmos-devcontainer-test-default",
						Status: "running",
					}, nil)
				runtime.EXPECT().
					Logs(gomock.Any(), "atmos-devcontainer-test-default", false, "100", nil, nil).
					Return(errors.New("logs failed"))
			},
			expectError:   true,
			errorContains: "logs failed",
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

			err := mgr.Logs(&schema.AtmosConfiguration{}, tt.devName, tt.instance, tt.follow, tt.tail)

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
