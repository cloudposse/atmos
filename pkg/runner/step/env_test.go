package step

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

// EnvHandler registration and validation are tested in ui_handlers_test.go.
// This file tests the Execute method.

func TestEnvHandler_Execute(t *testing.T) {
	handler, ok := Get("env")
	require.True(t, ok)

	t.Run("sets single environment variable", func(t *testing.T) {
		step := &schema.WorkflowStep{
			Name: "test",
			Type: "env",
			Vars: map[string]string{
				"MY_VAR": "my_value",
			},
		}
		vars := NewVariables()
		ctx := context.Background()

		result, err := handler.Execute(ctx, step, vars)
		require.NoError(t, err)
		assert.NotNil(t, result)
		assert.Equal(t, "my_value", vars.Env["MY_VAR"])
	})

	t.Run("sets multiple environment variables", func(t *testing.T) {
		step := &schema.WorkflowStep{
			Name: "test",
			Type: "env",
			Vars: map[string]string{
				"VAR1": "value1",
				"VAR2": "value2",
				"VAR3": "value3",
			},
		}
		vars := NewVariables()
		ctx := context.Background()

		result, err := handler.Execute(ctx, step, vars)
		require.NoError(t, err)
		assert.NotNil(t, result)
		assert.Equal(t, "value1", vars.Env["VAR1"])
		assert.Equal(t, "value2", vars.Env["VAR2"])
		assert.Equal(t, "value3", vars.Env["VAR3"])
	})

	t.Run("resolves templates in values", func(t *testing.T) {
		step := &schema.WorkflowStep{
			Name: "test",
			Type: "env",
			Vars: map[string]string{
				"TARGET_ENV": "{{ .steps.selected_env.value }}",
			},
		}
		vars := NewVariables()
		vars.Set("selected_env", NewStepResult("production"))
		ctx := context.Background()

		result, err := handler.Execute(ctx, step, vars)
		require.NoError(t, err)
		assert.NotNil(t, result)
		assert.Equal(t, "production", vars.Env["TARGET_ENV"])
	})

	t.Run("returns error for invalid template", func(t *testing.T) {
		step := &schema.WorkflowStep{
			Name: "test",
			Type: "env",
			Vars: map[string]string{
				"BAD_VAR": "{{ .invalid",
			},
		}
		vars := NewVariables()
		ctx := context.Background()

		_, err := handler.Execute(ctx, step, vars)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to resolve env var BAD_VAR")
	})

	t.Run("overwrites existing env var", func(t *testing.T) {
		step := &schema.WorkflowStep{
			Name: "test",
			Type: "env",
			Vars: map[string]string{
				"EXISTING": "new_value",
			},
		}
		vars := NewVariables()
		vars.SetEnv("EXISTING", "old_value")
		ctx := context.Background()

		result, err := handler.Execute(ctx, step, vars)
		require.NoError(t, err)
		assert.NotNil(t, result)
		assert.Equal(t, "new_value", vars.Env["EXISTING"])
	})

	t.Run("returns empty result value", func(t *testing.T) {
		step := &schema.WorkflowStep{
			Name: "test",
			Type: "env",
			Vars: map[string]string{
				"MY_VAR": "my_value",
			},
		}
		vars := NewVariables()
		ctx := context.Background()

		result, err := handler.Execute(ctx, step, vars)
		require.NoError(t, err)
		assert.Empty(t, result.Value)
	})
}
