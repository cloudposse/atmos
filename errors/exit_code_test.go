package errors

import (
	"os/exec"
	"runtime"
	"testing"

	"github.com/cockroachdb/errors"
	"github.com/stretchr/testify/assert"
)

func TestWithExitCode(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		code     int
		wantCode int
	}{
		{
			name:     "nil error returns nil",
			err:      nil,
			code:     1,
			wantCode: 0,
		},
		{
			name:     "simple error with code 0",
			err:      errors.New("test error"),
			code:     0,
			wantCode: 0,
		},
		{
			name:     "simple error with code 1",
			err:      errors.New("test error"),
			code:     1,
			wantCode: 1,
		},
		{
			name:     "simple error with code 2",
			err:      errors.New("test error"),
			code:     2,
			wantCode: 2,
		},
		{
			name:     "wrapped error preserves code",
			err:      errors.Wrap(errors.New("base error"), "wrapper"),
			code:     3,
			wantCode: 3,
		},
		{
			name:     "multiple wrappers preserve code",
			err:      errors.Wrap(errors.Wrap(errors.New("base"), "middle"), "top"),
			code:     42,
			wantCode: 42,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := WithExitCode(tt.err, tt.code)
			got := GetExitCode(err)
			assert.Equal(t, tt.wantCode, got)
		})
	}
}

func TestWithExitCode_Chaining(t *testing.T) {
	// Test that exit code survives wrapping
	err := errors.New("base error")
	err = WithExitCode(err, 5)
	err = errors.Wrap(err, "wrapped once")
	err = errors.Wrap(err, "wrapped twice")

	assert.Equal(t, 5, GetExitCode(err))
}

func TestWithExitCode_MultipleExitCodes(t *testing.T) {
	// Test that the first exit code in the chain is used
	err := errors.New("base error")
	err = WithExitCode(err, 3)
	err = errors.Wrap(err, "wrapper")
	err = WithExitCode(err, 5) // Second exit code

	// Should use the first one encountered when traversing from top
	assert.Equal(t, 5, GetExitCode(err))
}

func TestGetExitCode_NilError(t *testing.T) {
	assert.Equal(t, 0, GetExitCode(nil))
}

func TestGetExitCode_DefaultValue(t *testing.T) {
	// Error without exit code should return 1
	err := errors.New("test error")
	assert.Equal(t, 1, GetExitCode(err))
}

func TestGetExitCode_ExecExitError(t *testing.T) {
	// Create a command that will fail with exit code 1
	err := failingCommand().Run()

	assert.NotNil(t, err)

	// Should extract exit code from exec.ExitError
	code := GetExitCode(err)
	assert.NotEqual(t, 0, code)
	assert.Equal(t, 1, code) // exit 1
}

func TestGetExitCode_WrappedExecError(t *testing.T) {
	// Create a command that will fail
	baseErr := failingCommand().Run()

	// Wrap the exec error
	err := errors.Wrap(baseErr, "command failed")
	err = errors.Wrap(err, "execution error")

	assert.NotNil(t, err)

	// Should still extract exit code from wrapped exec.ExitError
	code := GetExitCode(err)
	assert.NotEqual(t, 0, code)
}

// failingCommand returns a cross-platform command that will fail with exit code 1.
func failingCommand() *exec.Cmd {
	if runtime.GOOS == "windows" {
		return exec.Command("cmd", "/C", "exit 1")
	}
	return exec.Command("sh", "-c", "exit 1")
}

func TestExitCoder_Error(t *testing.T) {
	baseErr := errors.New("test error")
	ec := &exitCoder{
		cause: baseErr,
		code:  42,
	}
	assert.Equal(t, "test error", ec.Error())
	assert.Equal(t, 42, ec.ExitCode())
}

func TestWithExitCode_PreservesOriginalError(t *testing.T) {
	original := errors.New("original error")
	withCode := WithExitCode(original, 5)

	// Should be able to unwrap to get original
	assert.True(t, errors.Is(withCode, original))

	// Error message should still be the original
	assert.Contains(t, withCode.Error(), "original error")
}

// TestExitCodeSIGINT verifies that ExitCodeSIGINT constant has the correct POSIX value.
// POSIX specifies that when a process is terminated by a signal, the exit code is 128 + signal_number.
// SIGINT (Ctrl+C) is signal number 2, so the exit code should be 130 (128 + 2).
func TestExitCodeSIGINT(t *testing.T) {
	assert.Equal(t, 130, ExitCodeSIGINT, "ExitCodeSIGINT should be 130 (POSIX: 128 + SIGINT signal 2)")
}

// TestExitCodeSIGINT_UserAbort verifies that user abort errors exit with SIGINT code.
// This is a regression test for the identity selector exit handling bug.
func TestExitCodeSIGINT_UserAbort(t *testing.T) {
	// When user presses Ctrl+C during identity selection, we should exit with ExitCodeSIGINT.
	// This test documents the expected behavior.

	// Verify that ErrUserAborted is defined
	assert.NotNil(t, ErrUserAborted, "ErrUserAborted should be defined")

	// Verify that WithExitCode can attach SIGINT code to user abort error
	err := WithExitCode(ErrUserAborted, ExitCodeSIGINT)
	assert.Equal(t, ExitCodeSIGINT, GetExitCode(err), "User abort should use SIGINT exit code")
}
