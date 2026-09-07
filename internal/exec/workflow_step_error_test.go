package exec

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	atmosansi "github.com/cloudposse/atmos/pkg/ansi"
)

func TestBuildWorkflowStepError(t *testing.T) {
	tests := []struct {
		name           string
		err            error
		ctx            *workflowStepErrorContext
		expectContains []string
		expectSentinel error
		expectExitCode int
	}{
		{
			name: "shell command failure with stack",
			err:  errors.New("command failed"),
			ctx: &workflowStepErrorContext{
				WorkflowPath:     "/path/to/stacks/workflows/deploy.yaml",
				WorkflowBasePath: "/path/to/stacks/workflows",
				Workflow:         "deploy-all",
				StepName:         "step1",
				Command:          "echo hello",
				CommandType:      "shell",
				FinalStack:       "dev-us-east-1",
			},
			expectContains: []string{"deploy-all", "step1", "echo hello", "dev-us-east-1"},
			expectSentinel: errUtils.ErrWorkflowStepFailed,
		},
		{
			name: "atmos command failure without stack",
			err:  errors.New("terraform failed"),
			ctx: &workflowStepErrorContext{
				WorkflowPath:     "/base/stacks/workflows/infra.yaml",
				WorkflowBasePath: "/base/stacks/workflows",
				Workflow:         "provision",
				StepName:         "terraform-step",
				Command:          "terraform plan vpc",
				CommandType:      "atmos",
				FinalStack:       "",
			},
			expectContains: []string{"provision", "terraform-step", "atmos terraform plan vpc"},
			expectSentinel: errUtils.ErrWorkflowStepFailed,
		},
		{
			name: "atmos command failure with stack",
			err:  errors.New("plan failed"),
			ctx: &workflowStepErrorContext{
				WorkflowPath:     "/workflows/deploy.yaml",
				WorkflowBasePath: "/workflows",
				Workflow:         "deploy",
				StepName:         "plan-step",
				Command:          "terraform plan mycomponent",
				CommandType:      "atmos",
				FinalStack:       "prod",
			},
			expectContains: []string{"deploy", "plan-step", "atmos", "prod"},
			expectSentinel: errUtils.ErrWorkflowStepFailed,
		},
		{
			name: "error with exit code",
			err:  errUtils.WithExitCode(errors.New("exit error"), 2),
			ctx: &workflowStepErrorContext{
				WorkflowPath:     "/workflows/test.yaml",
				WorkflowBasePath: "/workflows",
				Workflow:         "test-wf",
				StepName:         "failing-step",
				Command:          "exit 2",
				CommandType:      "shell",
				FinalStack:       "",
			},
			expectContains: []string{"test-wf", "failing-step"},
			expectSentinel: errUtils.ErrWorkflowStepFailed,
			expectExitCode: 2,
		},
		{
			name: "workflow file in nested directory",
			err:  errors.New("nested failure"),
			ctx: &workflowStepErrorContext{
				WorkflowPath:     "/base/stacks/workflows/team/project/deploy.yaml",
				WorkflowBasePath: "/base/stacks/workflows",
				Workflow:         "deploy",
				StepName:         "step1",
				Command:          "echo test",
				CommandType:      "shell",
				FinalStack:       "",
			},
			expectContains: []string{"deploy", "step1", "team/project/deploy"},
			expectSentinel: errUtils.ErrWorkflowStepFailed,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := buildWorkflowStepError(tt.err, tt.ctx)

			assert.Error(t, result)
			assert.ErrorIs(t, result, tt.expectSentinel)

			// Use Format to get the full formatted error including hints. Strip ANSI
			// since CI-enabled color rendering can split an expected substring (e.g. a
			// syntax-highlighted code fence) across separate escape-coded spans.
			formattedErr := atmosansi.Strip(errUtils.Format(result, errUtils.DefaultFormatterConfig()))
			for _, expected := range tt.expectContains {
				assert.Contains(t, formattedErr, expected)
			}

			if tt.expectExitCode > 0 {
				exitCode := errUtils.GetExitCode(result)
				assert.Equal(t, tt.expectExitCode, exitCode)
			}
		})
	}
}

// TestBuildWorkflowStepErrorPreservesInnerHintsAndContext reproduces a bug
// found while field-testing the working-directory fix: a handler-level error
// built via errUtils.Build(...).WithContext(...).WithHint(...).Err() (e.g.
// ResolveInWorkingDirectory's template-evaluation error) lost its hint and
// context entirely once wrapped as a workflow step failure, even with
// --verbose. Root cause: buildWorkflowStepError used to dual-wrap the error
// with a raw fmt.Errorf before handing it to errUtils.Build, and
// cockroachdb/errors' hint/safe-detail extraction treats a Go 1.20 multi-error
// (Unwrap() []error) as an opaque leaf node, never reaching the decorations
// further down the chain.
func TestBuildWorkflowStepErrorPreservesInnerHintsAndContext(t *testing.T) {
	innerErr := errUtils.Build(errUtils.ErrTemplateEvaluation).
		WithCause(errors.New("template: step-pass-1:1: unclosed action")).
		WithHint("Check the working_directory template for a missing closing '}}'").
		WithContext("step", "pack").
		WithContext("field", "working_directory").
		Err()

	result := buildWorkflowStepError(innerErr, &workflowStepErrorContext{
		WorkflowPath:     "/workflows/deploy.yaml",
		WorkflowBasePath: "/workflows",
		Workflow:         "deploy",
		StepName:         "pack",
		Command:          "echo hi",
		CommandType:      "shell",
	})

	require.Error(t, result)
	assert.ErrorIs(t, result, errUtils.ErrWorkflowStepFailed)
	assert.ErrorIs(t, result, errUtils.ErrTemplateEvaluation)

	formatted := atmosansi.Strip(errUtils.Format(result, errUtils.FormatterConfig{Verbose: true}))
	assert.Contains(t, formatted, "Check the working_directory template for a missing closing", "inner hint must survive the workflow step wrapping")
	assert.Contains(t, formatted, "## Context", "inner context section must render")
	// Context renders as a "Key │ Value" markdown table, so assert the keys and
	// values independently rather than an exact formatted row.
	assert.Contains(t, formatted, "field", "inner context key must survive the workflow step wrapping")
	assert.Contains(t, formatted, "working_directory", "inner context value must survive the workflow step wrapping")
	assert.Contains(t, formatted, "step", "inner context key must survive the workflow step wrapping")
	assert.Contains(t, formatted, "pack", "inner context value must survive the workflow step wrapping")
}
