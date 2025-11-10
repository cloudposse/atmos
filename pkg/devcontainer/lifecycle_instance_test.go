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

func TestManager_GenerateNewInstance(t *testing.T) {
	tests := []struct {
		name           string
		devName        string
		baseInstance   string
		setupMocks     func(*MockConfigLoader, *MockRuntimeDetector, *MockRuntime)
		expectedResult string
		expectError    bool
		errorIs        error
		errorContains  string
	}{
		{
			name:         "generate first instance when no containers exist",
			devName:      "test",
			baseInstance: "default",
			setupMocks: func(loader *MockConfigLoader, detector *MockRuntimeDetector, runtime *MockRuntime) {
				loader.EXPECT().
					LoadConfig(gomock.Any(), "test").
					Return(&Config{}, &Settings{}, nil)
				detector.EXPECT().
					DetectRuntime("").
					Return(runtime, nil)
				runtime.EXPECT().
					List(gomock.Any(), nil).
					Return([]container.Info{}, nil)
			},
			expectedResult: "default-1",
			expectError:    false,
		},
		{
			name:         "generate next instance with hyphenated base (dot format)",
			devName:      "test",
			baseInstance: "dev",
			setupMocks: func(loader *MockConfigLoader, detector *MockRuntimeDetector, runtime *MockRuntime) {
				loader.EXPECT().
					LoadConfig(gomock.Any(), "test").
					Return(&Config{}, &Settings{}, nil)
				detector.EXPECT().
					DetectRuntime("").
					Return(runtime, nil)
				runtime.EXPECT().
					List(gomock.Any(), nil).
					Return([]container.Info{
						{Name: "atmos-devcontainer.test.dev-1"},
						{Name: "atmos-devcontainer.test.dev-2"},
						{Name: "atmos-devcontainer.other.dev-1"},
					}, nil)
			},
			expectedResult: "dev-3",
			expectError:    false,
		},
		{
			name:         "generate instance with gaps (dot format)",
			devName:      "test",
			baseInstance: "dev",
			setupMocks: func(loader *MockConfigLoader, detector *MockRuntimeDetector, runtime *MockRuntime) {
				loader.EXPECT().
					LoadConfig(gomock.Any(), "test").
					Return(&Config{}, &Settings{}, nil)
				detector.EXPECT().
					DetectRuntime("").
					Return(runtime, nil)
				runtime.EXPECT().
					List(gomock.Any(), nil).
					Return([]container.Info{
						{Name: "atmos-devcontainer.test.dev-1"},
						{Name: "atmos-devcontainer.test.dev-5"},
						{Name: "atmos-devcontainer.test.dev-3"},
					}, nil)
			},
			expectedResult: "dev-6",
			expectError:    false,
		},
		{
			name:         "use DefaultInstance when baseInstance is empty",
			devName:      "test",
			baseInstance: "",
			setupMocks: func(loader *MockConfigLoader, detector *MockRuntimeDetector, runtime *MockRuntime) {
				loader.EXPECT().
					LoadConfig(gomock.Any(), "test").
					Return(&Config{}, &Settings{}, nil)
				detector.EXPECT().
					DetectRuntime("").
					Return(runtime, nil)
				runtime.EXPECT().
					List(gomock.Any(), nil).
					Return([]container.Info{}, nil)
			},
			expectedResult: "default-1",
			expectError:    false,
		},
		{
			name:         "ignore containers from different devcontainers",
			devName:      "test",
			baseInstance: "default",
			setupMocks: func(loader *MockConfigLoader, detector *MockRuntimeDetector, runtime *MockRuntime) {
				loader.EXPECT().
					LoadConfig(gomock.Any(), "test").
					Return(&Config{}, &Settings{}, nil)
				detector.EXPECT().
					DetectRuntime("").
					Return(runtime, nil)
				runtime.EXPECT().
					List(gomock.Any(), nil).
					Return([]container.Info{
						{Name: "atmos-devcontainer.other.default-10"},
						{Name: "atmos-devcontainer.another.default-20"},
					}, nil)
			},
			expectedResult: "default-1",
			expectError:    false,
		},
		{
			name:         "config load fails",
			devName:      "test",
			baseInstance: "default",
			setupMocks: func(loader *MockConfigLoader, detector *MockRuntimeDetector, runtime *MockRuntime) {
				loader.EXPECT().
					LoadConfig(gomock.Any(), "test").
					Return(nil, nil, errors.New("config error"))
			},
			expectError:   true,
			errorContains: "config error",
		},
		{
			name:         "runtime detection fails",
			devName:      "test",
			baseInstance: "default",
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
		{
			name:         "container list fails",
			devName:      "test",
			baseInstance: "default",
			setupMocks: func(loader *MockConfigLoader, detector *MockRuntimeDetector, runtime *MockRuntime) {
				loader.EXPECT().
					LoadConfig(gomock.Any(), "test").
					Return(&Config{}, &Settings{}, nil)
				detector.EXPECT().
					DetectRuntime("").
					Return(runtime, nil)
				runtime.EXPECT().
					List(gomock.Any(), nil).
					Return(nil, errors.New("list failed"))
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

			result, err := mgr.GenerateNewInstance(&schema.AtmosConfiguration{}, tt.devName, tt.baseInstance)

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
				assert.Equal(t, tt.expectedResult, result)
			}
		})
	}
}

func TestFindMaxInstanceNumber(t *testing.T) {
	tests := []struct {
		name         string
		containers   []container.Info
		devName      string
		baseInstance string
		expected     int
	}{
		{
			name:         "no matching containers returns 0",
			containers:   []container.Info{},
			devName:      "myapp",
			baseInstance: "default",
			expected:     0,
		},
		{
			name: "finds max instance number with dot format",
			containers: []container.Info{
				{Name: "atmos-devcontainer.myapp.default-1"},
				{Name: "atmos-devcontainer.myapp.default-5"},
			},
			devName:      "myapp",
			baseInstance: "default",
			expected:     5,
		},
		{
			name: "finds max with hyphenated devcontainer name (dot format)",
			containers: []container.Info{
				{Name: "atmos-devcontainer.backend-api.dev-1"},
				{Name: "atmos-devcontainer.backend-api.dev-3"},
				{Name: "atmos-devcontainer.backend-api.dev-2"},
			},
			devName:      "backend-api",
			baseInstance: "dev",
			expected:     3,
		},
		{
			name: "ignores legacy hyphen format (ambiguous parsing)",
			containers: []container.Info{
				{Name: "atmos-devcontainer-myapp-default-1"},
				{Name: "atmos-devcontainer-myapp-default-5"},
			},
			devName:      "myapp",
			baseInstance: "default",
			expected:     0, // Legacy format not reliably parseable with hyphenated names
		},
		{
			name: "ignores containers from different devcontainers",
			containers: []container.Info{
				{Name: "atmos-devcontainer.myapp.default-3"},
				{Name: "atmos-devcontainer.other.default-10"},
				{Name: "atmos-devcontainer.another.default-20"},
			},
			devName:      "myapp",
			baseInstance: "default",
			expected:     3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := findMaxInstanceNumber(tt.containers, tt.devName, tt.baseInstance)
			assert.Equal(t, tt.expected, result)
		})
	}
}
