package devcontainer

import (
	"context"
	"errors"
	"testing"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/container"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestManager_Start(t *testing.T) {
	tests := []struct {
		name         string
		devName      string
		instance     string
		identityName string
		setupMocks   func(*MockConfigLoader, *MockIdentityManager, *MockRuntimeDetector, *MockRuntime)
		expectError  bool
		errorIs      error
	}{
		{
			name:     "start new container without identity",
			devName:  "test",
			instance: "default",
			setupMocks: func(loader *MockConfigLoader, identity *MockIdentityManager, detector *MockRuntimeDetector, runtime *MockRuntime) {
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
					Return([]container.Info{}, nil)
				// createAndStartNewContainer calls
				runtime.EXPECT().
					Create(gomock.Any(), gomock.Any()).
					Return("container-id", nil)
				runtime.EXPECT().
					Start(gomock.Any(), "container-id").
					Return(nil)
			},
			expectError: false,
		},
		{
			name:         "start with identity injection",
			devName:      "test",
			instance:     "default",
			identityName: "dev-identity",
			setupMocks: func(loader *MockConfigLoader, identity *MockIdentityManager, detector *MockRuntimeDetector, runtime *MockRuntime) {
				config := &Config{
					Name:  "test",
					Image: "ubuntu:22.04",
				}
				loader.EXPECT().
					LoadConfig(gomock.Any(), "test").
					Return(config, &Settings{}, nil)
				identity.EXPECT().
					InjectIdentityEnvironment(gomock.Any(), config, "dev-identity").
					Return(nil)
				detector.EXPECT().
					DetectRuntime("").
					Return(runtime, nil)
				runtime.EXPECT().
					List(gomock.Any(), gomock.Any()).
					Return([]container.Info{}, nil)
				runtime.EXPECT().
					Create(gomock.Any(), gomock.Any()).
					Return("container-id", nil)
				runtime.EXPECT().
					Start(gomock.Any(), "container-id").
					Return(nil)
			},
			expectError: false,
		},
		{
			name:     "start existing stopped container",
			devName:  "test",
			instance: "default",
			setupMocks: func(loader *MockConfigLoader, identity *MockIdentityManager, detector *MockRuntimeDetector, runtime *MockRuntime) {
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
							ID:     "existing-id",
							Name:   "atmos-devcontainer.test.default",
							Status: "exited",
						},
					}, nil)
				runtime.EXPECT().
					Start(gomock.Any(), "existing-id").
					Return(nil)
			},
			expectError: false,
		},
		{
			name:     "container already running - no-op",
			devName:  "test",
			instance: "default",
			setupMocks: func(loader *MockConfigLoader, identity *MockIdentityManager, detector *MockRuntimeDetector, runtime *MockRuntime) {
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
			},
			expectError: false,
		},
		{
			name:     "config load fails",
			devName:  "test",
			instance: "default",
			setupMocks: func(loader *MockConfigLoader, identity *MockIdentityManager, detector *MockRuntimeDetector, runtime *MockRuntime) {
				loader.EXPECT().
					LoadConfig(gomock.Any(), "test").
					Return(nil, nil, errors.New("config not found"))
			},
			expectError: true,
		},
		{
			name:         "identity injection fails",
			devName:      "test",
			instance:     "default",
			identityName: "bad-identity",
			setupMocks: func(loader *MockConfigLoader, identity *MockIdentityManager, detector *MockRuntimeDetector, runtime *MockRuntime) {
				config := &Config{Name: "test", Image: "ubuntu:22.04"}
				loader.EXPECT().
					LoadConfig(gomock.Any(), "test").
					Return(config, &Settings{}, nil)
				identity.EXPECT().
					InjectIdentityEnvironment(gomock.Any(), config, "bad-identity").
					Return(errors.New("identity not found"))
			},
			expectError: true,
		},
		{
			name:     "runtime detection fails",
			devName:  "test",
			instance: "default",
			setupMocks: func(loader *MockConfigLoader, identity *MockIdentityManager, detector *MockRuntimeDetector, runtime *MockRuntime) {
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
			name:     "container list fails",
			devName:  "test",
			instance: "default",
			setupMocks: func(loader *MockConfigLoader, identity *MockIdentityManager, detector *MockRuntimeDetector, runtime *MockRuntime) {
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
			name:     "container create fails",
			devName:  "test",
			instance: "default",
			setupMocks: func(loader *MockConfigLoader, identity *MockIdentityManager, detector *MockRuntimeDetector, runtime *MockRuntime) {
				loader.EXPECT().
					LoadConfig(gomock.Any(), "test").
					Return(&Config{Name: "test", Image: "ubuntu:22.04"}, &Settings{}, nil)
				detector.EXPECT().
					DetectRuntime("").
					Return(runtime, nil)
				runtime.EXPECT().
					List(gomock.Any(), gomock.Any()).
					Return([]container.Info{}, nil)
				runtime.EXPECT().
					Create(gomock.Any(), gomock.Any()).
					Return("", errors.New("create failed"))
			},
			expectError: true,
			errorIs:     errUtils.ErrContainerRuntimeOperation,
		},
		{
			name:     "container start fails",
			devName:  "test",
			instance: "default",
			setupMocks: func(loader *MockConfigLoader, identity *MockIdentityManager, detector *MockRuntimeDetector, runtime *MockRuntime) {
				loader.EXPECT().
					LoadConfig(gomock.Any(), "test").
					Return(&Config{Name: "test", Image: "ubuntu:22.04"}, &Settings{}, nil)
				detector.EXPECT().
					DetectRuntime("").
					Return(runtime, nil)
				runtime.EXPECT().
					List(gomock.Any(), gomock.Any()).
					Return([]container.Info{}, nil)
				runtime.EXPECT().
					Create(gomock.Any(), gomock.Any()).
					Return("new-id", nil)
				runtime.EXPECT().
					Start(gomock.Any(), "new-id").
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

			mockLoader := NewMockConfigLoader(ctrl)
			mockIdentity := NewMockIdentityManager(ctrl)
			mockDetector := NewMockRuntimeDetector(ctrl)
			mockRuntime := NewMockRuntime(ctrl)

			if tt.setupMocks != nil {
				tt.setupMocks(mockLoader, mockIdentity, mockDetector, mockRuntime)
			}

			mgr := NewManager(
				WithConfigLoader(mockLoader),
				WithIdentityManager(mockIdentity),
				WithRuntimeDetector(mockDetector),
			)

			err := mgr.Start(&schema.AtmosConfiguration{}, tt.devName, tt.instance, tt.identityName)

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

func TestStartExistingContainer(t *testing.T) {
	tests := []struct {
		name          string
		containerInfo *container.Info
		setupMocks    func(*MockRuntime)
		expectError   bool
	}{
		{
			name: "container already running - no-op",
			containerInfo: &container.Info{
				ID:     "running-id",
				Name:   "test-container",
				Status: "running",
			},
			setupMocks:  func(runtime *MockRuntime) {},
			expectError: false,
		},
		{
			name: "start stopped container successfully",
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
			name: "start fails - runtime error",
			containerInfo: &container.Info{
				ID:     "stopped-id",
				Name:   "test-container",
				Status: "exited",
			},
			setupMocks: func(runtime *MockRuntime) {
				runtime.EXPECT().
					Start(gomock.Any(), "stopped-id").
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
			tt.setupMocks(mockRuntime)

			ctx := context.Background()
			err := startExistingContainer(ctx, mockRuntime, tt.containerInfo, tt.containerInfo.Name)

			if tt.expectError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}
