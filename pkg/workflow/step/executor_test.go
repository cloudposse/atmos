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
		{"success", true},
		{"info", true},
		{"warn", true},
		{"error", true},
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
			Type:    "success",
			Content: "Success message",
		}

		err := ValidateStep(step)
		assert.NoError(t, err)
	})

	t.Run("returns error for invalid step", func(t *testing.T) {
		step := &schema.WorkflowStep{
			Name: "test",
			Type: "success",
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
				{Name: "step1", Type: "success", Content: "Step 1"},
				{Name: "step2", Type: "info", Content: "Step 2"},
			},
		}

		errors := ValidateWorkflow(workflow)
		assert.Empty(t, errors)
	})

	t.Run("returns errors for invalid steps", func(t *testing.T) {
		workflow := &schema.WorkflowDefinition{
			Description: "Test workflow",
			Steps: []schema.WorkflowStep{
				{Name: "step1", Type: "success"}, // Missing content.
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
				{Type: "success", Content: "No name"},
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
	assert.Contains(t, types[CategoryUI], "success")
	assert.Contains(t, types[CategoryUI], "error")
	assert.Contains(t, types[CategoryCommand], "atmos")
	assert.Contains(t, types[CategoryCommand], "shell")
}
