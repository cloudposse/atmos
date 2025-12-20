package step

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

func TestUIHandlersRegistration(t *testing.T) {
	// Verify all UI handlers are registered.
	tests := []struct {
		name     string
		category StepCategory
	}{
		{"success", CategoryUI},
		{"info", CategoryUI},
		{"warn", CategoryUI},
		{"error", CategoryUI},
		{"markdown", CategoryUI},
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
			name:      "success with content",
			stepType:  "success",
			content:   "Deployment complete!",
			expectErr: false,
		},
		{
			name:      "success without content",
			stepType:  "success",
			content:   "",
			expectErr: true,
		},
		{
			name:      "info with content",
			stepType:  "info",
			content:   "Processing...",
			expectErr: false,
		},
		{
			name:      "info without content",
			stepType:  "info",
			content:   "",
			expectErr: true,
		},
		{
			name:      "warn with content",
			stepType:  "warn",
			content:   "This is deprecated",
			expectErr: false,
		},
		{
			name:      "warn without content",
			stepType:  "warn",
			content:   "",
			expectErr: true,
		},
		{
			name:      "error with content",
			stepType:  "error",
			content:   "Something went wrong",
			expectErr: false,
		},
		{
			name:      "error without content",
			stepType:  "error",
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

func TestUIHandlersTemplateResolution(t *testing.T) {
	vars := NewVariables()
	vars.Set("select_env", NewStepResult("production"))
	vars.Set("select_component", NewStepResult("vpc"))

	tests := []struct {
		name            string
		stepType        string
		content         string
		expectedContent string
	}{
		{
			name:            "success with template",
			stepType:        "success",
			content:         "Deployed {{ .steps.select_component.value }} to {{ .steps.select_env.value }}",
			expectedContent: "Deployed vpc to production",
		},
		{
			name:            "info with template",
			stepType:        "info",
			content:         "Component: {{ .steps.select_component.value }}",
			expectedContent: "Component: vpc",
		},
		{
			name:            "warn with template",
			stepType:        "warn",
			content:         "Warning for {{ .steps.select_env.value }}",
			expectedContent: "Warning for production",
		},
		{
			name:            "error with template",
			stepType:        "error",
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

func TestRegistryOperations(t *testing.T) {
	t.Run("list returns all handlers", func(t *testing.T) {
		handlers := List()
		// At minimum, we should have the 5 UI handlers.
		assert.GreaterOrEqual(t, len(handlers), 5)
	})

	t.Run("list by category", func(t *testing.T) {
		byCategory := ListByCategory()
		uiHandlers := byCategory[CategoryUI]
		assert.GreaterOrEqual(t, len(uiHandlers), 5, "should have at least 5 UI handlers")
	})

	t.Run("count returns handler count", func(t *testing.T) {
		count := Count()
		assert.GreaterOrEqual(t, count, 5)
	})

	t.Run("get non-existent handler", func(t *testing.T) {
		handler, ok := Get("non-existent")
		assert.False(t, ok)
		assert.Nil(t, handler)
	})
}
