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

func TestManager_List(t *testing.T) {
	tests := []struct {
		name          string
		setupMocks    func(*MockConfigLoader, *MockRuntimeDetector, *MockRuntime)
		expectError   bool
		errorIs       error
		errorContains string
	}{
		{
			name: "no devcontainers configured",
			setupMocks: func(loader *MockConfigLoader, detector *MockRuntimeDetector, runtime *MockRuntime) {
				loader.EXPECT().
					LoadAllConfigs(gomock.Any()).
					Return(map[string]*Config{}, nil)
			},
			expectError: false,
		},
		{
			name: "load configs fails",
			setupMocks: func(loader *MockConfigLoader, detector *MockRuntimeDetector, runtime *MockRuntime) {
				loader.EXPECT().
					LoadAllConfigs(gomock.Any()).
					Return(nil, errors.New("config error"))
			},
			expectError:   true,
			errorContains: "config error",
		},
		{
			name: "list with no running containers",
			setupMocks: func(loader *MockConfigLoader, detector *MockRuntimeDetector, runtime *MockRuntime) {
				loader.EXPECT().
					LoadAllConfigs(gomock.Any()).
					Return(map[string]*Config{
						"test": {
							Name:  "test",
							Image: "ubuntu:22.04",
						},
					}, nil)
				detector.EXPECT().
					DetectRuntime("").
					Return(runtime, nil)
				runtime.EXPECT().
					List(gomock.Any(), gomock.Any()).
					Return([]container.Info{}, nil)
			},
			expectError: false,
		},
		{
			name: "list with running containers",
			setupMocks: func(loader *MockConfigLoader, detector *MockRuntimeDetector, runtime *MockRuntime) {
				loader.EXPECT().
					LoadAllConfigs(gomock.Any()).
					Return(map[string]*Config{
						"test": {
							Name:  "test",
							Image: "ubuntu:22.04",
						},
						"dev": {
							Name:  "dev",
							Image: "node:18",
						},
					}, nil)
				detector.EXPECT().
					DetectRuntime("").
					Return(runtime, nil)
				runtime.EXPECT().
					List(gomock.Any(), gomock.Any()).
					Return([]container.Info{
						{
							Name:   "atmos-devcontainer-test-default",
							Status: "running",
						},
						{
							Name:   "atmos-devcontainer-dev-default",
							Status: "exited",
						},
					}, nil)
			},
			expectError: false,
		},
		{
			name: "runtime detection fails",
			setupMocks: func(loader *MockConfigLoader, detector *MockRuntimeDetector, runtime *MockRuntime) {
				loader.EXPECT().
					LoadAllConfigs(gomock.Any()).
					Return(map[string]*Config{
						"test": {Name: "test", Image: "ubuntu:22.04"},
					}, nil)
				detector.EXPECT().
					DetectRuntime("").
					Return(nil, errors.New("runtime not found"))
			},
			expectError:   true,
			errorIs:       errUtils.ErrContainerRuntimeOperation,
			errorContains: "failed to initialize container runtime",
		},
		{
			name: "runtime list fails",
			setupMocks: func(loader *MockConfigLoader, detector *MockRuntimeDetector, runtime *MockRuntime) {
				loader.EXPECT().
					LoadAllConfigs(gomock.Any()).
					Return(map[string]*Config{
						"test": {Name: "test", Image: "ubuntu:22.04"},
					}, nil)
				detector.EXPECT().
					DetectRuntime("").
					Return(runtime, nil)
				runtime.EXPECT().
					List(gomock.Any(), gomock.Any()).
					Return(nil, errors.New("list error"))
			},
			expectError:   true,
			errorIs:       errUtils.ErrContainerRuntimeOperation,
			errorContains: "failed to list containers",
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

			err := mgr.List(&schema.AtmosConfiguration{})

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

func TestRenderListTable(t *testing.T) {
	tests := []struct {
		name         string
		configs      map[string]*Config
		runningNames map[string]bool
	}{
		{
			name:         "empty configs",
			configs:      map[string]*Config{},
			runningNames: map[string]bool{},
		},
		{
			name: "single config not running",
			configs: map[string]*Config{
				"test": {
					Name:  "test",
					Image: "ubuntu:22.04",
				},
			},
			runningNames: map[string]bool{},
		},
		{
			name: "single config running",
			configs: map[string]*Config{
				"test": {
					Name:  "test",
					Image: "ubuntu:22.04",
				},
			},
			runningNames: map[string]bool{"test": true},
		},
		{
			name: "multiple configs mixed status",
			configs: map[string]*Config{
				"web": {
					Name:         "web",
					Image:        "nginx:latest",
					ForwardPorts: []interface{}{8080, "3000:3001"},
				},
				"db": {
					Name:         "db",
					Image:        "postgres:14",
					ForwardPorts: []interface{}{5432},
				},
			},
			runningNames: map[string]bool{"web": true},
		},
		{
			name: "config with build instead of image",
			configs: map[string]*Config{
				"builder": {
					Name: "builder",
					Build: &Build{
						Dockerfile: "Dockerfile.dev",
						Context:    ".",
					},
				},
			},
			runningNames: map[string]bool{"builder": true},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// This test just verifies the function doesn't panic.
			// Output verification would require capturing stdout.
			assert.NotPanics(t, func() {
				renderListTable(tt.configs, tt.runningNames)
			})
		})
	}
}
