package exec

import (
	"errors"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestExecAuthShellCommand_ExitCodePropagation(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skipf("Skipping Unix shell test on Windows")
	}

	if testing.Short() {
		t.Skipf("Skipping test in short mode: spawns actual shell process")
	}

	tests := []struct {
		name         string
		shellArgs    []string
		expectedCode int
	}{
		{
			name:         "exit code 0",
			shellArgs:    []string{"-c", "exit 0"},
			expectedCode: 0,
		},
		{
			name:         "exit code 1",
			shellArgs:    []string{"-c", "exit 1"},
			expectedCode: 1,
		},
		{
			name:         "exit code 42",
			shellArgs:    []string{"-c", "exit 42"},
			expectedCode: 42,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			envVars := []string{"TEST_VAR=test_value"}
			atmosConfig := &schema.AtmosConfiguration{}

			err := ExecAuthShellCommand(atmosConfig, "test-identity", "test-provider", envVars, "/bin/sh", tt.shellArgs)

			if tt.expectedCode == 0 {
				assert.NoError(t, err)
			} else {
				require.Error(t, err)
				var exitCodeErr errUtils.ExitCodeError
				require.True(t, errors.As(err, &exitCodeErr), "error should be ExitCodeError")
				assert.Equal(t, tt.expectedCode, exitCodeErr.Code)
			}
		})
	}
}
