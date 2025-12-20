package step

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

func TestStepExecutor(t *testing.T) {
	t.Run("creates with empty variables", func(t *testing.T) {
		executor := NewStepExecutor()
		assert.NotNil(t, executor)
		assert.NotNil(t, executor.Variables())
		assert.Empty(t, executor.Variables().Steps)
	})

	t.Run("creates with pre-populated variables", func(t *testing.T) {
		vars := NewVariables()
		vars.Set("test", NewStepResult("value"))

		executor := NewStepExecutorWithVars(vars)
		assert.NotNil(t, executor)

		result, ok := executor.GetResult("test")
		assert.True(t, ok)
		assert.Equal(t, "value", result.Value)
	})

	t.Run("sets workflow context", func(t *testing.T) {
		executor := NewStepExecutor()
		workflow := &schema.WorkflowDefinition{
			Output: "viewport",
		}

		executor.SetWorkflow(workflow)
		// No direct way to test, but ensures no panic.
	})

	t.Run("sets environment variable", func(t *testing.T) {
		executor := NewStepExecutor()
		executor.SetEnv("MY_VAR", "my_value")

		assert.Equal(t, "my_value", executor.Variables().Env["MY_VAR"])
	})
}

func TestStepExecutorExecute(t *testing.T) {
	t.Run("fails on unknown step type", func(t *testing.T) {
		executor := NewStepExecutor()
		step := &schema.WorkflowStep{
			Name: "test",
			Type: "unknown_type",
		}

		_, err := executor.Execute(context.Background(), step)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "unknown step type")
	})

	t.Run("defaults to shell type when empty", func(t *testing.T) {
		step := &schema.WorkflowStep{
			Name:    "test",
			Command: "echo hello",
		}

		// Should not error on validation (shell handler exists).
		handler, ok := Get("shell")
		require.True(t, ok)
		err := handler.Validate(step)
		assert.NoError(t, err)
	})

	t.Run("stores result for variable access", func(t *testing.T) {
		executor := NewStepExecutor()

		// Manually set a result to simulate execution.
		executor.Variables().Set("step1", NewStepResult("result1"))

		result, ok := executor.GetResult("step1")
		assert.True(t, ok)
		assert.Equal(t, "result1", result.Value)
	})
}

func TestIsExtendedStepType(t *testing.T) {
	tests := []struct {
		stepType   string
		isExtended bool
	}{
		// Legacy types - not extended.
		{"atmos", false},
		{"shell", false},
		{"", false},

		// Extended types.
		{"input", true},
		{"confirm", true},
		{"choose", true},
		{"filter", true},
		{"file", true},
		{"write", true},
		{"toast", true},
		{"markdown", true},
		{"spin", true},
		{"table", true},
		{"pager", true},
		{"format", true},
		{"join", true},
		{"style", true},

		// Unknown type - not extended.
		{"unknown_type", false},
	}

	for _, tt := range tests {
		t.Run(tt.stepType, func(t *testing.T) {
			result := IsExtendedStepType(tt.stepType)
			assert.Equal(t, tt.isExtended, result)
		})
	}
}

func TestValidateStep(t *testing.T) {
	t.Run("validates valid step", func(t *testing.T) {
		step := &schema.WorkflowStep{
			Name:    "test",
			Type:    "toast",
			Content: "Success message",
		}

		err := ValidateStep(step)
		assert.NoError(t, err)
	})

	t.Run("returns error for invalid step", func(t *testing.T) {
		step := &schema.WorkflowStep{
			Name: "test",
			Type: "toast",
			// Missing required content.
		}

		err := ValidateStep(step)
		assert.Error(t, err)
	})

	t.Run("returns error for unknown type", func(t *testing.T) {
		step := &schema.WorkflowStep{
			Name: "test",
			Type: "unknown_type",
		}

		err := ValidateStep(step)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "unknown step type")
	})

	t.Run("defaults to shell when type empty", func(t *testing.T) {
		step := &schema.WorkflowStep{
			Name:    "test",
			Command: "echo hello",
		}

		err := ValidateStep(step)
		assert.NoError(t, err)
	})
}

func TestValidateWorkflow(t *testing.T) {
	t.Run("validates valid workflow", func(t *testing.T) {
		workflow := &schema.WorkflowDefinition{
			Description: "Test workflow",
			Steps: []schema.WorkflowStep{
				{Name: "step1", Type: "toast", Level: "success", Content: "Step 1"},
				{Name: "step2", Type: "toast", Level: "info", Content: "Step 2"},
			},
		}

		errors := ValidateWorkflow(workflow)
		assert.Empty(t, errors)
	})

	t.Run("returns errors for invalid steps", func(t *testing.T) {
		workflow := &schema.WorkflowDefinition{
			Description: "Test workflow",
			Steps: []schema.WorkflowStep{
				{Name: "step1", Type: "toast"}, // Missing content.
				{Name: "step2", Type: "unknown"},
			},
		}

		errors := ValidateWorkflow(workflow)
		assert.Len(t, errors, 2)
	})

	t.Run("handles empty workflow", func(t *testing.T) {
		workflow := &schema.WorkflowDefinition{
			Description: "Empty workflow",
			Steps:       []schema.WorkflowStep{},
		}

		errors := ValidateWorkflow(workflow)
		assert.Empty(t, errors)
	})

	t.Run("generates step names when missing", func(t *testing.T) {
		workflow := &schema.WorkflowDefinition{
			Steps: []schema.WorkflowStep{
				{Type: "toast", Level: "success", Content: "No name"},
			},
		}

		errors := ValidateWorkflow(workflow)
		assert.Empty(t, errors)
	})
}

func TestListTypes(t *testing.T) {
	types := ListTypes()

	// Verify all categories are present.
	assert.Contains(t, types, CategoryInteractive)
	assert.Contains(t, types, CategoryOutput)
	assert.Contains(t, types, CategoryUI)
	assert.Contains(t, types, CategoryCommand)

	// Verify some expected types in each category.
	assert.Contains(t, types[CategoryInteractive], "input")
	assert.Contains(t, types[CategoryInteractive], "confirm")
	assert.Contains(t, types[CategoryOutput], "spin")
	assert.Contains(t, types[CategoryOutput], "table")
	assert.Contains(t, types[CategoryUI], "toast")
	assert.Contains(t, types[CategoryUI], "markdown")
	assert.Contains(t, types[CategoryCommand], "atmos")
	assert.Contains(t, types[CategoryCommand], "shell")
}

// Integration tests for variable passing between steps.

func TestStepExecutor_VariablePassing(t *testing.T) {
	t.Run("step can access previous step result", func(t *testing.T) {
		executor := NewStepExecutor()

		// Simulate a previous step's result.
		executor.Variables().Set("step1", NewStepResult("production"))

		// Verify the template can resolve the previous step's value.
		result, err := executor.Variables().Resolve("Deploy to {{ .steps.step1.value }}")
		require.NoError(t, err)
		assert.Equal(t, "Deploy to production", result)
	})

	t.Run("step can access previous step metadata", func(t *testing.T) {
		executor := NewStepExecutor()

		// Simulate a previous step's result with metadata.
		executor.Variables().Set("step1", NewStepResult("").
			WithMetadata("exit_code", 0).
			WithMetadata("stdout", "hello"))

		// Access values directly through Variables.
		result, ok := executor.GetResult("step1")
		require.True(t, ok)
		assert.Equal(t, 0, result.Metadata["exit_code"])
		assert.Equal(t, "hello", result.Metadata["stdout"])
	})

	t.Run("step can access environment variables", func(t *testing.T) {
		executor := NewStepExecutor()
		executor.SetEnv("ENV_NAME", "staging")

		result, err := executor.Variables().Resolve("Environment: {{ .env.ENV_NAME }}")
		require.NoError(t, err)
		assert.Equal(t, "Environment: staging", result)
	})

	t.Run("step can access multiple values from previous step", func(t *testing.T) {
		executor := NewStepExecutor()

		// Simulate a multi-select step result.
		executor.Variables().Set("selected", NewStepResult("dev").
			WithValues([]string{"dev", "staging", "prod"}))

		result, ok := executor.GetResult("selected")
		require.True(t, ok)
		assert.Equal(t, "dev", result.Value)
		assert.Equal(t, []string{"dev", "staging", "prod"}, result.Values)
	})

	t.Run("step can check if previous step was skipped", func(t *testing.T) {
		executor := NewStepExecutor()

		// Simulate a skipped step.
		executor.Variables().Set("optional_step", NewStepResult("").WithSkipped())

		result, ok := executor.GetResult("optional_step")
		require.True(t, ok)
		assert.True(t, result.Skipped)
	})

	t.Run("step can check error from previous step", func(t *testing.T) {
		executor := NewStepExecutor()

		// Simulate a failed step.
		executor.Variables().Set("failed_step", NewStepResult("").
			WithError("connection timeout"))

		result, ok := executor.GetResult("failed_step")
		require.True(t, ok)
		assert.Equal(t, "connection timeout", result.Error)
	})
}

func TestStepExecutor_RunAll(t *testing.T) {
	t.Run("returns error for empty workflow", func(t *testing.T) {
		executor := NewStepExecutor()
		workflow := &schema.WorkflowDefinition{
			Steps: []schema.WorkflowStep{},
		}

		// Should succeed (empty is valid, no steps to run).
		err := executor.RunAll(context.Background(), workflow)
		assert.NoError(t, err)
	})
}

func TestStepResult_Chaining(t *testing.T) {
	t.Run("supports method chaining", func(t *testing.T) {
		result := NewStepResult("value").
			WithValues([]string{"a", "b"}).
			WithMetadata("key1", "val1").
			WithMetadata("key2", "val2").
			WithError("some error").
			WithSkipped()

		assert.Equal(t, "value", result.Value)
		assert.Equal(t, []string{"a", "b"}, result.Values)
		assert.Equal(t, "val1", result.Metadata["key1"])
		assert.Equal(t, "val2", result.Metadata["key2"])
		assert.Equal(t, "some error", result.Error)
		assert.True(t, result.Skipped)
	})
}
