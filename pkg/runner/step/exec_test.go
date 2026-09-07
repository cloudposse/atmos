package step

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

func TestExecHandlerValidate(t *testing.T) {
	handler := &ExecHandler{}

	t.Run("missing command is an error", func(t *testing.T) {
		step := &schema.WorkflowStep{Name: "replace"}
		err := handler.Validate(step)
		require.Error(t, err)
	})

	t.Run("present command passes", func(t *testing.T) {
		step := &schema.WorkflowStep{Name: "replace", Command: "atmos version"}
		require.NoError(t, handler.Validate(step))
	})
}

func TestExecHandlerExecEnv(t *testing.T) {
	handler := &ExecHandler{}

	t.Run("no step env returns vars env unchanged", func(t *testing.T) {
		vars := NewVariables()
		vars.SetEnv("EXISTING", "value")
		step := &schema.WorkflowStep{Name: "replace"}

		env, err := handler.execEnv(step, vars)
		require.NoError(t, err)
		assert.Equal(t, vars.EnvSlice(), env)
	})

	t.Run("templated step env resolves and merges", func(t *testing.T) {
		vars := NewVariables()
		vars.SetEnv("BASE", "base-value")
		step := &schema.WorkflowStep{
			Name: "replace",
			Env:  map[string]string{"DERIVED": "{{ .env.BASE }}-derived"},
		}

		env, err := handler.execEnv(step, vars)
		require.NoError(t, err)
		assert.Contains(t, env, "DERIVED=base-value-derived")
		assert.Contains(t, env, "BASE=base-value")
	})

	t.Run("bad template in step env returns wrapped error", func(t *testing.T) {
		vars := NewVariables()
		step := &schema.WorkflowStep{
			Name: "replace",
			Env:  map[string]string{"BROKEN": "{{ .missing"},
		}

		_, err := handler.execEnv(step, vars)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "replace")
	})
}

func TestExecHandlerExecuteResolutionErrors(t *testing.T) {
	handler := &ExecHandler{}

	t.Run("bad template in command", func(t *testing.T) {
		vars := NewVariables()
		step := &schema.WorkflowStep{Name: "replace", Command: "{{ .missing"}

		_, err := handler.Execute(context.Background(), step, vars)
		require.Error(t, err)
	})

	t.Run("bad template in working_directory", func(t *testing.T) {
		vars := NewVariables()
		step := &schema.WorkflowStep{
			Name:             "replace",
			Command:          "atmos version",
			WorkingDirectory: "{{ .missing",
		}

		_, err := handler.Execute(context.Background(), step, vars)
		require.Error(t, err)
	})

	t.Run("bad template in env", func(t *testing.T) {
		vars := NewVariables()
		step := &schema.WorkflowStep{
			Name:    "replace",
			Command: "atmos version",
			Env:     map[string]string{"BROKEN": "{{ .missing"},
		}

		_, err := handler.Execute(context.Background(), step, vars)
		require.Error(t, err)
	})
}
