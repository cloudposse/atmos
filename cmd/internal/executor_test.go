package internal

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"

	errUtils "github.com/cloudposse/atmos/errors"
)

// TestExtractCommandName tests extraction of command names from Cobra error messages.
func TestExtractCommandName(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "standard error format",
			input:    `unknown command "foobar" for "atmos"`,
			expected: "foobar",
		},
		{
			name:     "command with hyphens",
			input:    `unknown command "my-custom-cmd" for "atmos"`,
			expected: "my-custom-cmd",
		},
		{
			name:     "empty quotes",
			input:    `unknown command "" for "atmos"`,
			expected: "",
		},
		{
			name:     "no match",
			input:    "some other error message",
			expected: "",
		},
		{
			name:     "multiple quoted strings (should extract first)",
			input:    `unknown command "first" and "second"`,
			expected: "first",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractCommandName(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestConvertCobraError tests conversion of Cobra errors to sentinel errors.
func TestConvertCobraError(t *testing.T) {
	tests := []struct {
		name           string
		inputErr       error
		expectSentinel error
		expectCommand  string
	}{
		{
			name:           "nil error returns nil",
			inputErr:       nil,
			expectSentinel: nil,
			expectCommand:  "",
		},
		{
			name:           "unknown command error converts to ErrCommandNotFound",
			inputErr:       fmt.Errorf(`unknown command "foobar" for "atmos"`),
			expectSentinel: errUtils.ErrCommandNotFound,
			expectCommand:  "foobar",
		},
		{
			name:           "unknown command with suggestions",
			inputErr:       fmt.Errorf(`unknown command "terrafrom" for "atmos"\n\nDid you mean this?\n\tterraform`),
			expectSentinel: errUtils.ErrCommandNotFound,
			expectCommand:  "terrafrom",
		},
		{
			name:           "other error passes through unchanged",
			inputErr:       fmt.Errorf("some other error"),
			expectSentinel: nil,
			expectCommand:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := convertCobraError(tt.inputErr)

			if tt.expectSentinel == nil {
				// For nil or passthrough cases.
				if tt.inputErr == nil {
					assert.Nil(t, result)
				} else {
					assert.Equal(t, tt.inputErr, result)
				}
			} else {
				// For converted cases, use errors.Is().
				assert.True(t, errors.Is(result, tt.expectSentinel),
					"expected error to match sentinel %v, got %v", tt.expectSentinel, result)

				// Verify context is preserved.
				command, ok := errUtils.GetContext(result, "command")
				assert.True(t, ok, "expected context key 'command' to exist")
				assert.Equal(t, tt.expectCommand, command)
			}
		})
	}
}
