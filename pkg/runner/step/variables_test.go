package step

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestVariablesGetValue(t *testing.T) {
	vars := NewVariables()
	vars.Set("test_step", NewStepResult("test_value"))

	t.Run("returns value when step exists", func(t *testing.T) {
		value, ok := vars.GetValue("test_step")
		assert.True(t, ok)
		assert.Equal(t, "test_value", value)
	})

	t.Run("returns false when step does not exist", func(t *testing.T) {
		value, ok := vars.GetValue("non_existent")
		assert.False(t, ok)
		assert.Empty(t, value)
	})
}

func TestVariablesGetValues(t *testing.T) {
	vars := NewVariables()
	vars.Set("multi_step", NewStepResult("").WithValues([]string{"a", "b", "c"}))
	vars.Set("single_step", NewStepResult("value"))

	t.Run("returns values when step exists with multiple values", func(t *testing.T) {
		values, ok := vars.GetValues("multi_step")
		assert.True(t, ok)
		assert.Equal(t, []string{"a", "b", "c"}, values)
	})

	t.Run("returns empty values when step has single value", func(t *testing.T) {
		values, ok := vars.GetValues("single_step")
		assert.True(t, ok)
		assert.Empty(t, values)
	})

	t.Run("returns false when step does not exist", func(t *testing.T) {
		values, ok := vars.GetValues("non_existent")
		assert.False(t, ok)
		assert.Nil(t, values)
	})
}

func TestVariablesSetEnv(t *testing.T) {
	vars := NewVariables()

	vars.SetEnv("MY_VAR", "my_value")
	vars.SetEnv("ANOTHER_VAR", "another_value")

	assert.Equal(t, "my_value", vars.Env["MY_VAR"])
	assert.Equal(t, "another_value", vars.Env["ANOTHER_VAR"])
}

func TestVariablesTemplateData(t *testing.T) {
	vars := NewVariables()
	vars.Set("step1", NewStepResult("value1").
		WithValues([]string{"a", "b"}).
		WithMetadata("key", "meta_value").
		WithSkipped())
	vars.SetEnv("ENV_VAR", "env_value")

	// Access templateData indirectly through Resolve.
	result, err := vars.Resolve("{{ .steps.step1.value }}-{{ .steps.step1.skipped }}-{{ .env.ENV_VAR }}")
	require.NoError(t, err)
	assert.Equal(t, "value1-true-env_value", result)
}

func TestVariablesResolveExecutionError(t *testing.T) {
	vars := NewVariables()

	// Template that has invalid syntax.
	_, err := vars.Resolve("{{ range .steps }}{{ . }}")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to parse template")
}

func TestVariablesResolveEnvMapNil(t *testing.T) {
	vars := NewVariables()

	result, err := vars.ResolveEnvMap(nil)
	assert.NoError(t, err)
	assert.Nil(t, result)
}

func TestVariablesResolveEnvMapError(t *testing.T) {
	vars := NewVariables()

	// Use invalid template syntax instead of undefined fields.
	envMap := map[string]string{
		"GOOD":    "static",
		"BAD_VAR": "{{ range .steps }}{{ . }}",
	}

	_, err := vars.ResolveEnvMap(envMap)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to resolve env var BAD_VAR")
}

//nolint:dupl // Similar test patterns for different handler methods.
func TestBaseHandlerResolveContent(t *testing.T) {
	handler := NewBaseHandler("test", CategoryUI, false)
	vars := NewVariables()
	vars.Set("name", NewStepResult("world"))

	t.Run("resolves template in content", func(t *testing.T) {
		step := &schema.WorkflowStep{
			Name:    "test_step",
			Content: "Hello {{ .steps.name.value }}",
		}

		result, err := handler.ResolveContent(context.Background(), step, vars)
		require.NoError(t, err)
		assert.Equal(t, "Hello world", result)
	})

	t.Run("returns empty for empty content", func(t *testing.T) {
		step := &schema.WorkflowStep{
			Name:    "test_step",
			Content: "",
		}

		result, err := handler.ResolveContent(context.Background(), step, vars)
		require.NoError(t, err)
		assert.Empty(t, result)
	})

	t.Run("returns error for invalid template", func(t *testing.T) {
		step := &schema.WorkflowStep{
			Name:    "test_step",
			Content: "{{ .invalid",
		}

		_, err := handler.ResolveContent(context.Background(), step, vars)
		assert.Error(t, err)
		assert.ErrorIs(t, err, errUtils.ErrTemplateEvaluation)
	})
}

//nolint:dupl // Similar test patterns for different handler methods.
func TestBaseHandlerResolvePrompt(t *testing.T) {
	handler := NewBaseHandler("test", CategoryInteractive, true)
	vars := NewVariables()
	vars.Set("action", NewStepResult("deploy"))

	t.Run("resolves template in prompt", func(t *testing.T) {
		step := &schema.WorkflowStep{
			Name:   "test_step",
			Prompt: "Do you want to {{ .steps.action.value }}?",
		}

		result, err := handler.ResolvePrompt(context.Background(), step, vars)
		require.NoError(t, err)
		assert.Equal(t, "Do you want to deploy?", result)
	})

	t.Run("returns empty for empty prompt", func(t *testing.T) {
		step := &schema.WorkflowStep{
			Name:   "test_step",
			Prompt: "",
		}

		result, err := handler.ResolvePrompt(context.Background(), step, vars)
		require.NoError(t, err)
		assert.Empty(t, result)
	})

	t.Run("returns error for invalid template", func(t *testing.T) {
		step := &schema.WorkflowStep{
			Name:   "test_step",
			Prompt: "{{ .invalid",
		}

		_, err := handler.ResolvePrompt(context.Background(), step, vars)
		assert.Error(t, err)
		assert.ErrorIs(t, err, errUtils.ErrTemplateEvaluation)
	})
}

func TestBaseHandlerResolveCommand(t *testing.T) {
	handler := NewBaseHandler("test", CategoryCommand, false)
	vars := NewVariables()
	vars.Set("env", NewStepResult("production"))
	vars.Set("component", NewStepResult("vpc"))

	t.Run("resolves template in command", func(t *testing.T) {
		step := &schema.WorkflowStep{
			Name:    "test_step",
			Command: "atmos terraform apply {{ .steps.component.value }} -s {{ .steps.env.value }}",
		}

		result, err := handler.ResolveCommand(context.Background(), step, vars)
		require.NoError(t, err)
		assert.Equal(t, "atmos terraform apply vpc -s production", result)
	})

	t.Run("returns empty for empty command", func(t *testing.T) {
		step := &schema.WorkflowStep{
			Name:    "test_step",
			Command: "",
		}

		result, err := handler.ResolveCommand(context.Background(), step, vars)
		require.NoError(t, err)
		assert.Empty(t, result)
	})

	t.Run("returns error for invalid template", func(t *testing.T) {
		step := &schema.WorkflowStep{
			Name:    "test_step",
			Command: "echo {{ .invalid",
		}

		_, err := handler.ResolveCommand(context.Background(), step, vars)
		assert.Error(t, err)
		assert.ErrorIs(t, err, errUtils.ErrTemplateEvaluation)
	})
}

func TestBaseHandlerValidateRequired(t *testing.T) {
	handler := NewBaseHandler("test", CategoryUI, false)

	t.Run("returns nil for non-empty value", func(t *testing.T) {
		step := &schema.WorkflowStep{
			Name: "test_step",
			Type: "test",
		}

		err := handler.ValidateRequired(step, "content", "some content")
		assert.NoError(t, err)
	})

	t.Run("returns error for empty value", func(t *testing.T) {
		step := &schema.WorkflowStep{
			Name: "test_step",
			Type: "test",
		}

		err := handler.ValidateRequired(step, "content", "")
		assert.Error(t, err)
		assert.ErrorIs(t, err, errUtils.ErrStepFieldRequired)
	})
}

// Note: TestRegistryReset is not included because Reset() would break other tests
// since handlers are registered via init() and cannot be easily restored.
// The Reset() function is primarily for testing infrastructure.

func TestStepResultMetadataNilMap(t *testing.T) {
	// Create result without using NewStepResult to test nil metadata handling.
	result := &StepResult{
		Value:    "test",
		Metadata: nil,
	}

	// WithMetadata should create the map if nil.
	result = result.WithMetadata("key", "value")
	assert.NotNil(t, result.Metadata)
	assert.Equal(t, "value", result.Metadata["key"])
}

func TestVariablesStageTracking(t *testing.T) {
	t.Run("total stages defaults to zero", func(t *testing.T) {
		vars := NewVariables()
		assert.Equal(t, 0, vars.GetTotalStages())
	})

	t.Run("set and get total stages", func(t *testing.T) {
		vars := NewVariables()
		vars.SetTotalStages(5)
		assert.Equal(t, 5, vars.GetTotalStages())
	})

	t.Run("stage index defaults to zero", func(t *testing.T) {
		vars := NewVariables()
		assert.Equal(t, 0, vars.GetStageIndex())
	})

	t.Run("increment stage index", func(t *testing.T) {
		vars := NewVariables()
		assert.Equal(t, 0, vars.GetStageIndex())

		// First increment returns 1.
		idx := vars.IncrementStageIndex()
		assert.Equal(t, 1, idx)
		assert.Equal(t, 1, vars.GetStageIndex())

		// Second increment returns 2.
		idx = vars.IncrementStageIndex()
		assert.Equal(t, 2, idx)
		assert.Equal(t, 2, vars.GetStageIndex())

		// Third increment returns 3.
		idx = vars.IncrementStageIndex()
		assert.Equal(t, 3, idx)
		assert.Equal(t, 3, vars.GetStageIndex())
	})

	t.Run("stage tracking together", func(t *testing.T) {
		vars := NewVariables()
		vars.SetTotalStages(3)

		// Simulate stage progression.
		assert.Equal(t, 1, vars.IncrementStageIndex())
		assert.Equal(t, 2, vars.IncrementStageIndex())
		assert.Equal(t, 3, vars.IncrementStageIndex())

		// Final state.
		assert.Equal(t, 3, vars.GetStageIndex())
		assert.Equal(t, 3, vars.GetTotalStages())
	})
}

func TestVariablesLoadOSEnv(t *testing.T) {
	// Set test environment variables for portability.
	t.Setenv("PATH", "test-path")
	t.Setenv("HOME", "test-home")

	// NewVariables automatically loads OS env.
	vars := NewVariables()

	// Verify test environment variables are loaded.
	path, ok := vars.Env["PATH"]
	assert.True(t, ok, "PATH should be loaded from OS environment")
	assert.Equal(t, "test-path", path)

	home, ok := vars.Env["HOME"]
	assert.True(t, ok, "HOME should be loaded from OS environment")
	assert.Equal(t, "test-home", home)
}

func TestVariablesResolveWithEnv(t *testing.T) {
	vars := NewVariables()
	vars.SetEnv("CUSTOM_VAR", "custom_value")

	result, err := vars.Resolve("env value is {{ .env.CUSTOM_VAR }}")
	require.NoError(t, err)
	assert.Equal(t, "env value is custom_value", result)
}

func TestVariablesResolveMetadata(t *testing.T) {
	vars := NewVariables()
	vars.Set("step1", NewStepResult("value").WithMetadata("exit_code", 0))

	result, err := vars.Resolve("exit code: {{ .steps.step1.metadata.exit_code }}")
	require.NoError(t, err)
	assert.Equal(t, "exit code: 0", result)
}

func TestVariablesResolveError(t *testing.T) {
	vars := NewVariables()
	vars.Set("step1", NewStepResult("").WithError("something failed"))

	result, err := vars.Resolve("error: {{ .steps.step1.error }}")
	require.NoError(t, err)
	assert.Equal(t, "error: something failed", result)
}
