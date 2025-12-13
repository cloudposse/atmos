//nolint:dupl // Table-driven test boilerplate - structural similarity across lifecycle tests is intentional.
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

func TestManager_Rebuild(t *testing.T) {
	tests := []struct {
		name         string
		devName      string
		instance     string
		identityName string
		noPull       bool
		setupMocks   func(*MockConfigLoader, *MockIdentityManager, *MockRuntimeDetector, *MockRuntime)
		expectError  bool
		errorIs      error
	}{
		{
			name:     "rebuild container successfully",
			devName:  "test",
			instance: "default",
			noPull:   false,
			setupMocks: func(loader *MockConfigLoader, identity *MockIdentityManager, detector *MockRuntimeDetector, runtime *MockRuntime) {
				config := &Config{
					Name:  "test",
					Image: "ubuntu:22.04",
				}
				loader.EXPECT().
					LoadConfig(gomock.Any(), "test").
					Return(config, &Settings{}, nil)
				detector.EXPECT().
					DetectRuntime("").
					Return(runtime, nil)
				// stopAndRemoveContainer checks if container exists
				runtime.EXPECT().
					Inspect(gomock.Any(), "atmos-devcontainer.test.default").
					Return(&container.Info{
						ID:     "old-id",
						Name:   "atmos-devcontainer.test.default",
						Status: "running",
					}, nil)
				// Stop the container
				runtime.EXPECT().
					Stop(gomock.Any(), "old-id", gomock.Any()).
					Return(nil)
				// Remove the container
				runtime.EXPECT().
					Remove(gomock.Any(), "old-id", true).
					Return(nil)
				// Pull image
				runtime.EXPECT().
					Pull(gomock.Any(), "ubuntu:22.04").
					Return(nil)
				// Create new container
				runtime.EXPECT().
					Create(gomock.Any(), gomock.Any()).
					Return("new-id", nil)
				// Start new container
				runtime.EXPECT().
					Start(gomock.Any(), "new-id").
					Return(nil)
				runtime.EXPECT().
					Inspect(gomock.Any(), "new-id").
					Return(&container.Info{
						ID:    "new-id",
						Ports: []container.PortBinding{},
					}, nil)
			},
			expectError: false,
		},
		{
			name:         "rebuild with identity injection",
			devName:      "test",
			instance:     "default",
			identityName: "dev-identity",
			noPull:       false,
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
					Inspect(gomock.Any(), "atmos-devcontainer.test.default").
					Return(nil, errors.New("not found"))
				runtime.EXPECT().
					Pull(gomock.Any(), "ubuntu:22.04").
					Return(nil)
				runtime.EXPECT().
					Create(gomock.Any(), gomock.Any()).
					Return("new-id", nil)
				runtime.EXPECT().
					Start(gomock.Any(), "new-id").
					Return(nil)
				runtime.EXPECT().
					Inspect(gomock.Any(), "new-id").
					Return(&container.Info{
						ID:    "new-id",
						Ports: []container.PortBinding{},
					}, nil)
			},
			expectError: false,
		},
		{
			name:     "rebuild with --no-pull flag",
			devName:  "test",
			instance: "default",
			noPull:   true,
			setupMocks: func(loader *MockConfigLoader, identity *MockIdentityManager, detector *MockRuntimeDetector, runtime *MockRuntime) {
				config := &Config{
					Name:  "test",
					Image: "ubuntu:22.04",
				}
				loader.EXPECT().
					LoadConfig(gomock.Any(), "test").
					Return(config, &Settings{}, nil)
				detector.EXPECT().
					DetectRuntime("").
					Return(runtime, nil)
				runtime.EXPECT().
					Inspect(gomock.Any(), "atmos-devcontainer.test.default").
					Return(nil, errors.New("not found"))
				// No Pull call expected when noPull=true
				runtime.EXPECT().
					Create(gomock.Any(), gomock.Any()).
					Return("new-id", nil)
				runtime.EXPECT().
					Start(gomock.Any(), "new-id").
					Return(nil)
				runtime.EXPECT().
					Inspect(gomock.Any(), "new-id").
					Return(&container.Info{
						ID:    "new-id",
						Ports: []container.PortBinding{},
					}, nil)
			},
			expectError: false,
		},
		{
			name:     "config load fails",
			devName:  "test",
			instance: "default",
			noPull:   false,
			setupMocks: func(loader *MockConfigLoader, identity *MockIdentityManager, detector *MockRuntimeDetector, runtime *MockRuntime) {
				loader.EXPECT().
					LoadConfig(gomock.Any(), "test").
					Return(nil, nil, errors.New("config error"))
			},
			expectError: true,
		},
		{
			name:         "identity injection fails",
			devName:      "test",
			instance:     "default",
			identityName: "bad-identity",
			noPull:       false,
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
			noPull:   false,
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
			name:     "stop container fails",
			devName:  "test",
			instance: "default",
			noPull:   false,
			setupMocks: func(loader *MockConfigLoader, identity *MockIdentityManager, detector *MockRuntimeDetector, runtime *MockRuntime) {
				config := &Config{Name: "test", Image: "ubuntu:22.04"}
				loader.EXPECT().
					LoadConfig(gomock.Any(), "test").
					Return(config, &Settings{}, nil)
				detector.EXPECT().
					DetectRuntime("").
					Return(runtime, nil)
				runtime.EXPECT().
					Inspect(gomock.Any(), "atmos-devcontainer.test.default").
					Return(&container.Info{
						ID:     "old-id",
						Name:   "atmos-devcontainer.test.default",
						Status: "running",
					}, nil)
				runtime.EXPECT().
					Stop(gomock.Any(), "old-id", gomock.Any()).
					Return(errors.New("stop failed"))
			},
			expectError: true,
			errorIs:     errUtils.ErrContainerRuntimeOperation,
		},
		{
			name:     "pull image fails",
			devName:  "test",
			instance: "default",
			noPull:   false,
			setupMocks: func(loader *MockConfigLoader, identity *MockIdentityManager, detector *MockRuntimeDetector, runtime *MockRuntime) {
				config := &Config{Name: "test", Image: "ubuntu:22.04"}
				loader.EXPECT().
					LoadConfig(gomock.Any(), "test").
					Return(config, &Settings{}, nil)
				detector.EXPECT().
					DetectRuntime("").
					Return(runtime, nil)
				runtime.EXPECT().
					Inspect(gomock.Any(), "atmos-devcontainer.test.default").
					Return(nil, errors.New("not found"))
				runtime.EXPECT().
					Pull(gomock.Any(), "ubuntu:22.04").
					Return(errors.New("pull failed"))
			},
			expectError: true,
			errorIs:     errUtils.ErrContainerRuntimeOperation,
		},
		{
			name:     "create container fails",
			devName:  "test",
			instance: "default",
			noPull:   true,
			setupMocks: func(loader *MockConfigLoader, identity *MockIdentityManager, detector *MockRuntimeDetector, runtime *MockRuntime) {
				config := &Config{Name: "test", Image: "ubuntu:22.04"}
				loader.EXPECT().
					LoadConfig(gomock.Any(), "test").
					Return(config, &Settings{}, nil)
				detector.EXPECT().
					DetectRuntime("").
					Return(runtime, nil)
				runtime.EXPECT().
					Inspect(gomock.Any(), "atmos-devcontainer.test.default").
					Return(nil, errors.New("not found"))
				runtime.EXPECT().
					Create(gomock.Any(), gomock.Any()).
					Return("", errors.New("create failed"))
			},
			expectError: true,
			errorIs:     errUtils.ErrContainerRuntimeOperation,
		},
		{
			name:     "start container fails",
			devName:  "test",
			instance: "default",
			noPull:   true,
			setupMocks: func(loader *MockConfigLoader, identity *MockIdentityManager, detector *MockRuntimeDetector, runtime *MockRuntime) {
				config := &Config{Name: "test", Image: "ubuntu:22.04"}
				loader.EXPECT().
					LoadConfig(gomock.Any(), "test").
					Return(config, &Settings{}, nil)
				detector.EXPECT().
					DetectRuntime("").
					Return(runtime, nil)
				runtime.EXPECT().
					Inspect(gomock.Any(), "atmos-devcontainer.test.default").
					Return(nil, errors.New("not found"))
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
		{
			name:     "remove fails after stop succeeds",
			devName:  "test",
			instance: "default",
			noPull:   false,
			setupMocks: func(loader *MockConfigLoader, identity *MockIdentityManager, detector *MockRuntimeDetector, runtime *MockRuntime) {
				config := &Config{Name: "test", Image: "ubuntu:22.04"}
				loader.EXPECT().
					LoadConfig(gomock.Any(), "test").
					Return(config, &Settings{}, nil)
				detector.EXPECT().
					DetectRuntime("").
					Return(runtime, nil)
				runtime.EXPECT().
					Inspect(gomock.Any(), "atmos-devcontainer.test.default").
					Return(&container.Info{
						ID:     "old-id",
						Name:   "atmos-devcontainer.test.default",
						Status: "running",
					}, nil)
				runtime.EXPECT().
					Stop(gomock.Any(), "old-id", gomock.Any()).
					Return(nil)
				runtime.EXPECT().
					Remove(gomock.Any(), "old-id", true).
					Return(errors.New("remove failed"))
			},
			expectError: true,
			errorIs:     errUtils.ErrContainerRuntimeOperation,
		},
		{
			name:     "explicit runtime specification - docker",
			devName:  "test",
			instance: "default",
			noPull:   true,
			setupMocks: func(loader *MockConfigLoader, identity *MockIdentityManager, detector *MockRuntimeDetector, runtime *MockRuntime) {
				config := &Config{
					Name:  "test",
					Image: "ubuntu:22.04",
				}
				settings := &Settings{
					Runtime: "docker",
				}
				loader.EXPECT().
					LoadConfig(gomock.Any(), "test").
					Return(config, settings, nil)
				detector.EXPECT().
					DetectRuntime("docker").
					Return(runtime, nil)
				runtime.EXPECT().
					Inspect(gomock.Any(), "atmos-devcontainer.test.default").
					Return(nil, errors.New("not found"))
				runtime.EXPECT().
					Create(gomock.Any(), gomock.Any()).
					Return("new-id", nil)
				runtime.EXPECT().
					Start(gomock.Any(), "new-id").
					Return(nil)
				runtime.EXPECT().
					Inspect(gomock.Any(), "new-id").
					Return(&container.Info{
						ID:    "new-id",
						Ports: []container.PortBinding{},
					}, nil)
			},
			expectError: false,
		},
		{
			name:     "no identity injection when identityName is empty",
			devName:  "test",
			instance: "default",
			noPull:   true,
			setupMocks: func(loader *MockConfigLoader, identity *MockIdentityManager, detector *MockRuntimeDetector, runtime *MockRuntime) {
				config := &Config{Name: "test", Image: "ubuntu:22.04"}
				loader.EXPECT().
					LoadConfig(gomock.Any(), "test").
					Return(config, &Settings{}, nil)
				// InjectIdentityEnvironment should NOT be called
				detector.EXPECT().
					DetectRuntime("").
					Return(runtime, nil)
				runtime.EXPECT().
					Inspect(gomock.Any(), "atmos-devcontainer.test.default").
					Return(nil, errors.New("not found"))
				runtime.EXPECT().
					Create(gomock.Any(), gomock.Any()).
					Return("new-id", nil)
				runtime.EXPECT().
					Start(gomock.Any(), "new-id").
					Return(nil)
				runtime.EXPECT().
					Inspect(gomock.Any(), "new-id").
					Return(&container.Info{
						ID:    "new-id",
						Ports: []container.PortBinding{},
					}, nil)
			},
			expectError: false,
		},
		{
			name:     "rebuild with dockerfile build configuration",
			devName:  "geodesic",
			instance: "default",
			noPull:   false,
			setupMocks: func(loader *MockConfigLoader, identity *MockIdentityManager, detector *MockRuntimeDetector, runtime *MockRuntime) {
				// Config has Build instead of Image (like user's geodesic example).
				config := &Config{
					Name:  "geodesic",
					Image: "", // Empty - will be set by buildImageIfNeeded.
					Build: &Build{
						Context:    ".",
						Dockerfile: "Dockerfile",
						Args: map[string]string{
							"ATMOS_VERSION": "1.201.0",
						},
					},
				}
				loader.EXPECT().
					LoadConfig(gomock.Any(), "geodesic").
					Return(config, &Settings{}, nil)
				detector.EXPECT().
					DetectRuntime("").
					Return(runtime, nil)
				// Container doesn't exist.
				runtime.EXPECT().
					Inspect(gomock.Any(), "atmos-devcontainer.geodesic.default").
					Return(nil, errors.New("not found"))
				// Build should be called since config.Build is set.
				runtime.EXPECT().
					Build(gomock.Any(), gomock.Any()).
					DoAndReturn(func(_ interface{}, buildConfig *container.BuildConfig) error {
						// Verify build config has correct values.
						assert.Equal(t, ".", buildConfig.Context)
						assert.Equal(t, "Dockerfile", buildConfig.Dockerfile)
						assert.Equal(t, []string{"atmos-devcontainer-geodesic"}, buildConfig.Tags)
						assert.Equal(t, map[string]string{"ATMOS_VERSION": "1.201.0"}, buildConfig.Args)
						return nil
					})
				// Pull is NOT called for locally built images since they don't exist
				// in remote registries and pulling would fail.
				// Create new container.
				runtime.EXPECT().
					Create(gomock.Any(), gomock.Any()).
					DoAndReturn(func(_ interface{}, createConfig *container.CreateConfig) (string, error) {
						// Verify the image is set to the built image name.
						assert.Equal(t, "atmos-devcontainer-geodesic", createConfig.Image)
						return "new-id", nil
					})
				// Start new container.
				runtime.EXPECT().
					Start(gomock.Any(), "new-id").
					Return(nil)
				runtime.EXPECT().
					Inspect(gomock.Any(), "new-id").
					Return(&container.Info{
						ID:    "new-id",
						Ports: []container.PortBinding{},
					}, nil)
			},
			expectError: false,
		},
		{
			name:     "rebuild with dockerfile build fails",
			devName:  "geodesic",
			instance: "default",
			noPull:   false,
			setupMocks: func(loader *MockConfigLoader, identity *MockIdentityManager, detector *MockRuntimeDetector, runtime *MockRuntime) {
				config := &Config{
					Name:  "geodesic",
					Image: "",
					Build: &Build{
						Context:    ".",
						Dockerfile: "Dockerfile",
					},
				}
				loader.EXPECT().
					LoadConfig(gomock.Any(), "geodesic").
					Return(config, &Settings{}, nil)
				detector.EXPECT().
					DetectRuntime("").
					Return(runtime, nil)
				runtime.EXPECT().
					Inspect(gomock.Any(), "atmos-devcontainer.geodesic.default").
					Return(nil, errors.New("not found"))
				// Build fails.
				runtime.EXPECT().
					Build(gomock.Any(), gomock.Any()).
					Return(errors.New("build failed: Dockerfile not found"))
			},
			expectError: true,
			errorIs:     errUtils.ErrContainerRuntimeOperation,
		},
		{
			name:     "operations execute in correct order",
			devName:  "test",
			instance: "default",
			noPull:   false,
			setupMocks: func(loader *MockConfigLoader, identity *MockIdentityManager, detector *MockRuntimeDetector, runtime *MockRuntime) {
				config := &Config{
					Name:  "test",
					Image: "ubuntu:22.04",
				}
				// Use gomock.InOrder to enforce sequence: Inspect → Stop → Remove → Pull → Create → Start
				gomock.InOrder(
					loader.EXPECT().
						LoadConfig(gomock.Any(), "test").
						Return(config, &Settings{}, nil),
					detector.EXPECT().
						DetectRuntime("").
						Return(runtime, nil),
					runtime.EXPECT().
						Inspect(gomock.Any(), "atmos-devcontainer.test.default").
						Return(&container.Info{
							ID:     "old-id",
							Name:   "atmos-devcontainer.test.default",
							Status: "running",
						}, nil),
					runtime.EXPECT().
						Stop(gomock.Any(), "old-id", gomock.Any()).
						Return(nil),
					runtime.EXPECT().
						Remove(gomock.Any(), "old-id", true).
						Return(nil),
					runtime.EXPECT().
						Pull(gomock.Any(), "ubuntu:22.04").
						Return(nil),
					runtime.EXPECT().
						Create(gomock.Any(), gomock.Any()).
						Return("new-id", nil),
					runtime.EXPECT().
						Start(gomock.Any(), "new-id").
						Return(nil),
					runtime.EXPECT().
						Inspect(gomock.Any(), "new-id").
						Return(&container.Info{
							ID:    "new-id",
							Ports: []container.PortBinding{},
						}, nil),
				)
			},
			expectError: false,
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

			err := mgr.Rebuild(&schema.AtmosConfiguration{}, tt.devName, tt.instance, tt.identityName, tt.noPull)

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
