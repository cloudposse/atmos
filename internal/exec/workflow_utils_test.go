package exec

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestIsKnownWorkflowError tests the IsKnownWorkflowError function.
func TestIsKnownWorkflowError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "nil error",
			err:      nil,
			expected: false,
		},
		{
			name:     "ErrWorkflowNoSteps",
			err:      ErrWorkflowNoSteps,
			expected: true,
		},
		{
			name:     "ErrInvalidWorkflowStepType",
			err:      ErrInvalidWorkflowStepType,
			expected: true,
		},
		{
			name:     "ErrInvalidFromStep",
			err:      ErrInvalidFromStep,
			expected: true,
		},
		{
			name:     "ErrWorkflowStepFailed",
			err:      ErrWorkflowStepFailed,
			expected: true,
		},
		{
			name:     "ErrWorkflowNoWorkflow",
			err:      ErrWorkflowNoWorkflow,
			expected: true,
		},
		{
			name:     "ErrWorkflowFileNotFound",
			err:      ErrWorkflowFileNotFound,
			expected: true,
		},
		{
			name:     "ErrInvalidWorkflowManifest",
			err:      ErrInvalidWorkflowManifest,
			expected: true,
		},
		{
			name:     "wrapped known error",
			err:      errors.Join(ErrWorkflowNoSteps, errors.New("additional context")),
			expected: true,
		},
		{
			name:     "unknown error",
			err:      errors.New("some random error"),
			expected: false,
		},
		{
			name:     "wrapped unknown error",
			err:      errors.Join(errors.New("unknown"), errors.New("more context")),
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsKnownWorkflowError(tt.err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestFormatList tests the FormatList function.
func TestFormatList(t *testing.T) {
	tests := []struct {
		name     string
		input    []string
		expected string
	}{
		{
			name:     "empty list",
			input:    []string{},
			expected: "",
		},
		{
			name:     "single item",
			input:    []string{"item1"},
			expected: "- `item1`\n",
		},
		{
			name:     "multiple items",
			input:    []string{"item1", "item2", "item3"},
			expected: "- `item1`\n- `item2`\n- `item3`\n",
		},
		{
			name:     "items with spaces",
			input:    []string{"item with spaces", "another item"},
			expected: "- `item with spaces`\n- `another item`\n",
		},
		{
			name:     "items with special characters",
			input:    []string{"item-1", "item_2", "item.3"},
			expected: "- `item-1`\n- `item_2`\n- `item.3`\n",
		},
		{
			name:     "items with backticks in name",
			input:    []string{"item`with`backticks"},
			expected: "- `item`with`backticks`\n",
		},
		{
			name:     "empty string item",
			input:    []string{""},
			expected: "- ``\n",
		},
		{
			name:     "mixed empty and non-empty items",
			input:    []string{"item1", "", "item3"},
			expected: "- `item1`\n- ``\n- `item3`\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := FormatList(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}
