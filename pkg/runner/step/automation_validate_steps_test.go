package step

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/automation"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestAutomationLibraryValidateSteps(t *testing.T) {
	library := NewAutomationLibrary(NewVariables(), nil)

	require.NoError(t, library.ValidateSteps(schema.Tasks{{Name: "a", Type: "shell", Command: "echo a"}, {Name: "b", Type: "shell", Command: "echo b"}}))
	require.NoError(t, library.ValidateSteps(nil))
	require.ErrorIs(t, library.ValidateSteps(schema.Tasks{{Type: "not-a-step"}}), errUtils.ErrUnknownStepType)
	require.ErrorIs(t, library.ValidateSteps(schema.Tasks{
		{Name: "same", Type: "shell", Command: "echo 1"},
		{Name: "same", Type: "shell", Command: "echo 2"},
	}), errUtils.ErrAutomation)
}

func TestAutomationLibraryRejectsRecordedLoadErrorsBeforeExecution(t *testing.T) {
	for _, cause := range []error{schema.ErrTaskUnknownField, schema.ErrInvalidRetryConfig} {
		t.Run(cause.Error(), func(t *testing.T) {
			library := NewAutomationLibrary(NewVariables(), nil)
			tasks := schema.Tasks{
				// If preflight misses the later decode failure, this step would fail first.
				{Name: "must-not-run", Type: "script", Interpreter: "starlark", Script: `fail("ran before validation")`},
				{Name: "invalid", Type: "shell", Command: "unused", LoadError: fmt.Errorf("invalid step field: %w", cause)},
			}
			require.ErrorIs(t, library.ValidateSteps(tasks), cause)
			require.ErrorIs(t, library.RunSteps(context.Background(), tasks, &automation.StepCall{}), cause)
		})
	}
}
