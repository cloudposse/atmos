package errors

import (
	"testing"

	"github.com/cockroachdb/errors"
	"github.com/stretchr/testify/assert"
)

func TestNewExecError(t *testing.T) {
	cause := errors.New("underlying error")
	err := NewExecError("terraform", []string{"plan", "-var=foo"}, 1, cause)

	assert.NotNil(t, err)
	assert.Equal(t, "terraform", err.Cmd)
	assert.Equal(t, []string{"plan", "-var=foo"}, err.Args)
	assert.Equal(t, 1, err.ExitCode)
	assert.Equal(t, cause, err.cause)
	assert.Empty(t, err.Stderr)
}

func TestNewExecError_NoArgs(t *testing.T) {
	cause := errors.New("command failed")
	err := NewExecError("helm", []string{}, 2, cause)

	assert.NotNil(t, err)
	assert.Equal(t, "helm", err.Cmd)
	assert.Empty(t, err.Args)
	assert.Equal(t, 2, err.ExitCode)
	assert.Equal(t, cause, err.cause)
}

func TestNewExecError_NilCause(t *testing.T) {
	err := NewExecError("sh", []string{"-c", "exit 1"}, 1, nil)

	assert.NotNil(t, err)
	assert.Equal(t, "sh", err.Cmd)
	assert.Equal(t, []string{"-c", "exit 1"}, err.Args)
	assert.Equal(t, 1, err.ExitCode)
	assert.Nil(t, err.cause)
}

func TestExecError_Error_WithCause(t *testing.T) {
	cause := errors.New("connection timeout")
	err := NewExecError("terraform", []string{"apply"}, 1, cause)

	// When cause is set, Error() returns cause.Error().
	assert.Equal(t, "connection timeout", err.Error())
}

func TestExecError_Error_WithoutCause(t *testing.T) {
	err := NewExecError("terraform", []string{"plan"}, 1, nil)

	// When cause is nil, Error() returns formatted message.
	assert.Equal(t, "command terraform plan exited with code 1", err.Error())
}

func TestExecError_Error_WithoutCause_NoArgs(t *testing.T) {
	err := NewExecError("helm", []string{}, 2, nil)

	// Without args, should only show command name.
	assert.Equal(t, "command helm exited with code 2", err.Error())
}

func TestExecError_Error_WithoutCause_MultipleArgs(t *testing.T) {
	err := NewExecError("terraform", []string{"apply", "-auto-approve", "-var=foo"}, 1, nil)

	// Should join args with spaces.
	assert.Equal(t, "command terraform apply -auto-approve -var=foo exited with code 1", err.Error())
}

func TestExecError_Unwrap(t *testing.T) {
	cause := errors.New("underlying error")
	err := NewExecError("terraform", []string{"plan"}, 1, cause)

	// Unwrap should return the cause.
	unwrapped := err.Unwrap()
	assert.Equal(t, cause, unwrapped)
}

func TestExecError_Unwrap_NilCause(t *testing.T) {
	err := NewExecError("helm", []string{}, 1, nil)

	// Unwrap should return nil when cause is nil.
	unwrapped := err.Unwrap()
	assert.Nil(t, unwrapped)
}

func TestExecError_WithStderr(t *testing.T) {
	err := NewExecError("terraform", []string{"plan"}, 1, nil)
	stderrOutput := "Error: Invalid configuration\nstack trace here"

	result := err.WithStderr(stderrOutput)

	// Should return the same error instance.
	assert.Equal(t, err, result)
	// Should set stderr field.
	assert.Equal(t, stderrOutput, err.Stderr)
}

func TestExecError_WithStderr_Chaining(t *testing.T) {
	err := NewExecError("terraform", []string{"apply"}, 1, nil).
		WithStderr("Error: Failed to apply")

	assert.Equal(t, "Error: Failed to apply", err.Stderr)
	assert.Equal(t, "terraform", err.Cmd)
	assert.Equal(t, 1, err.ExitCode)
}

func TestExecError_WithStderr_EmptyString(t *testing.T) {
	err := NewExecError("helm", []string{}, 1, nil).
		WithStderr("")

	assert.Empty(t, err.Stderr)
}

func TestExecError_CompleteExample(t *testing.T) {
	cause := errors.New("network error")
	err := NewExecError("terraform", []string{"apply", "-auto-approve"}, 1, cause).
		WithStderr("Error: timeout waiting for resources")

	assert.Equal(t, "terraform", err.Cmd)
	assert.Equal(t, []string{"apply", "-auto-approve"}, err.Args)
	assert.Equal(t, 1, err.ExitCode)
	assert.Equal(t, cause, err.cause)
	assert.Equal(t, "Error: timeout waiting for resources", err.Stderr)
	assert.Equal(t, "network error", err.Error())
	assert.Equal(t, cause, err.Unwrap())
}

func TestExecError_ErrorIsCompatible(t *testing.T) {
	cause := errors.New("base error")
	wrappedCause := errors.Wrap(cause, "wrapped")
	err := NewExecError("terraform", []string{"plan"}, 1, wrappedCause)

	// Should be compatible with errors.Is for the cause chain.
	assert.True(t, errors.Is(err, wrappedCause))
	assert.True(t, errors.Is(err, cause))
}
