package devcontainer

import (
	"errors"
	"testing"
	"time"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/container"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestManager_Stop(t *testing.T) {
	tests := []struct {
		name        string
		devName     string
		instance    string
		timeout     int
		setupMocks  func(*MockConfigLoader, *MockRuntimeDetector, *MockRuntime)
		expectError bool
		errorIs     error
	}{
		{
			name:     "stop running container successfully",
			devName:  "test",
			instance: "default",
			timeout:  10,
			setupMocks: func(loader *MockConfigLoader, detector *MockRuntimeDetector, runtime *MockRuntime) {
				loader.EXPECT().
					LoadConfig(gomock.Any(), "test").
					Return(&Config{}, &Settings{}, nil)
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
					Stop(gomock.Any(), "running-id", 10*time.Second).
					Return(nil)
			},
			expectError: false,
		},
		{
			name:     "container already stopped - no-op",
			devName:  "test",
			instance: "default",
			timeout:  10,
			setupMocks: func(loader *MockConfigLoader, detector *MockRuntimeDetector, runtime *MockRuntime) {
				loader.EXPECT().
					LoadConfig(gomock.Any(), "test").
					Return(&Config{}, &Settings{}, nil)
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
			},
			expectError: false,
		},
		{
			name:     "container not found",
			devName:  "test",
			instance: "default",
			timeout:  10,
			setupMocks: func(loader *MockConfigLoader, detector *MockRuntimeDetector, runtime *MockRuntime) {
				loader.EXPECT().
					LoadConfig(gomock.Any(), "test").
					Return(&Config{}, &Settings{}, nil)
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
			name:     "config load fails",
			devName:  "test",
			instance: "default",
			timeout:  10,
			setupMocks: func(loader *MockConfigLoader, detector *MockRuntimeDetector, runtime *MockRuntime) {
				loader.EXPECT().
					LoadConfig(gomock.Any(), "test").
					Return(nil, nil, errors.New("config error"))
			},
			expectError: true,
		},
		{
			name:     "runtime detection fails",
			devName:  "test",
			instance: "default",
			timeout:  10,
			setupMocks: func(loader *MockConfigLoader, detector *MockRuntimeDetector, runtime *MockRuntime) {
				loader.EXPECT().
					LoadConfig(gomock.Any(), "test").
					Return(&Config{}, &Settings{}, nil)
				detector.EXPECT().
					DetectRuntime("").
					Return(nil, errors.New("runtime not found"))
			},
			expectError: true,
		},
		{
			name:     "stop fails - runtime error",
			devName:  "test",
			instance: "default",
			timeout:  10,
			setupMocks: func(loader *MockConfigLoader, detector *MockRuntimeDetector, runtime *MockRuntime) {
				loader.EXPECT().
					LoadConfig(gomock.Any(), "test").
					Return(&Config{}, &Settings{}, nil)
				detector.EXPECT().
					DetectRuntime("").
					Return(runtime, nil)
				runtime.EXPECT().
					List(gomock.Any(), gomock.Any()).
					Return([]container.Info{
						{ID: "id", Name: "test", Status: "running"},
					}, nil)
				runtime.EXPECT().
					Stop(gomock.Any(), "id", 10*time.Second).
					Return(errors.New("stop failed"))
			},
			expectError: true,
			errorIs:     errUtils.ErrContainerRuntimeOperation,
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

			err := mgr.Stop(&schema.AtmosConfiguration{}, tt.devName, tt.instance, tt.timeout)

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
