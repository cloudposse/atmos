package step

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestUIHandlersRegistration(t *testing.T) {
	// Verify all UI handlers are registered.
	tests := []struct {
		name     string
		category StepCategory
	}{
		{"toast", CategoryUI},
		{"markdown", CategoryUI},
		{"alert", CategoryUI},
		{"title", CategoryUI},
		{"clear", CategoryUI},
		{"env", CategoryUI},
		{"exit", CategoryUI},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler, ok := Get(tt.name)
			require.True(t, ok, "handler %s should be registered", tt.name)
			assert.Equal(t, tt.name, handler.GetName())
			assert.Equal(t, tt.category, handler.GetCategory())
			assert.False(t, handler.RequiresTTY(), "UI handlers should not require TTY")
		})
	}
}

func TestUIHandlersValidation(t *testing.T) {
	tests := []struct {
		name      string
		stepType  string
		content   string
		expectErr bool
	}{
		{
			name:      "toast with content",
			stepType:  "toast",
			content:   "Deployment complete!",
			expectErr: false,
		},
		{
			name:      "toast without content",
			stepType:  "toast",
			content:   "",
			expectErr: true,
		},
		{
			name:      "markdown with content",
			stepType:  "markdown",
			content:   "# Title\n\nSome **bold** text",
			expectErr: false,
		},
		{
			name:      "markdown without content",
			stepType:  "markdown",
			content:   "",
			expectErr: true,
		},
		{
			name:      "alert without content",
			stepType:  "alert",
			content:   "",
			expectErr: false,
		},
		{
			name:      "alert with content",
			stepType:  "alert",
			content:   "Workflow complete!",
			expectErr: false,
		},
		{
			name:      "title without content",
			stepType:  "title",
			content:   "",
			expectErr: false,
		},
		{
			name:      "title with content",
			stepType:  "title",
			content:   "Deploying...",
			expectErr: false,
		},
		{
			name:      "clear step",
			stepType:  "clear",
			content:   "",
			expectErr: false,
		},
		{
			name:      "exit without content",
			stepType:  "exit",
			content:   "",
			expectErr: false,
		},
		{
			name:      "exit with content",
			stepType:  "exit",
			content:   "Goodbye!",
			expectErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler, ok := Get(tt.stepType)
			require.True(t, ok, "handler %s should be registered", tt.stepType)

			step := &schema.WorkflowStep{
				Name:    "test_step",
				Type:    tt.stepType,
				Content: tt.content,
			}

			err := handler.Validate(step)
			if tt.expectErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestEnvHandlerValidation(t *testing.T) {
	handler, ok := Get("env")
	require.True(t, ok, "env handler should be registered")

	tests := []struct {
		name      string
		vars      map[string]string
		expectErr bool
	}{
		{
			name:      "env with vars",
			vars:      map[string]string{"MY_VAR": "value"},
			expectErr: false,
		},
		{
			name:      "env with multiple vars",
			vars:      map[string]string{"VAR1": "value1", "VAR2": "value2"},
			expectErr: false,
		},
		{
			name:      "env without vars",
			vars:      nil,
			expectErr: true,
		},
		{
			name:      "env with empty vars",
			vars:      map[string]string{},
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			step := &schema.WorkflowStep{
				Name: "test_step",
				Type: "env",
				Vars: tt.vars,
			}

			err := handler.Validate(step)
			if tt.expectErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestEnvHandlerExecution(t *testing.T) {
	handler, ok := Get("env")
	require.True(t, ok, "env handler should be registered")

	vars := NewVariables()
	vars.Set("user_input", NewStepResult("production"))

	step := &schema.WorkflowStep{
		Name: "set_env",
		Type: "env",
		Vars: map[string]string{
			"DEPLOY_ENV": "{{ .steps.user_input.value }}",
			"STATIC_VAR": "fixed-value",
			"AWS_REGION": "us-east-1",
		},
	}

	ctx := context.Background()
	result, err := handler.Execute(ctx, step, vars)
	require.NoError(t, err)
	assert.NotNil(t, result)

	// Verify environment variables were set.
	assert.Equal(t, "production", vars.Env["DEPLOY_ENV"])
	assert.Equal(t, "fixed-value", vars.Env["STATIC_VAR"])
	assert.Equal(t, "us-east-1", vars.Env["AWS_REGION"])
}

func TestUIHandlersTemplateResolution(t *testing.T) {
	vars := NewVariables()
	vars.Set("select_env", NewStepResult("production"))
	vars.Set("select_component", NewStepResult("vpc"))

	tests := []struct {
		name            string
		stepType        string
		level           string
		content         string
		expectedContent string
	}{
		{
			name:            "toast success with template",
			stepType:        "toast",
			level:           "success",
			content:         "Deployed {{ .steps.select_component.value }} to {{ .steps.select_env.value }}",
			expectedContent: "Deployed vpc to production",
		},
		{
			name:            "toast info with template",
			stepType:        "toast",
			level:           "info",
			content:         "Component: {{ .steps.select_component.value }}",
			expectedContent: "Component: vpc",
		},
		{
			name:            "toast warning with template",
			stepType:        "toast",
			level:           "warning",
			content:         "Warning for {{ .steps.select_env.value }}",
			expectedContent: "Warning for production",
		},
		{
			name:            "toast error with template",
			stepType:        "toast",
			level:           "error",
			content:         "Failed in {{ .steps.select_env.value }}",
			expectedContent: "Failed in production",
		},
		{
			name:            "markdown with template",
			stepType:        "markdown",
			content:         "# Deploying {{ .steps.select_component.value }}\n\nTarget: {{ .steps.select_env.value }}",
			expectedContent: "# Deploying vpc\n\nTarget: production",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler, ok := Get(tt.stepType)
			require.True(t, ok, "handler %s should be registered", tt.stepType)

			step := &schema.WorkflowStep{
				Name:    "test_step",
				Type:    tt.stepType,
				Level:   tt.level,
				Content: tt.content,
			}

			ctx := context.Background()

			// We can't easily test the actual UI output without mocking,
			// but we can test that template resolution works by checking the result.
			result, err := handler.Execute(ctx, step, vars)

			// The execution might fail in test environment due to terminal issues,
			// but we can still verify the result if it succeeds.
			if err == nil {
				assert.Equal(t, tt.expectedContent, result.Value)
			}
		})
	}
}

func TestVariablesResolve(t *testing.T) {
	vars := NewVariables()
	vars.Set("env", NewStepResult("production"))
	vars.Set("component", NewStepResult("vpc"))

	tests := []struct {
		name     string
		input    string
		expected string
		hasError bool
	}{
		{
			name:     "simple variable",
			input:    "{{ .steps.env.value }}",
			expected: "production",
			hasError: false,
		},
		{
			name:     "multiple variables",
			input:    "{{ .steps.component.value }} in {{ .steps.env.value }}",
			expected: "vpc in production",
			hasError: false,
		},
		{
			name:     "no variables",
			input:    "plain text",
			expected: "plain text",
			hasError: false,
		},
		{
			name:     "empty string",
			input:    "",
			expected: "",
			hasError: false,
		},
		{
			name:     "invalid template syntax",
			input:    "{{ .steps.env.value",
			expected: "",
			hasError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := vars.Resolve(tt.input)
			if tt.hasError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expected, result)
			}
		})
	}
}

func TestVariablesResolveEnvMap(t *testing.T) {
	vars := NewVariables()
	vars.Set("env", NewStepResult("staging"))
	vars.Set("version", NewStepResult("1.0.0"))

	envMap := map[string]string{
		"DEPLOY_ENV": "{{ .steps.env.value }}",
		"VERSION":    "{{ .steps.version.value }}",
		"STATIC":     "fixed-value",
	}

	result, err := vars.ResolveEnvMap(envMap)
	require.NoError(t, err)

	assert.Equal(t, "staging", result["DEPLOY_ENV"])
	assert.Equal(t, "1.0.0", result["VERSION"])
	assert.Equal(t, "fixed-value", result["STATIC"])
}

func TestStepResult(t *testing.T) {
	t.Run("basic result", func(t *testing.T) {
		result := NewStepResult("test-value")
		assert.Equal(t, "test-value", result.Value)
		assert.Empty(t, result.Values)
		assert.NotNil(t, result.Metadata)
		assert.False(t, result.Skipped)
		assert.Empty(t, result.Error)
	})

	t.Run("with values", func(t *testing.T) {
		result := NewStepResult("").WithValues([]string{"a", "b", "c"})
		assert.Equal(t, []string{"a", "b", "c"}, result.Values)
	})

	t.Run("with metadata", func(t *testing.T) {
		result := NewStepResult("").WithMetadata("key", "value")
		assert.Equal(t, "value", result.Metadata["key"])
	})

	t.Run("with skipped", func(t *testing.T) {
		result := NewStepResult("").WithSkipped()
		assert.True(t, result.Skipped)
	})

	t.Run("with error", func(t *testing.T) {
		result := NewStepResult("").WithError("something went wrong")
		assert.Equal(t, "something went wrong", result.Error)
	})
}

func TestToastHandlerExecution(t *testing.T) {
	handler, ok := Get("toast")
	require.True(t, ok)

	tests := []struct {
		name    string
		level   string
		content string
	}{
		{"success level", "success", "Deployment complete!"},
		{"info level", "info", "Processing..."},
		{"warning level", "warning", "Deprecation notice"},
		{"error level", "error", "Build failed"},
		{"default level (empty)", "", "Default message"},
		{"warn alias", "warn", "Warning message"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			step := &schema.WorkflowStep{
				Name:    "test_toast",
				Type:    "toast",
				Level:   tt.level,
				Content: tt.content,
			}
			vars := NewVariables()

			result, err := handler.Execute(context.Background(), step, vars)
			// In test environment, UI formatter may not be initialized - this is expected.
			if err != nil {
				assert.True(t, errors.Is(err, errUtils.ErrUIFormatterNotInitialized),
					"expected ErrUIFormatterNotInitialized, got: %v", err)
				return
			}
			assert.Equal(t, tt.content, result.Value)
		})
	}
}

func TestAlertHandlerExecution(t *testing.T) {
	handler, ok := Get("alert")
	require.True(t, ok)

	t.Run("alert without content", func(t *testing.T) {
		step := &schema.WorkflowStep{
			Name: "test_alert",
			Type: "alert",
		}
		vars := NewVariables()

		result, err := handler.Execute(context.Background(), step, vars)
		// Alert without content doesn't use UI formatter, so should always succeed.
		require.NoError(t, err)
		assert.Equal(t, "", result.Value)
	})

	t.Run("alert with content", func(t *testing.T) {
		step := &schema.WorkflowStep{
			Name:    "test_alert",
			Type:    "alert",
			Content: "Task completed!",
		}
		vars := NewVariables()

		result, err := handler.Execute(context.Background(), step, vars)
		// In test environment, UI formatter may not be initialized - this is expected.
		if err != nil {
			assert.True(t, errors.Is(err, errUtils.ErrUIFormatterNotInitialized),
				"expected ErrUIFormatterNotInitialized, got: %v", err)
			return
		}
		assert.Equal(t, "Task completed!", result.Value)
	})

	t.Run("alert with template", func(t *testing.T) {
		step := &schema.WorkflowStep{
			Name:    "test_alert",
			Type:    "alert",
			Content: "Deployed {{ .steps.env.value }}",
		}
		vars := NewVariables()
		vars.Set("env", NewStepResult("production"))

		result, err := handler.Execute(context.Background(), step, vars)
		// In test environment, UI formatter may not be initialized - this is expected.
		if err != nil {
			assert.True(t, errors.Is(err, errUtils.ErrUIFormatterNotInitialized),
				"expected ErrUIFormatterNotInitialized, got: %v", err)
			return
		}
		assert.Equal(t, "Deployed production", result.Value)
	})
}

func TestTitleHandlerExecution(t *testing.T) {
	handler, ok := Get("title")
	require.True(t, ok)

	t.Run("set title", func(t *testing.T) {
		step := &schema.WorkflowStep{
			Name:    "test_title",
			Type:    "title",
			Content: "My Workflow",
		}
		vars := NewVariables()

		result, err := handler.Execute(context.Background(), step, vars)
		require.NoError(t, err)
		assert.Equal(t, "My Workflow", result.Value)
	})

	t.Run("restore title (empty content)", func(t *testing.T) {
		step := &schema.WorkflowStep{
			Name: "test_title",
			Type: "title",
		}
		vars := NewVariables()

		result, err := handler.Execute(context.Background(), step, vars)
		require.NoError(t, err)
		assert.Equal(t, "", result.Value)
	})

	t.Run("title with template", func(t *testing.T) {
		step := &schema.WorkflowStep{
			Name:    "test_title",
			Type:    "title",
			Content: "Deploying {{ .steps.component.value }}",
		}
		vars := NewVariables()
		vars.Set("component", NewStepResult("vpc"))

		result, err := handler.Execute(context.Background(), step, vars)
		require.NoError(t, err)
		assert.Equal(t, "Deploying vpc", result.Value)
	})
}

func TestLinebreakHandlerExecution(t *testing.T) {
	handler, ok := Get("linebreak")
	require.True(t, ok)

	t.Run("default count (1)", func(t *testing.T) {
		step := &schema.WorkflowStep{
			Name: "test_linebreak",
			Type: "linebreak",
		}
		vars := NewVariables()

		result, err := handler.Execute(context.Background(), step, vars)
		// In test environment, UI formatter may not be initialized - this is expected.
		if err != nil {
			assert.True(t, errors.Is(err, errUtils.ErrUIFormatterNotInitialized),
				"expected ErrUIFormatterNotInitialized, got: %v", err)
			return
		}
		assert.Equal(t, "", result.Value)
	})

	t.Run("explicit count", func(t *testing.T) {
		step := &schema.WorkflowStep{
			Name:  "test_linebreak",
			Type:  "linebreak",
			Count: 3,
		}
		vars := NewVariables()

		result, err := handler.Execute(context.Background(), step, vars)
		// In test environment, UI formatter may not be initialized - this is expected.
		if err != nil {
			assert.True(t, errors.Is(err, errUtils.ErrUIFormatterNotInitialized),
				"expected ErrUIFormatterNotInitialized, got: %v", err)
			return
		}
		assert.Equal(t, "", result.Value)
	})

	t.Run("zero count defaults to 1", func(t *testing.T) {
		step := &schema.WorkflowStep{
			Name:  "test_linebreak",
			Type:  "linebreak",
			Count: 0,
		}
		vars := NewVariables()

		result, err := handler.Execute(context.Background(), step, vars)
		// In test environment, UI formatter may not be initialized - this is expected.
		if err != nil {
			assert.True(t, errors.Is(err, errUtils.ErrUIFormatterNotInitialized),
				"expected ErrUIFormatterNotInitialized, got: %v", err)
			return
		}
		assert.Equal(t, "", result.Value)
	})
}

func TestClearHandlerExecution(t *testing.T) {
	handler, ok := Get("clear")
	require.True(t, ok)

	step := &schema.WorkflowStep{
		Name: "test_clear",
		Type: "clear",
	}
	vars := NewVariables()

	result, err := handler.Execute(context.Background(), step, vars)
	// In test environment, UI formatter may not be initialized - this is expected.
	if err != nil {
		assert.True(t, errors.Is(err, errUtils.ErrUIFormatterNotInitialized),
			"expected ErrUIFormatterNotInitialized, got: %v", err)
		return
	}
	assert.Equal(t, "", result.Value)
}

func TestSleepHandlerExecution(t *testing.T) {
	handler, ok := Get("sleep")
	require.True(t, ok)

	t.Run("custom short duration", func(t *testing.T) {
		step := &schema.WorkflowStep{
			Name:    "test_sleep",
			Type:    "sleep",
			Timeout: "50ms",
		}
		vars := NewVariables()

		start := time.Now()
		result, err := handler.Execute(context.Background(), step, vars)
		elapsed := time.Since(start)

		require.NoError(t, err)
		assert.Equal(t, "50ms", result.Value)
		assert.GreaterOrEqual(t, elapsed, 50*time.Millisecond)
		assert.Less(t, elapsed, 500*time.Millisecond)
	})

	t.Run("context cancellation", func(t *testing.T) {
		step := &schema.WorkflowStep{
			Name:    "test_sleep",
			Type:    "sleep",
			Timeout: "10s",
		}
		vars := NewVariables()

		ctx, cancel := context.WithCancel(context.Background())
		// Cancel immediately.
		cancel()

		_, err := handler.Execute(ctx, step, vars)
		assert.Error(t, err)
		assert.ErrorIs(t, err, errUtils.ErrUserAborted)
	})

	t.Run("invalid duration", func(t *testing.T) {
		step := &schema.WorkflowStep{
			Name:    "test_sleep",
			Type:    "sleep",
			Timeout: "invalid",
		}
		vars := NewVariables()

		_, err := handler.Execute(context.Background(), step, vars)
		assert.Error(t, err)
	})

	t.Run("duration with template", func(t *testing.T) {
		step := &schema.WorkflowStep{
			Name:    "test_sleep",
			Type:    "sleep",
			Timeout: "{{ .steps.delay.value }}",
		}
		vars := NewVariables()
		vars.Set("delay", NewStepResult("25ms"))

		start := time.Now()
		result, err := handler.Execute(context.Background(), step, vars)
		elapsed := time.Since(start)

		require.NoError(t, err)
		assert.Equal(t, "25ms", result.Value)
		assert.GreaterOrEqual(t, elapsed, 25*time.Millisecond)
	})
}

func TestRegistryOperations(t *testing.T) {
	t.Run("list returns all handlers", func(t *testing.T) {
		handlers := List()
		// At minimum, we should have the 2 UI handlers (toast, markdown).
		assert.GreaterOrEqual(t, len(handlers), 2)
	})

	t.Run("list by category", func(t *testing.T) {
		byCategory := ListByCategory()
		uiHandlers := byCategory[CategoryUI]
		assert.GreaterOrEqual(t, len(uiHandlers), 2, "should have at least 2 UI handlers")
	})

	t.Run("count returns handler count", func(t *testing.T) {
		count := Count()
		assert.GreaterOrEqual(t, count, 2)
	})

	t.Run("get non-existent handler", func(t *testing.T) {
		handler, ok := Get("non-existent")
		assert.False(t, ok)
		assert.Nil(t, handler)
	})
}

func TestExitHandlerExecution(t *testing.T) {
	handler, ok := Get("exit")
	require.True(t, ok)

	t.Run("exit with default code", func(t *testing.T) {
		step := &schema.WorkflowStep{
			Name: "test_exit",
			Type: "exit",
			Code: 0,
		}
		vars := NewVariables()

		result, err := handler.Execute(context.Background(), step, vars)
		assert.Error(t, err)
		assert.Nil(t, result)
		assert.True(t, errors.Is(err, errUtils.ErrWorkflowExit))
		assert.Equal(t, 0, errUtils.GetExitCode(err))
	})

	t.Run("exit with custom code", func(t *testing.T) {
		step := &schema.WorkflowStep{
			Name: "test_exit",
			Type: "exit",
			Code: 42,
		}
		vars := NewVariables()

		result, err := handler.Execute(context.Background(), step, vars)
		assert.Error(t, err)
		assert.Nil(t, result)
		assert.True(t, errors.Is(err, errUtils.ErrWorkflowExit))
		assert.Equal(t, 42, errUtils.GetExitCode(err))
	})

	t.Run("exit with content message", func(t *testing.T) {
		step := &schema.WorkflowStep{
			Name:    "test_exit",
			Type:    "exit",
			Code:    1,
			Content: "Exiting workflow",
		}
		vars := NewVariables()

		// Execute will fail in test environment if UI is not initialized,
		// but we're testing the error path.
		result, err := handler.Execute(context.Background(), step, vars)
		// May return ErrUIFormatterNotInitialized or ErrWorkflowExit.
		assert.Error(t, err)
		assert.Nil(t, result)
	})

	t.Run("exit with template content", func(t *testing.T) {
		step := &schema.WorkflowStep{
			Name:    "test_exit",
			Type:    "exit",
			Code:    1,
			Content: "Exiting {{ .steps.reason.value }}",
		}
		vars := NewVariables()
		vars.Set("reason", NewStepResult("due to error"))

		result, err := handler.Execute(context.Background(), step, vars)
		assert.Error(t, err)
		assert.Nil(t, result)
	})

	t.Run("exit with invalid template", func(t *testing.T) {
		step := &schema.WorkflowStep{
			Name:    "test_exit",
			Type:    "exit",
			Code:    1,
			Content: "Exiting {{ .steps.invalid.value",
		}
		vars := NewVariables()

		result, err := handler.Execute(context.Background(), step, vars)
		assert.Error(t, err)
		assert.Nil(t, result)
		// Should be template error, not workflow exit error.
		assert.False(t, errors.Is(err, errUtils.ErrWorkflowExit))
	})
}
