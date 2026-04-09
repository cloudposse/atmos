package container

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestGetRuntimeType_Comprehensive tests runtime type detection with edge cases.
func TestGetRuntimeType_Comprehensive(t *testing.T) {
	tests := []struct {
		name     string
		runtime  Runtime
		expected Type
	}{
		{
			name:     "DockerRuntime returns TypeDocker",
			runtime:  NewDockerRuntime(),
			expected: TypeDocker,
		},
		{
			name:     "PodmanRuntime returns TypePodman",
			runtime:  NewPodmanRuntime(),
			expected: TypePodman,
		},
		{
			name:     "nil runtime returns empty Type",
			runtime:  nil,
			expected: "",
		},
		{
			name:     "MockRuntime returns empty Type (unknown implementation)",
			runtime:  NewMockRuntime(nil),
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GetRuntimeType(tt.runtime)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestDetectRuntime_EnvVariable_Comprehensive tests environment variable handling.
func TestDetectRuntime_EnvVariable_Comprehensive(t *testing.T) {
	tests := []struct {
		name        string
		envValue    string
		expectError bool
		errorMsg    string
		expectType  Type
	}{
		{
			name:        "invalid runtime type",
			envValue:    "invalid-runtime",
			expectError: true,
			errorMsg:    "unknown runtime type 'invalid-runtime'",
		},
		{
			name:        "random string",
			envValue:    "foobar",
			expectError: true,
			errorMsg:    "unknown runtime type 'foobar'",
		},
		{
			name:        "empty string skips env check",
			envValue:    "",
			expectError: false, // Falls back to auto-detection.
		},
		{
			name:        "case sensitive - Docker (uppercase)",
			envValue:    "Docker",
			expectError: true,
			errorMsg:    "unknown runtime type 'Docker'",
		},
		{
			name:        "case sensitive - DOCKER (all caps)",
			envValue:    "DOCKER",
			expectError: true,
			errorMsg:    "unknown runtime type 'DOCKER'",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.envValue != "" {
				t.Setenv("ATMOS_CONTAINER_RUNTIME", tt.envValue)
			}

			ctx := context.Background()
			runtime, err := DetectRuntime(ctx)

			if tt.expectError {
				require.Error(t, err)
				if tt.errorMsg != "" {
					assert.Contains(t, err.Error(), tt.errorMsg)
				}
				assert.Nil(t, runtime)
			} else {
				// For empty env, auto-detection happens.
				// We can't assert success without mocking, but we can verify no panic.
				_ = runtime
				_ = err
			}
		})
	}
}

// TestDetectRuntime_PriorityOrder tests the detection priority order.
func TestDetectRuntime_PriorityOrder(t *testing.T) {
	// This test documents the priority order:
	// 1. ATMOS_CONTAINER_RUNTIME env var
	// 2. Docker (if available)
	// 3. Podman (if available)
	// 4. Error if none available

	t.Run("env var docker takes precedence", func(t *testing.T) {
		t.Setenv("ATMOS_CONTAINER_RUNTIME", "docker")
		ctx := context.Background()

		runtime, err := DetectRuntime(ctx)

		// If docker is not available, will error.
		// If docker is available, will return DockerRuntime.
		if err == nil {
			require.NotNil(t, runtime)
			assert.Equal(t, TypeDocker, GetRuntimeType(runtime))
		} else {
			assert.Contains(t, err.Error(), "docker is not available")
		}
	})

	t.Run("env var podman takes precedence", func(t *testing.T) {
		t.Setenv("ATMOS_CONTAINER_RUNTIME", "podman")
		ctx := context.Background()

		runtime, err := DetectRuntime(ctx)

		// If podman is not available, will error.
		// If podman is available, will return PodmanRuntime.
		if err == nil {
			require.NotNil(t, runtime)
			assert.Equal(t, TypePodman, GetRuntimeType(runtime))
		} else {
			assert.Contains(t, err.Error(), "podman is not available")
		}
	})
}

// TestType_String_Additional tests additional Type String() method cases.
func TestType_String_Additional(t *testing.T) {
	tests := []struct {
		name     string
		typ      Type
		expected string
	}{
		{
			name:     "empty Type",
			typ:      Type(""),
			expected: "",
		},
		{
			name:     "custom Type",
			typ:      Type("custom-runtime"),
			expected: "custom-runtime",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.typ.String()
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestDetectRuntime_AutoDetection tests auto-detection when no env var is set.
func TestDetectRuntime_AutoDetection(t *testing.T) {
	// Ensure no env var is set.
	t.Setenv("ATMOS_CONTAINER_RUNTIME", "")

	ctx := context.Background()
	runtime, err := DetectRuntime(ctx)

	// Result depends on what's available on the system.
	if err != nil {
		// No runtime available - expected in some CI environments.
		assert.Contains(t, err.Error(), "neither docker nor podman is available")
		assert.Nil(t, runtime)
	} else {
		// At least one runtime available.
		require.NotNil(t, runtime)
		runtimeType := GetRuntimeType(runtime)
		assert.True(t,
			runtimeType == TypeDocker || runtimeType == TypePodman,
			"runtime should be Docker or Podman",
		)
	}
}

// TestPodmanMachineExists_Logic tests the machine existence check logic.
func TestPodmanMachineExists_Logic(t *testing.T) {
	tests := []struct {
		name           string
		output         string
		expectMachines bool
	}{
		{
			name:           "machines exist - single machine",
			output:         "podman-machine-default",
			expectMachines: true,
		},
		{
			name: "machines exist - multiple machines",
			output: `machine1
machine2
machine3`,
			expectMachines: true,
		},
		{
			name:           "no machines - empty output",
			output:         "",
			expectMachines: false,
		},
		{
			name:           "no machines - whitespace only",
			output:         "   \n\t  ",
			expectMachines: false,
		},
		{
			name:           "machines exist - with trailing newline",
			output:         "podman-machine-default\n",
			expectMachines: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Use the actual production function to test the logic.
			exists := hasMachines(tt.output)

			assert.Equal(t, tt.expectMachines, exists)
		})
	}
}

// TestDetectRuntime_ErrorMessages tests error message format.
func TestDetectRuntime_ErrorMessages(t *testing.T) {
	tests := []struct {
		name          string
		envValue      string
		expectedError string
	}{
		{
			name:          "unknown runtime type",
			envValue:      "foobar",
			expectedError: "unknown runtime type 'foobar'",
		},
		{
			name:          "docker specified but not available",
			envValue:      "docker",
			expectedError: "docker is not available or not running",
		},
		{
			name:          "podman specified but not available",
			envValue:      "podman",
			expectedError: "podman is not available or not running",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("ATMOS_CONTAINER_RUNTIME", tt.envValue)

			ctx := context.Background()
			runtime, err := DetectRuntime(ctx)
			// We expect an error for these cases (unless the runtime actually is available).
			// If no error, skip assertion (runtime is available on this system).
			if err != nil {
				assert.Contains(t, err.Error(), tt.expectedError)
				assert.Nil(t, runtime)
			}
		})
	}
}

// TestDetectRuntime_ContextCancellation tests context handling.
func TestDetectRuntime_ContextCancellation(t *testing.T) {
	// Create a cancelled context.
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately.

	runtime, err := DetectRuntime(ctx)
	// Behavior depends on implementation details and timing.
	// In most cases, a cancelled context will cause command execution to fail.
	// We document that both outcomes are possible.
	if err != nil {
		// Context cancellation may cause error.
		t.Logf("DetectRuntime failed with cancelled context: %v", err)
	}
	if runtime == nil {
		// Or runtime may be nil.
		t.Logf("DetectRuntime returned nil with cancelled context")
	}

	// Main assertion: no panic.
	// If we reach here, test passes.
}

// TestNewDockerRuntime_Type tests Docker runtime type.
func TestNewDockerRuntime_Type(t *testing.T) {
	runtime := NewDockerRuntime()
	require.NotNil(t, runtime)

	// Verify it implements Runtime interface.
	var _ Runtime = runtime

	// Verify type detection works.
	assert.Equal(t, TypeDocker, GetRuntimeType(runtime))
}

// TestNewPodmanRuntime_Type tests Podman runtime type.
func TestNewPodmanRuntime_Type(t *testing.T) {
	runtime := NewPodmanRuntime()
	require.NotNil(t, runtime)

	// Verify it implements Runtime interface.
	var _ Runtime = runtime

	// Verify type detection works.
	assert.Equal(t, TypePodman, GetRuntimeType(runtime))
}
