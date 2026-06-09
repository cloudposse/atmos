package step

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

func TestCommandHandlersRegistration(t *testing.T) {
	// Verify all command handlers are registered.
	tests := []struct {
		name        string
		category    StepCategory
		requiresTTY bool
	}{
		{"atmos", CategoryCommand, false},
		{"shell", CategoryCommand, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler, ok := Get(tt.name)
			require.True(t, ok, "handler %s should be registered", tt.name)
			assert.Equal(t, tt.name, handler.GetName())
			assert.Equal(t, tt.category, handler.GetCategory())
			assert.Equal(t, tt.requiresTTY, handler.RequiresTTY())
		})
	}
}

func TestAtmosHandlerValidation(t *testing.T) {
	handler, ok := Get("atmos")
	require.True(t, ok)

	tests := []struct {
		name      string
		step      *schema.WorkflowStep
		expectErr bool
	}{
		{
			name: "valid with command",
			step: &schema.WorkflowStep{
				Name:    "test",
				Type:    "atmos",
				Command: "terraform plan vpc",
			},
			expectErr: false,
		},
		{
			name: "missing command",
			step: &schema.WorkflowStep{
				Name: "test",
				Type: "atmos",
			},
			expectErr: true,
		},
		{
			name: "with stack",
			step: &schema.WorkflowStep{
				Name:    "test",
				Type:    "atmos",
				Command: "terraform plan vpc",
				Stack:   "ue2-dev",
			},
			expectErr: false,
		},
		{
			name: "with output mode",
			step: &schema.WorkflowStep{
				Name:    "test",
				Type:    "atmos",
				Command: "terraform plan vpc",
				Output:  "viewport",
			},
			expectErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := handler.Validate(tt.step)
			if tt.expectErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestShellHandlerValidation(t *testing.T) {
	handler, ok := Get("shell")
	require.True(t, ok)

	tests := []struct {
		name      string
		step      *schema.WorkflowStep
		expectErr bool
	}{
		{
			name: "valid with command",
			step: &schema.WorkflowStep{
				Name:    "test",
				Type:    "shell",
				Command: "echo hello",
			},
			expectErr: false,
		},
		{
			name: "missing command",
			step: &schema.WorkflowStep{
				Name: "test",
				Type: "shell",
			},
			expectErr: true,
		},
		{
			name: "with working directory",
			step: &schema.WorkflowStep{
				Name:             "test",
				Type:             "shell",
				Command:          "ls -la",
				WorkingDirectory: "/tmp",
			},
			expectErr: false,
		},
		{
			name: "with environment",
			step: &schema.WorkflowStep{
				Name:    "test",
				Type:    "shell",
				Command: "echo $MY_VAR",
				Env: map[string]string{
					"MY_VAR": "hello",
				},
			},
			expectErr: false,
		},
		{
			name: "with output mode",
			step: &schema.WorkflowStep{
				Name:    "test",
				Type:    "shell",
				Command: "echo hello",
				Output:  "none",
			},
			expectErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := handler.Validate(tt.step)
			if tt.expectErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestOutputModeTypes(t *testing.T) {
	tests := []struct {
		mode     OutputMode
		expected string
	}{
		{OutputModeViewport, "viewport"},
		{OutputModeRaw, "raw"},
		{OutputModeLog, "log"},
		{OutputModeNone, "none"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			assert.Equal(t, tt.expected, string(tt.mode))
		})
	}
}

func TestGetOutputMode(t *testing.T) {
	tests := []struct {
		name           string
		stepOutput     string
		workflowOutput string
		expected       OutputMode
	}{
		{
			name:           "step overrides workflow",
			stepOutput:     "viewport",
			workflowOutput: "log",
			expected:       OutputModeViewport,
		},
		{
			name:           "workflow default used when step empty",
			stepOutput:     "",
			workflowOutput: "raw",
			expected:       OutputModeRaw,
		},
		{
			name:           "default to log when both empty",
			stepOutput:     "",
			workflowOutput: "",
			expected:       OutputModeLog,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			step := &schema.WorkflowStep{Output: tt.stepOutput}
			workflow := &schema.WorkflowDefinition{Output: tt.workflowOutput}

			result := GetOutputMode(step, workflow)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGetViewportConfig(t *testing.T) {
	t.Run("step overrides workflow", func(t *testing.T) {
		stepViewport := &schema.ViewportConfig{Height: 30, Width: 100}
		workflowViewport := &schema.ViewportConfig{Height: 20, Width: 80}

		step := &schema.WorkflowStep{Viewport: stepViewport}
		workflow := &schema.WorkflowDefinition{Viewport: workflowViewport}

		result := GetViewportConfig(step, workflow)
		assert.Equal(t, stepViewport, result)
	})

	t.Run("workflow default when step nil", func(t *testing.T) {
		workflowViewport := &schema.ViewportConfig{Height: 20, Width: 80}

		step := &schema.WorkflowStep{}
		workflow := &schema.WorkflowDefinition{Viewport: workflowViewport}

		result := GetViewportConfig(step, workflow)
		assert.Equal(t, workflowViewport, result)
	})

	t.Run("nil when both empty", func(t *testing.T) {
		step := &schema.WorkflowStep{}
		workflow := &schema.WorkflowDefinition{}

		result := GetViewportConfig(step, workflow)
		assert.Nil(t, result)
	})
}

func TestContainsStackFlag(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		expected bool
	}{
		{
			name:     "short flag with separate value",
			args:     []string{"terraform", "plan", "-s", "ue2-dev"},
			expected: true,
		},
		{
			name:     "short flag with equals",
			args:     []string{"terraform", "plan", "-s=ue2-dev"},
			expected: true,
		},
		{
			name:     "long flag with separate value",
			args:     []string{"terraform", "plan", "--stack", "ue2-dev"},
			expected: true,
		},
		{
			name:     "long flag with equals",
			args:     []string{"terraform", "plan", "--stack=ue2-dev"},
			expected: true,
		},
		{
			name:     "no stack flag",
			args:     []string{"terraform", "plan", "vpc"},
			expected: false,
		},
		{
			name:     "empty args",
			args:     []string{},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := containsStackFlag(tt.args)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestCommandHandlersByCategory(t *testing.T) {
	byCategory := ListByCategory()
	commandHandlers := byCategory[CategoryCommand]

	// Should have at least 2 command handlers.
	assert.GreaterOrEqual(t, len(commandHandlers), 2,
		"should have at least 2 command handlers (atmos, shell)")

	// Verify specific handlers exist.
	handlerNames := make([]string, 0, len(commandHandlers))
	for _, h := range commandHandlers {
		handlerNames = append(handlerNames, h.GetName())
	}
	assert.Contains(t, handlerNames, "atmos")
	assert.Contains(t, handlerNames, "shell")
}
