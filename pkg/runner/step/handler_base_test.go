package step

import (
	"context"
	"errors"
	"os"
	"path/filepath"
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

func TestBaseHandler_ResolveInWorkingDirectory(t *testing.T) {
	handler := NewBaseHandler("archive", CategoryCommand, false)

	t.Run("empty value short-circuits without resolving working directory", func(t *testing.T) {
		step := &schema.WorkflowStep{Name: "test", WorkingDirectory: "{{ .invalid"}
		vars := NewVariables()

		result, err := handler.ResolveInWorkingDirectory(step, vars, "", "source")
		assert.NoError(t, err)
		assert.Empty(t, result)
	})

	t.Run("relative value joins against working directory", func(t *testing.T) {
		workDir := t.TempDir()
		step := &schema.WorkflowStep{Name: "test", WorkingDirectory: workDir}
		vars := NewVariables()

		result, err := handler.ResolveInWorkingDirectory(step, vars, "sub/file.txt", "source")
		assert.NoError(t, err)
		assert.Equal(t, filepath.Join(workDir, "sub", "file.txt"), result)
	})

	t.Run("absolute value passes through unchanged", func(t *testing.T) {
		absValue := filepath.Join(t.TempDir(), "file.txt")
		step := &schema.WorkflowStep{Name: "test", WorkingDirectory: t.TempDir()}
		vars := NewVariables()

		result, err := handler.ResolveInWorkingDirectory(step, vars, absValue, "source")
		assert.NoError(t, err)
		assert.Equal(t, absValue, result)
	})

	t.Run("empty working directory falls back to process cwd", func(t *testing.T) {
		workDir := t.TempDir()
		t.Chdir(workDir)
		step := &schema.WorkflowStep{Name: "test", WorkingDirectory: ""}
		vars := NewVariables()

		result, err := handler.ResolveInWorkingDirectory(step, vars, "rel/file.txt", "source")
		assert.NoError(t, err)
		assert.Equal(t, filepath.Join(workDir, "rel", "file.txt"), result)
	})

	t.Run("resolves template in value before join", func(t *testing.T) {
		workDir := t.TempDir()
		step := &schema.WorkflowStep{Name: "test", WorkingDirectory: workDir}
		vars := NewVariables()
		vars.SetEnv("SUBDIR", "sub")

		result, err := handler.ResolveInWorkingDirectory(step, vars, "{{ .env.SUBDIR }}/file.txt", "source")
		assert.NoError(t, err)
		assert.Equal(t, filepath.Join(workDir, "sub", "file.txt"), result)
	})

	t.Run("resolves template in working directory before join", func(t *testing.T) {
		workDir := t.TempDir()
		step := &schema.WorkflowStep{Name: "test", WorkingDirectory: "{{ .env.WORKDIR }}"}
		vars := NewVariables()
		vars.SetEnv("WORKDIR", workDir)

		result, err := handler.ResolveInWorkingDirectory(step, vars, "file.txt", "source")
		assert.NoError(t, err)
		assert.Equal(t, filepath.Join(workDir, "file.txt"), result)
	})

	t.Run("returns error on invalid template in value", func(t *testing.T) {
		step := &schema.WorkflowStep{Name: "test", WorkingDirectory: t.TempDir()}
		vars := NewVariables()

		_, err := handler.ResolveInWorkingDirectory(step, vars, "{{ .invalid", "source")
		assert.Error(t, err)
		assert.ErrorIs(t, err, errUtils.ErrTemplateEvaluation)
	})

	t.Run("returns error on invalid template in working directory", func(t *testing.T) {
		step := &schema.WorkflowStep{Name: "test", WorkingDirectory: "{{ .invalid"}
		vars := NewVariables()

		_, err := handler.ResolveInWorkingDirectory(step, vars, "file.txt", "source")
		assert.Error(t, err)
		assert.ErrorIs(t, err, errUtils.ErrTemplateEvaluation)
	})

	t.Run("resolved-to-empty value is not replaced by working directory", func(t *testing.T) {
		step := &schema.WorkflowStep{Name: "test", WorkingDirectory: t.TempDir()}
		vars := NewVariables()

		result, err := handler.ResolveInWorkingDirectory(step, vars, "{{ if false }}x{{ end }}", "source")
		assert.NoError(t, err)
		assert.Empty(t, result)
	})

	t.Run("relative working directory (no template) is anchored to process cwd", func(t *testing.T) {
		// WorkingDirectory here is a literal relative string (no template
		// syntax), so vars.Resolve returns it unchanged and resolveWorkingDirectory
		// must fall through to the filepath.Abs(workDir) conversion — a distinct
		// branch from the "already absolute" and "empty" cases exercised above.
		cwd := t.TempDir()
		t.Chdir(cwd)
		step := &schema.WorkflowStep{Name: "test", WorkingDirectory: filepath.Join("relative", "workdir")}
		vars := NewVariables()

		result, err := handler.ResolveInWorkingDirectory(step, vars, "file.txt", "source")
		require.NoError(t, err)
		assert.Equal(t, filepath.Join(cwd, "relative", "workdir", "file.txt"), result)
	})

	t.Run("bare relative working directory anchors to component working directory when set", func(t *testing.T) {
		cwd := t.TempDir()
		t.Chdir(cwd)
		componentDir := t.TempDir()
		step := &schema.WorkflowStep{Name: "test", WorkingDirectory: filepath.Join("relative", "workdir")}
		vars := NewVariables()
		vars.SetComponentWorkingDirectory(componentDir)

		result, err := handler.ResolveInWorkingDirectory(step, vars, "file.txt", "source")
		require.NoError(t, err)
		assert.Equal(t, filepath.Join(componentDir, "relative", "workdir", "file.txt"), result)
	})

	t.Run("dot-prefixed working directory stays cwd-relative even when a component anchor is set", func(t *testing.T) {
		cwd := t.TempDir()
		t.Chdir(cwd)
		componentDir := t.TempDir()
		// Deliberately not filepath.Join'd: filepath.Join calls Clean and would
		// strip the "./" prefix, defeating the point of this case.
		step := &schema.WorkflowStep{Name: "test", WorkingDirectory: "./relative/workdir"}
		vars := NewVariables()
		vars.SetComponentWorkingDirectory(componentDir)

		result, err := handler.ResolveInWorkingDirectory(step, vars, "file.txt", "source")
		require.NoError(t, err)
		assert.Equal(t, filepath.Join(cwd, "relative", "workdir", "file.txt"), result)
	})

	t.Run("bare-dot-dot working directory stays cwd-relative even when a component anchor is set", func(t *testing.T) {
		// ".." (not "../x") is the case pkg/component.IsExplicitComponentPath
		// would miss; this proves the dedicated classifier handles it.
		cwd := filepath.Join(t.TempDir(), "nested")
		require.NoError(t, os.MkdirAll(cwd, 0o755))
		t.Chdir(cwd)
		componentDir := t.TempDir()
		step := &schema.WorkflowStep{Name: "test", WorkingDirectory: ".."}
		vars := NewVariables()
		vars.SetComponentWorkingDirectory(componentDir)

		result, err := handler.ResolveInWorkingDirectory(step, vars, "file.txt", "source")
		require.NoError(t, err)
		assert.Equal(t, filepath.Join(filepath.Dir(cwd), "file.txt"), result)
	})

	t.Run("bare relative working directory falls back to process cwd when no component anchor is set", func(t *testing.T) {
		cwd := t.TempDir()
		t.Chdir(cwd)
		step := &schema.WorkflowStep{Name: "test", WorkingDirectory: filepath.Join("relative", "workdir")}
		vars := NewVariables() // no SetComponentWorkingDirectory call — workflows/custom-commands never call it

		result, err := handler.ResolveInWorkingDirectory(step, vars, "file.txt", "source")
		require.NoError(t, err)
		assert.Equal(t, filepath.Join(cwd, "relative", "workdir", "file.txt"), result)
	})
}
