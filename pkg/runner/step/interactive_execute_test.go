package step

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

// Interactive step handlers require a TTY. Under `go test` there is no TTY, so
// Execute must short-circuit via BaseHandler.CheckTTY and return
// ErrStepTTYRequired before attempting to run a huh form. This verifies that
// guard for every interactive handler.
func TestInteractiveHandlers_ExecuteWithoutTTY(t *testing.T) {
	handlers := []string{"confirm", "write", "input", "choose", "filter", "file"}

	for _, name := range handlers {
		t.Run(name, func(t *testing.T) {
			handler, ok := Get(name)
			require.True(t, ok, "handler %q should be registered", name)
			require.True(t, handler.RequiresTTY(), "handler %q should require a TTY", name)

			step := &schema.WorkflowStep{
				Name:    name + "-step",
				Type:    name,
				Prompt:  "Continue?",
				Options: []string{"a", "b"},
			}

			_, err := handler.Execute(context.Background(), step, NewVariables())

			require.Error(t, err)
			assert.ErrorIs(t, err, errUtils.ErrStepTTYRequired,
				"handler %q Execute should fail with ErrStepTTYRequired when no TTY", name)
		})
	}
}
