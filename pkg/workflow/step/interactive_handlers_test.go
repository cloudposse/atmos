package step

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

func TestInteractiveHandlersRegistration(t *testing.T) {
	// Verify all interactive handlers are registered.
	tests := []struct {
		name        string
		category    StepCategory
		requiresTTY bool
	}{
		{"input", CategoryInteractive, true},
		{"confirm", CategoryInteractive, true},
		{"choose", CategoryInteractive, true},
		{"write", CategoryInteractive, true},
		{"filter", CategoryInteractive, true},
		{"file", CategoryInteractive, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler, ok := Get(tt.name)
			require.True(t, ok, "handler %s should be registered", tt.name)
			assert.Equal(t, tt.name, handler.GetName())
			assert.Equal(t, tt.category, handler.GetCategory())
			assert.Equal(t, tt.requiresTTY, handler.RequiresTTY(), "interactive handlers should require TTY")
		})
	}
}

func TestInputHandlerValidation(t *testing.T) {
	handler, ok := Get("input")
	require.True(t, ok)

	tests := []struct {
		name      string
		step      *schema.WorkflowStep
		expectErr bool
	}{
		{
			name: "valid with prompt",
			step: &schema.WorkflowStep{
				Name:   "test",
				Type:   "input",
				Prompt: "Enter username",
			},
			expectErr: false,
		},
		{
			name: "missing prompt",
			step: &schema.WorkflowStep{
				Name: "test",
				Type: "input",
			},
			expectErr: true,
		},
		{
			name: "with all optional fields",
			step: &schema.WorkflowStep{
				Name:        "test",
				Type:        "input",
				Prompt:      "Enter password",
				Placeholder: "secret",
				Default:     "default",
				Password:    true,
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

// nolintlint: dupl - Similar test patterns for different handlers.
// nolint: dupl
func TestConfirmHandlerValidation(t *testing.T) {
	handler, ok := Get("confirm")
	require.True(t, ok)

	tests := []struct {
		name      string
		step      *schema.WorkflowStep
		expectErr bool
	}{
		{
			name: "valid with prompt",
			step: &schema.WorkflowStep{
				Name:   "test",
				Type:   "confirm",
				Prompt: "Continue?",
			},
			expectErr: false,
		},
		{
			name: "missing prompt",
			step: &schema.WorkflowStep{
				Name: "test",
				Type: "confirm",
			},
			expectErr: true,
		},
		{
			name: "with default yes",
			step: &schema.WorkflowStep{
				Name:    "test",
				Type:    "confirm",
				Prompt:  "Continue?",
				Default: "yes",
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

func TestChooseHandlerValidation(t *testing.T) {
	handler, ok := Get("choose")
	require.True(t, ok)

	tests := []struct {
		name      string
		step      *schema.WorkflowStep
		expectErr bool
	}{
		{
			name: "valid with prompt and options",
			step: &schema.WorkflowStep{
				Name:    "test",
				Type:    "choose",
				Prompt:  "Select environment",
				Options: []string{"dev", "staging", "prod"},
			},
			expectErr: false,
		},
		{
			name: "missing prompt",
			step: &schema.WorkflowStep{
				Name:    "test",
				Type:    "choose",
				Options: []string{"a", "b"},
			},
			expectErr: true,
		},
		{
			name: "missing options",
			step: &schema.WorkflowStep{
				Name:   "test",
				Type:   "choose",
				Prompt: "Select",
			},
			expectErr: true,
		},
		{
			name: "empty options",
			step: &schema.WorkflowStep{
				Name:    "test",
				Type:    "choose",
				Prompt:  "Select",
				Options: []string{},
			},
			expectErr: true,
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

// nolintlint: dupl - Similar test patterns for different handlers.
// nolint: dupl
func TestWriteHandlerValidation(t *testing.T) {
	handler, ok := Get("write")
	require.True(t, ok)

	tests := []struct {
		name      string
		step      *schema.WorkflowStep
		expectErr bool
	}{
		{
			name: "valid with prompt",
			step: &schema.WorkflowStep{
				Name:   "test",
				Type:   "write",
				Prompt: "Enter description",
			},
			expectErr: false,
		},
		{
			name: "missing prompt",
			step: &schema.WorkflowStep{
				Name: "test",
				Type: "write",
			},
			expectErr: true,
		},
		{
			name: "with height",
			step: &schema.WorkflowStep{
				Name:   "test",
				Type:   "write",
				Prompt: "Enter notes",
				Height: 10,
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

func TestFilterHandlerValidation(t *testing.T) {
	handler, ok := Get("filter")
	require.True(t, ok)

	tests := []struct {
		name      string
		step      *schema.WorkflowStep
		expectErr bool
	}{
		{
			name: "valid with prompt and options",
			step: &schema.WorkflowStep{
				Name:    "test",
				Type:    "filter",
				Prompt:  "Select component",
				Options: []string{"vpc", "eks", "rds"},
			},
			expectErr: false,
		},
		{
			name: "missing options",
			step: &schema.WorkflowStep{
				Name:   "test",
				Type:   "filter",
				Prompt: "Select",
			},
			expectErr: true,
		},
		{
			name: "with limit",
			step: &schema.WorkflowStep{
				Name:    "test",
				Type:    "filter",
				Prompt:  "Select components",
				Options: []string{"vpc", "eks", "rds"},
				Limit:   3,
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

func TestFileHandlerValidation(t *testing.T) {
	handler, ok := Get("file")
	require.True(t, ok)

	tests := []struct {
		name      string
		step      *schema.WorkflowStep
		expectErr bool
	}{
		{
			name: "valid with prompt",
			step: &schema.WorkflowStep{
				Name:   "test",
				Type:   "file",
				Prompt: "Select config file",
			},
			expectErr: false,
		},
		{
			name: "missing prompt",
			step: &schema.WorkflowStep{
				Name: "test",
				Type: "file",
			},
			expectErr: true,
		},
		{
			name: "with path and extensions",
			step: &schema.WorkflowStep{
				Name:       "test",
				Type:       "file",
				Prompt:     "Select YAML file",
				Path:       "./configs",
				Extensions: []string{".yaml", ".yml"},
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

func TestInteractiveHandlersByCategory(t *testing.T) {
	byCategory := ListByCategory()
	interactiveHandlers := byCategory[CategoryInteractive]

	// Should have at least 6 interactive handlers.
	assert.GreaterOrEqual(t, len(interactiveHandlers), 6,
		"should have at least 6 interactive handlers (input, confirm, choose, write, filter, file)")

	// All should require TTY.
	for _, handler := range interactiveHandlers {
		assert.True(t, handler.RequiresTTY(),
			"interactive handler %s should require TTY", handler.GetName())
	}
}
