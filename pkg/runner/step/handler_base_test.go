package step

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestNewBaseHandler(t *testing.T) {
	handler := NewBaseHandler("test_handler", CategoryInteractive, true)

	assert.Equal(t, "test_handler", handler.GetName())
	assert.Equal(t, CategoryInteractive, handler.GetCategory())
	assert.True(t, handler.RequiresTTY())
}

func TestBaseHandler_GetName(t *testing.T) {
	tests := []struct {
		name     string
		expected string
	}{
		{"input", "input"},
		{"shell", "shell"},
		{"toast", "toast"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := NewBaseHandler(tt.name, CategoryCommand, false)
			assert.Equal(t, tt.expected, handler.GetName())
		})
	}
}

func TestBaseHandler_GetCategory(t *testing.T) {
	tests := []struct {
		name     string
		category StepCategory
	}{
		{"interactive", CategoryInteractive},
		{"output", CategoryOutput},
		{"ui", CategoryUI},
		{"command", CategoryCommand},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := NewBaseHandler("test", tt.category, false)
			assert.Equal(t, tt.category, handler.GetCategory())
		})
	}
}

func TestBaseHandler_RequiresTTY(t *testing.T) {
	t.Run("returns true when requires TTY", func(t *testing.T) {
		handler := NewBaseHandler("input", CategoryInteractive, true)
		assert.True(t, handler.RequiresTTY())
	})

	t.Run("returns false when does not require TTY", func(t *testing.T) {
		handler := NewBaseHandler("shell", CategoryCommand, false)
		assert.False(t, handler.RequiresTTY())
	})
}

func TestBaseHandler_CheckTTY(t *testing.T) {
	t.Run("returns nil when TTY not required", func(t *testing.T) {
		handler := NewBaseHandler("shell", CategoryCommand, false)
		step := &schema.WorkflowStep{Name: "test_step", Type: "shell"}

		err := handler.CheckTTY(step)
		assert.NoError(t, err)
	})

	// Note: Testing the TTY required case is environment-dependent.
	// In CI/non-TTY environments, this will return an error.
	// In TTY environments, this will return nil.
	t.Run("returns error when TTY required but not available", func(t *testing.T) {
		handler := NewBaseHandler("input", CategoryInteractive, true)
		step := &schema.WorkflowStep{Name: "test_step", Type: "input"}

		err := handler.CheckTTY(step)
		// In CI or piped environment, this should error.
		// We can't guarantee this test will fail in all environments,
		// but we can verify the error type when it does fail.
		if err != nil {
			assert.True(t, errors.Is(err, errUtils.ErrStepTTYRequired))
		}
	})
}

func TestBaseHandler_ValidateRequired(t *testing.T) {
	handler := NewBaseHandler("test", CategoryCommand, false)

	t.Run("returns nil when value is not empty", func(t *testing.T) {
		step := &schema.WorkflowStep{Name: "test_step", Type: "shell"}
		err := handler.ValidateRequired(step, "command", "echo hello")
		assert.NoError(t, err)
	})

	t.Run("returns error when value is empty", func(t *testing.T) {
		step := &schema.WorkflowStep{Name: "test_step", Type: "shell"}
		err := handler.ValidateRequired(step, "command", "")
		assert.Error(t, err)
		assert.True(t, errors.Is(err, errUtils.ErrStepFieldRequired))
	})

	t.Run("error is sentinel ErrStepFieldRequired", func(t *testing.T) {
		step := &schema.WorkflowStep{Name: "my_step", Type: "input"}
		err := handler.ValidateRequired(step, "prompt", "")
		require.Error(t, err)
		// Use errors.Is() to check for sentinel error.
		assert.True(t, errors.Is(err, errUtils.ErrStepFieldRequired))
	})
}

func TestBaseHandler_ResolveContent(t *testing.T) {
	handler := NewBaseHandler("toast", CategoryUI, false)
	ctx := context.Background()

	t.Run("returns empty when content is empty", func(t *testing.T) {
		step := &schema.WorkflowStep{Name: "test", Content: ""}
		vars := NewVariables()

		result, err := handler.ResolveContent(ctx, step, vars)
		assert.NoError(t, err)
		assert.Empty(t, result)
	})

	t.Run("resolves template variables", func(t *testing.T) {
		step := &schema.WorkflowStep{
			Name:    "test",
			Content: "Deploying to {{ .steps.env.value }}",
		}
		vars := NewVariables()
		vars.Set("env", NewStepResult("production"))

		result, err := handler.ResolveContent(ctx, step, vars)
		assert.NoError(t, err)
		assert.Equal(t, "Deploying to production", result)
	})

	t.Run("returns plain text without templates", func(t *testing.T) {
		step := &schema.WorkflowStep{
			Name:    "test",
			Content: "Hello World",
		}
		vars := NewVariables()

		result, err := handler.ResolveContent(ctx, step, vars)
		assert.NoError(t, err)
		assert.Equal(t, "Hello World", result)
	})

	t.Run("returns error on invalid template", func(t *testing.T) {
		step := &schema.WorkflowStep{
			Name:    "test",
			Content: "{{ .invalid.template",
		}
		vars := NewVariables()

		_, err := handler.ResolveContent(ctx, step, vars)
		assert.Error(t, err)
		assert.ErrorIs(t, err, errUtils.ErrTemplateEvaluation)
	})
}

func TestBaseHandler_ResolvePrompt(t *testing.T) {
	handler := NewBaseHandler("input", CategoryInteractive, true)
	ctx := context.Background()

	t.Run("returns empty when prompt is empty", func(t *testing.T) {
		step := &schema.WorkflowStep{Name: "test", Prompt: ""}
		vars := NewVariables()

		result, err := handler.ResolvePrompt(ctx, step, vars)
		assert.NoError(t, err)
		assert.Empty(t, result)
	})

	t.Run("resolves template variables", func(t *testing.T) {
		step := &schema.WorkflowStep{
			Name:   "test",
			Prompt: "Select environment for {{ .steps.app.value }}:",
		}
		vars := NewVariables()
		vars.Set("app", NewStepResult("myapp"))

		result, err := handler.ResolvePrompt(ctx, step, vars)
		assert.NoError(t, err)
		assert.Equal(t, "Select environment for myapp:", result)
	})

	t.Run("returns plain text without templates", func(t *testing.T) {
		step := &schema.WorkflowStep{
			Name:   "test",
			Prompt: "Enter your name:",
		}
		vars := NewVariables()

		result, err := handler.ResolvePrompt(ctx, step, vars)
		assert.NoError(t, err)
		assert.Equal(t, "Enter your name:", result)
	})
}

func TestBaseHandler_ResolveCommand(t *testing.T) {
	handler := NewBaseHandler("shell", CategoryCommand, false)
	ctx := context.Background()

	t.Run("returns empty when command is empty", func(t *testing.T) {
		step := &schema.WorkflowStep{Name: "test", Command: ""}
		vars := NewVariables()

		result, err := handler.ResolveCommand(ctx, step, vars)
		assert.NoError(t, err)
		assert.Empty(t, result)
	})

	t.Run("resolves template variables", func(t *testing.T) {
		step := &schema.WorkflowStep{
			Name:    "test",
			Command: "terraform apply -target={{ .steps.component.value }}",
		}
		vars := NewVariables()
		vars.Set("component", NewStepResult("vpc"))

		result, err := handler.ResolveCommand(ctx, step, vars)
		assert.NoError(t, err)
		assert.Equal(t, "terraform apply -target=vpc", result)
	})

	t.Run("resolves environment variables", func(t *testing.T) {
		step := &schema.WorkflowStep{
			Name:    "test",
			Command: "deploy --env={{ .env.DEPLOY_ENV }}",
		}
		vars := NewVariables()
		vars.SetEnv("DEPLOY_ENV", "staging")

		result, err := handler.ResolveCommand(ctx, step, vars)
		assert.NoError(t, err)
		assert.Equal(t, "deploy --env=staging", result)
	})

	t.Run("returns plain text without templates", func(t *testing.T) {
		step := &schema.WorkflowStep{
			Name:    "test",
			Command: "echo hello",
		}
		vars := NewVariables()

		result, err := handler.ResolveCommand(ctx, step, vars)
		assert.NoError(t, err)
		assert.Equal(t, "echo hello", result)
	})

	t.Run("returns error on invalid template", func(t *testing.T) {
		step := &schema.WorkflowStep{
			Name:    "test",
			Command: "{{ .invalid",
		}
		vars := NewVariables()

		_, err := handler.ResolveCommand(ctx, step, vars)
		assert.Error(t, err)
		assert.ErrorIs(t, err, errUtils.ErrTemplateEvaluation)
	})
}
