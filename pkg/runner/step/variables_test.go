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
		WithOutput("alias", "declared").
		WithSkipped())
	vars.SetEnv("ENV_VAR", "env_value")
	vars.SetFlag("stack", "plat-ue2-dev")

	// Access templateData indirectly through Resolve.
	result, err := vars.Resolve("{{ .steps.step1.value }}-{{ .steps.step1.outputs.alias }}-{{ .steps.step1.key }}-{{ .steps.step1.skipped }}-{{ .env.ENV_VAR }}-{{ .Env.ENV_VAR }}-{{ .Flags.stack }}-{{ .flags.stack }}")
	require.NoError(t, err)
	assert.Equal(t, "value1-declared-meta_value-true-env_value-env_value-plat-ue2-dev-plat-ue2-dev", result)
}

func TestVariablesResolveCustomTemplateData(t *testing.T) {
	vars := NewVariables()
	vars.SetTemplatePasses(3)
	vars.ProtectTemplateRoots("Arguments", "Flags", "flags", "TrailingArgs")
	vars.Set("previous", NewStepResult("ready"))
	vars.SetTemplateData(map[string]any{
		"Arguments": map[string]string{
			"component": "api",
			"message":   "{{ .ComponentConfig.injected }}",
		},
		"Flags": map[string]any{
			"stack":   "plat-ue2-dev",
			"target":  "{{ .ComponentConfig.injected }}",
			"verbose": true,
		},
		"flags": map[string]any{
			"stack": "plat-ue2-dev",
		},
		"TrailingArgs": []string{"{{ .ComponentConfig.injected }}"},
		"ComponentConfig": map[string]any{
			"nested":   "{{ .Flags.stack }}",
			"injected": "expanded",
		},
	})

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "argument value with template markers stays literal",
			input:    "echo {{ .Arguments.message }}",
			expected: "echo {{ .ComponentConfig.injected }}",
		},
		{
			name:     "flag string with template markers stays literal",
			input:    "run --target {{ .Flags.target }}",
			expected: "run --target {{ .ComponentConfig.injected }}",
		},
		{
			name:     "trailing arg with template markers stays literal",
			input:    "cmd {{ index .TrailingArgs 0 }}",
			expected: "cmd {{ .ComponentConfig.injected }}",
		},
		{
			name:     "config owned nested template expands",
			input:    "stack {{ .ComponentConfig.nested }}",
			expected: "stack plat-ue2-dev",
		},
		{
			name:     "bool flag is preserved",
			input:    "run --verbose={{ .Flags.verbose }}",
			expected: "run --verbose=true",
		},
		{
			name:     "lowercase flags and step outputs coexist",
			input:    "table {{ .flags.stack }} {{ .steps.previous.value }}",
			expected: "table plat-ue2-dev ready",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := vars.Resolve(tt.input)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestVariablesResolveUsesTemplateRenderer(t *testing.T) {
	vars := NewVariables()
	vars.SetTemplateRenderer(func(name, input string, data any) (string, error) {
		assert.Equal(t, "step-pass-1", name)
		assert.Contains(t, data.(map[string]any), "Arguments")
		return "rendered by callback", nil
	})
	vars.SetTemplateData(map[string]any{
		"Arguments": map[string]string{"name": "api"},
	})

	result, err := vars.Resolve("ignored")
	require.NoError(t, err)
	assert.Equal(t, "rendered by callback", result)
}

func TestVariablesSetWithOutputs(t *testing.T) {
	vars := NewVariables()
	vars.Set("build", NewStepResult("app:local").WithMetadata("image", "app:local").WithOutput("image", "app:local"))

	result := NewStepResult("pushed").
		WithMetadata("stdout", "ok").
		WithMetadata("stderr", "").
		WithMetadata("exit_code", 0).
		WithMetadata("image", "registry.example.com/app:local").
		WithMetadata("digest", "sha256:abc")

	err := vars.SetWithOutputs("push", result, map[string]string{
		"image":        "{{ .metadata.image }}",
		"digest":       "{{ .digest }}",
		"build_image":  "{{ .steps.build.outputs.image }}",
		"command_code": "{{ .exit_code }}",
	})
	require.NoError(t, err)

	stored := vars.Steps["push"]
	require.NotNil(t, stored)
	assert.Equal(t, "registry.example.com/app:local", stored.Outputs["image"])
	assert.Equal(t, "sha256:abc", stored.Outputs["digest"])
	assert.Equal(t, "app:local", stored.Outputs["build_image"])
	assert.Equal(t, "0", stored.Outputs["command_code"])
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
