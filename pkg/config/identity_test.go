package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNormalizeIdentityValue(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		// Empty value should stay empty.
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},

		// Boolean false representations should be normalized to __DISABLED__.
		{
			name:     "false lowercase",
			input:    "false",
			expected: IdentityFlagDisabledValue,
		},
		{
			name:     "false uppercase",
			input:    "FALSE",
			expected: IdentityFlagDisabledValue,
		},
		{
			name:     "false mixed case",
			input:    "False",
			expected: IdentityFlagDisabledValue,
		},
		{
			name:     "zero",
			input:    "0",
			expected: IdentityFlagDisabledValue,
		},
		{
			name:     "no lowercase",
			input:    "no",
			expected: IdentityFlagDisabledValue,
		},
		{
			name:     "no uppercase",
			input:    "NO",
			expected: IdentityFlagDisabledValue,
		},
		{
			name:     "no mixed case",
			input:    "No",
			expected: IdentityFlagDisabledValue,
		},
		{
			name:     "off lowercase",
			input:    "off",
			expected: IdentityFlagDisabledValue,
		},
		{
			name:     "off uppercase",
			input:    "OFF",
			expected: IdentityFlagDisabledValue,
		},
		{
			name:     "off mixed case",
			input:    "Off",
			expected: IdentityFlagDisabledValue,
		},

		// Real identity names should pass through unchanged.
		{
			name:     "real identity name",
			input:    "prod-admin",
			expected: "prod-admin",
		},
		{
			name:     "identity with aws prefix",
			input:    "aws-sso-admin",
			expected: "aws-sso-admin",
		},
		{
			name:     "select value passthrough",
			input:    IdentityFlagSelectValue,
			expected: IdentityFlagSelectValue,
		},
		{
			name:     "disabled value passthrough",
			input:    IdentityFlagDisabledValue,
			expected: IdentityFlagDisabledValue,
		},

		// Edge cases - these should NOT be treated as false.
		{
			name:     "falsey but not false",
			input:    "falsey",
			expected: "falsey",
		},
		{
			name:     "nope is not no",
			input:    "nope",
			expected: "nope",
		},
		{
			name:     "offline is not off",
			input:    "offline",
			expected: "offline",
		},
		{
			name:     "truthy value",
			input:    "true",
			expected: "true",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := NormalizeIdentityValue(tc.input)
			assert.Equal(t, tc.expected, result)
		})
	}
}
