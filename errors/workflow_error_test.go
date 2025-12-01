package errors

import (
	"testing"

	"github.com/cockroachdb/errors"
	"github.com/stretchr/testify/assert"
)

func TestNewWorkflowStepError(t *testing.T) {
	cause := errors.New("command execution failed")
	err := NewWorkflowStepError("deploy", "step-1", "terraform apply", 1, cause)

	assert.NotNil(t, err)
	assert.Equal(t, "deploy", err.Workflow)
	assert.Equal(t, "step-1", err.Step)
	assert.Equal(t, "terraform apply", err.Command)
	assert.Equal(t, 1, err.ExitCode)
	assert.Equal(t, cause, err.cause)
}

func TestNewWorkflowStepError_NilCause(t *testing.T) {
	err := NewWorkflowStepError("deploy", "step-2", "helm install", 2, nil)

	assert.NotNil(t, err)
	assert.Equal(t, "deploy", err.Workflow)
	assert.Equal(t, "step-2", err.Step)
	assert.Equal(t, "helm install", err.Command)
	assert.Equal(t, 2, err.ExitCode)
	assert.Nil(t, err.cause)
}

func TestNewWorkflowStepError_EmptyStrings(t *testing.T) {
	err := NewWorkflowStepError("", "", "", 0, nil)

	assert.NotNil(t, err)
	assert.Empty(t, err.Workflow)
	assert.Empty(t, err.Step)
	assert.Empty(t, err.Command)
	assert.Equal(t, 0, err.ExitCode)
}

func TestWorkflowStepError_Error_WithCause(t *testing.T) {
	cause := errors.New("network timeout")
	err := NewWorkflowStepError("deploy", "apply", "terraform apply", 1, cause)

	// When cause is set, Error() returns cause.Error().
	assert.Equal(t, "network timeout", err.Error())
}

func TestWorkflowStepError_Error_WithoutCause(t *testing.T) {
	err := NewWorkflowStepError("deploy", "plan", "terraform plan", 1, nil)

	// When cause is nil, Error() returns generic message.
	assert.Equal(t, "workflow step execution failed", err.Error())
}

func TestWorkflowStepError_Unwrap(t *testing.T) {
	cause := errors.New("underlying error")
	err := NewWorkflowStepError("deploy", "step-1", "terraform apply", 1, cause)

	// Unwrap should return the cause.
	unwrapped := err.Unwrap()
	assert.Equal(t, cause, unwrapped)
}

func TestWorkflowStepError_Unwrap_NilCause(t *testing.T) {
	err := NewWorkflowStepError("deploy", "step-1", "terraform apply", 1, nil)

	// Unwrap should return nil when cause is nil.
	unwrapped := err.Unwrap()
	assert.Nil(t, unwrapped)
}

func TestWorkflowStepError_WorkflowStepMessage(t *testing.T) {
	err := NewWorkflowStepError("deploy", "apply", "terraform apply", 1, nil)

	message := err.WorkflowStepMessage()
	assert.Equal(t, "workflow step execution failed with exit code 1", message)
}

func TestWorkflowStepError_WorkflowStepMessage_DifferentExitCodes(t *testing.T) {
	tests := []struct {
		name     string
		exitCode int
		expected string
	}{
		{
			name:     "exit code 0",
			exitCode: 0,
			expected: "workflow step execution failed with exit code 0",
		},
		{
			name:     "exit code 1",
			exitCode: 1,
			expected: "workflow step execution failed with exit code 1",
		},
		{
			name:     "exit code 2",
			exitCode: 2,
			expected: "workflow step execution failed with exit code 2",
		},
		{
			name:     "exit code 127",
			exitCode: 127,
			expected: "workflow step execution failed with exit code 127",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := NewWorkflowStepError("workflow", "step", "command", tt.exitCode, nil)
			message := err.WorkflowStepMessage()
			assert.Equal(t, tt.expected, message)
		})
	}
}

func TestWorkflowStepError_CompleteExample(t *testing.T) {
	cause := errors.New("terraform command failed")
	err := NewWorkflowStepError("deploy-prod", "apply-infrastructure", "terraform apply -auto-approve", 1, cause)

	assert.Equal(t, "deploy-prod", err.Workflow)
	assert.Equal(t, "apply-infrastructure", err.Step)
	assert.Equal(t, "terraform apply -auto-approve", err.Command)
	assert.Equal(t, 1, err.ExitCode)
	assert.Equal(t, cause, err.cause)
	assert.Equal(t, "terraform command failed", err.Error())
	assert.Equal(t, cause, err.Unwrap())
	assert.Equal(t, "workflow step execution failed with exit code 1", err.WorkflowStepMessage())
}

func TestWorkflowStepError_ErrorIsCompatible(t *testing.T) {
	cause := errors.New("base error")
	wrappedCause := errors.Wrap(cause, "wrapped")
	err := NewWorkflowStepError("workflow", "step", "command", 1, wrappedCause)

	// Should be compatible with errors.Is for the cause chain.
	assert.True(t, errors.Is(err, wrappedCause))
	assert.True(t, errors.Is(err, cause))
}

func TestWorkflowStepError_WrappingWithExecError(t *testing.T) {
	// Simulate workflow wrapping an ExecError.
	execErr := NewExecError("terraform", []string{"apply"}, 1, errors.New("connection failed"))
	workflowErr := NewWorkflowStepError("deploy", "apply-step", "terraform apply", 1, execErr)

	assert.Equal(t, execErr, workflowErr.cause)
	assert.Equal(t, execErr.Error(), workflowErr.Error())
	assert.True(t, errors.Is(workflowErr, execErr))
}
