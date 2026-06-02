//go:build !windows

package auth

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestExecuteCommandWithEnv_WithFailingCommand(t *testing.T) {
	// Test with a command that always fails.
	// The "false" command is Unix-specific (not available on Windows).
	err := executeCommandWithEnv([]string{"false"}, nil)

	// "false" command should return non-zero exit code.
	assert.Error(t, err)
}
