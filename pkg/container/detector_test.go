package container

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetRuntimeType(t *testing.T) {
	tests := []struct {
		name     string
		runtime  Runtime
		expected Type
	}{
		{
			name:     "docker runtime returns TypeDocker",
			runtime:  NewDockerRuntime(),
			expected: TypeDocker,
		},
		{
			name:     "podman runtime returns TypePodman",
			runtime:  NewPodmanRuntime(),
			expected: TypePodman,
		},
		{
			name:     "nil runtime returns empty string",
			runtime:  nil,
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

func TestDetectRuntime_EnvVariable(t *testing.T) {
	tests := []struct {
		name        string
		envValue    string
		expectError bool
		errorMsg    string
	}{
		{
			name:        "unknown runtime type",
			envValue:    "invalid",
			expectError: true,
			errorMsg:    "unknown runtime type 'invalid'",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("ATMOS_CONTAINER_RUNTIME", tt.envValue)

			ctx := context.Background()
			runtime, err := DetectRuntime(ctx)

			if tt.expectError {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errorMsg)
				assert.Nil(t, runtime)
			} else {
				require.NoError(t, err)
				assert.NotNil(t, runtime)
			}
		})
	}
}

// Note: Full testing of DetectRuntime requires mocking exec.LookPath and exec.Command,
// which would require refactoring the detector to accept injectable dependencies.
// For now, we test the logic we can without external dependencies.
// Integration tests should cover the full detector behavior.
