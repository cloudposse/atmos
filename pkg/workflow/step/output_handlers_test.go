package step

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

func TestOutputHandlersRegistration(t *testing.T) {
	// Verify all output handlers are registered.
	tests := []struct {
		name        string
		category    StepCategory
		requiresTTY bool
	}{
		{"spin", CategoryOutput, false},
		{"table", CategoryOutput, false},
		{"pager", CategoryOutput, false},
		{"format", CategoryOutput, false},
		{"join", CategoryOutput, false},
		{"style", CategoryOutput, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler, ok := Get(tt.name)
			require.True(t, ok, "handler %s should be registered", tt.name)
			assert.Equal(t, tt.name, handler.GetName())
			assert.Equal(t, tt.category, handler.GetCategory())
			assert.Equal(t, tt.requiresTTY, handler.RequiresTTY(), "output handlers should not require TTY")
		})
	}
}

func TestSpinHandlerValidation(t *testing.T) {
	handler, ok := Get("spin")
	require.True(t, ok)

	tests := []struct {
		name      string
		step      *schema.WorkflowStep
		expectErr bool
	}{
		{
			name: "valid with title and command",
			step: &schema.WorkflowStep{
				Name:    "test",
				Type:    "spin",
				Title:   "Building...",
				Command: "make build",
			},
			expectErr: false,
		},
		{
			name: "missing title",
			step: &schema.WorkflowStep{
				Name:    "test",
				Type:    "spin",
				Command: "make build",
			},
			expectErr: true,
		},
		{
			name: "missing command",
			step: &schema.WorkflowStep{
				Name:  "test",
				Type:  "spin",
				Title: "Building...",
			},
			expectErr: true,
		},
		{
			name: "with timeout",
			step: &schema.WorkflowStep{
				Name:    "test",
				Type:    "spin",
				Title:   "Building...",
				Command: "make build",
				Timeout: "5m",
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

func TestTableHandlerValidation(t *testing.T) {
	handler, ok := Get("table")
	require.True(t, ok)

	tests := []struct {
		name      string
		step      *schema.WorkflowStep
		expectErr bool
	}{
		{
			name: "valid with data",
			step: &schema.WorkflowStep{
				Name: "test",
				Type: "table",
				Data: []map[string]any{
					{"Name": "vpc", "Status": "deployed"},
				},
			},
			expectErr: false,
		},
		{
			name: "valid with content",
			step: &schema.WorkflowStep{
				Name:    "test",
				Type:    "table",
				Content: "pre-formatted table",
			},
			expectErr: false,
		},
		{
			name: "missing data and content",
			step: &schema.WorkflowStep{
				Name: "test",
				Type: "table",
			},
			expectErr: true,
		},
		{
			name: "with columns",
			step: &schema.WorkflowStep{
				Name:    "test",
				Type:    "table",
				Columns: []string{"Name", "Status"},
				Data: []map[string]any{
					{"Name": "vpc", "Status": "deployed"},
				},
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
func TestPagerHandlerValidation(t *testing.T) {
	handler, ok := Get("pager")
	require.True(t, ok)

	tests := []struct {
		name      string
		step      *schema.WorkflowStep
		expectErr bool
	}{
		{
			name: "valid with content",
			step: &schema.WorkflowStep{
				Name:    "test",
				Type:    "pager",
				Content: "Long content to display in pager",
			},
			expectErr: false,
		},
		{
			name: "missing content",
			step: &schema.WorkflowStep{
				Name: "test",
				Type: "pager",
			},
			expectErr: true,
		},
		{
			name: "with title",
			step: &schema.WorkflowStep{
				Name:    "test",
				Type:    "pager",
				Title:   "View Output",
				Content: "Content here",
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

func TestFormatHandlerValidation(t *testing.T) {
	handler, ok := Get("format")
	require.True(t, ok)

	tests := []struct {
		name      string
		step      *schema.WorkflowStep
		expectErr bool
	}{
		{
			name: "valid with content",
			step: &schema.WorkflowStep{
				Name:    "test",
				Type:    "format",
				Content: "Environment: {{ .steps.env.value }}",
			},
			expectErr: false,
		},
		{
			name: "missing content",
			step: &schema.WorkflowStep{
				Name: "test",
				Type: "format",
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

func TestJoinHandlerValidation(t *testing.T) {
	handler, ok := Get("join")
	require.True(t, ok)

	tests := []struct {
		name      string
		step      *schema.WorkflowStep
		expectErr bool
	}{
		{
			name: "valid with options",
			step: &schema.WorkflowStep{
				Name:    "test",
				Type:    "join",
				Options: []string{"line1", "line2", "line3"},
			},
			expectErr: false,
		},
		{
			name: "valid with content",
			step: &schema.WorkflowStep{
				Name:    "test",
				Type:    "join",
				Content: "{{ .steps.header.value }}\n{{ .steps.body.value }}",
			},
			expectErr: false,
		},
		{
			name: "missing options and content",
			step: &schema.WorkflowStep{
				Name: "test",
				Type: "join",
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

func TestStyleHandlerValidation(t *testing.T) {
	handler, ok := Get("style")
	require.True(t, ok)

	tests := []struct {
		name      string
		step      *schema.WorkflowStep
		expectErr bool
	}{
		{
			name: "valid with content",
			step: &schema.WorkflowStep{
				Name:    "test",
				Type:    "style",
				Content: "Styled Header",
			},
			expectErr: false,
		},
		{
			name: "missing content",
			step: &schema.WorkflowStep{
				Name: "test",
				Type: "style",
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

func TestOutputHandlersByCategory(t *testing.T) {
	byCategory := ListByCategory()
	outputHandlers := byCategory[CategoryOutput]

	// Should have at least 6 output handlers.
	assert.GreaterOrEqual(t, len(outputHandlers), 6,
		"should have at least 6 output handlers (spin, table, pager, format, join, style)")

	// None should require TTY.
	for _, handler := range outputHandlers {
		assert.False(t, handler.RequiresTTY(),
			"output handler %s should not require TTY", handler.GetName())
	}
}
