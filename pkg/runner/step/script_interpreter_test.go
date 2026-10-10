package step

import (
	"context"
	"errors"
	"fmt"
	osexec "os/exec"
	"testing"

	cockroach "github.com/cockroachdb/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/script"
)

const interpreterHint = "Did you mean `starlark`? Embedded interpreter names are case-sensitive."

// hintsOf returns the raw hint texts attached to err, before any markdown rendering.
func hintsOf(err error) []string {
	return cockroach.GetAllHints(err)
}

func TestWrapScriptInterpreterError(t *testing.T) {
	notFound := fmt.Errorf("%w: %w", errUtils.ErrProcessWaitFailed, &osexec.Error{Name: "Starlark", Err: osexec.ErrNotFound})

	t.Run("wrong case of an embedded interpreter gets a hint", func(t *testing.T) {
		wrapped := WrapScriptInterpreterError("Starlark", notFound)
		require.ErrorIs(t, wrapped, osexec.ErrNotFound)
		assert.Contains(t, hintsOf(wrapped), interpreterHint)
	})

	t.Run("surrounding whitespace and upper case are still matched", func(t *testing.T) {
		wrapped := WrapScriptInterpreterError("  STARLARK ", notFound)
		assert.Contains(t, hintsOf(wrapped), interpreterHint)
	})

	t.Run("exact embedded name is left alone", func(t *testing.T) {
		assert.Equal(t, notFound, WrapScriptInterpreterError("starlark", notFound))
	})

	t.Run("unknown interpreter is left alone", func(t *testing.T) {
		assert.Equal(t, notFound, WrapScriptInterpreterError("Nonesuch", notFound))
	})

	t.Run("failures other than not found are left alone", func(t *testing.T) {
		other := errors.New("exit status 1")
		assert.Equal(t, other, WrapScriptInterpreterError("Starlark", other))
	})

	t.Run("nil stays nil", func(t *testing.T) {
		assert.NoError(t, WrapScriptInterpreterError("Starlark", nil))
	})
}

func TestScriptHandlerWrongCaseInterpreterHint(t *testing.T) {
	initShellTestIO(t)
	handler, ok := Get(schema.TaskTypeScript)
	require.True(t, ok)

	_, err := handler.Execute(context.Background(), &schema.WorkflowStep{
		Name: "case", Type: schema.TaskTypeScript, Interpreter: "Starlark", Script: "print('hi')", Output: "capture",
	}, NewVariables())

	require.Error(t, err)
	assert.Contains(t, hintsOf(err), interpreterHint)
}

func TestScriptHandlerRejectsTemplatedEmbeddedInterpreterUnderContainer(t *testing.T) {
	initShellTestIO(t)
	handler, ok := Get(schema.TaskTypeScript)
	require.True(t, ok)
	step := &schema.WorkflowStep{
		Name: "templated", Type: schema.TaskTypeScript, Interpreter: `{{ printf "starlark" }}`, Script: "print('hi')",
		Container: &schema.WorkflowContainer{Image: "example"},
	}

	// Validate cannot know a templated name is embedded; execution checks the rendered one.
	require.NoError(t, handler.Validate(step))
	_, err := handler.Execute(context.Background(), step, NewVariables())
	require.ErrorIs(t, err, errUtils.ErrScript)

	// Negative path: the same templated interpreter without a container runs on the host.
	step.Container = nil
	_, err = handler.Execute(context.Background(), step, NewVariables())
	require.NoError(t, err)
}

func TestScriptComponentHelpersAreNilSafe(t *testing.T) {
	assert.Nil(t, ScriptComponentRef(nil))
	assert.Nil(t, ScriptComponentResolver(nil))
	assert.Nil(t, ScriptComponentRef(NewVariables()), "no component in scope")
	_, err := ScriptComponentResolver(NewVariables())(context.Background(), script.ComponentRef{Name: "a", Stack: "b", Type: "c"})
	require.ErrorIs(t, err, errUtils.ErrScript)
}
