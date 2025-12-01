package env

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsColorEnabled(t *testing.T) {
	tests := []struct {
		name          string
		noColor       string
		cliColor      string
		cliColorForce string
		forceColor    string
		expected      *bool
		description   string
	}{
		{
			name:        "NO_COLOR set disables color",
			noColor:     "1",
			expected:    boolPtr(false),
			description: "NO_COLOR always wins and disables color",
		},
		{
			name:        "NO_COLOR empty string treated as unset",
			noColor:     "",
			expected:    nil,
			description: "Empty NO_COLOR should not disable color",
		},
		{
			name:        "CLICOLOR=0 disables color",
			cliColor:    "0",
			expected:    boolPtr(false),
			description: "CLICOLOR=0 disables color output",
		},
		{
			name:          "CLICOLOR=0 with CLICOLOR_FORCE enables color",
			cliColor:      "0",
			cliColorForce: "1",
			expected:      boolPtr(true),
			description:   "CLICOLOR_FORCE overrides CLICOLOR=0",
		},
		{
			name:        "CLICOLOR=0 with FORCE_COLOR enables color",
			cliColor:    "0",
			forceColor:  "1",
			expected:    boolPtr(true),
			description: "FORCE_COLOR overrides CLICOLOR=0",
		},
		{
			name:          "NO_COLOR overrides CLICOLOR_FORCE",
			noColor:       "1",
			cliColorForce: "1",
			expected:      boolPtr(false),
			description:   "NO_COLOR has highest priority",
		},
		{
			name:        "NO_COLOR overrides FORCE_COLOR",
			noColor:     "1",
			forceColor:  "1",
			expected:    boolPtr(false),
			description: "NO_COLOR has highest priority",
		},
		{
			name:          "CLICOLOR_FORCE alone enables color",
			cliColorForce: "1",
			expected:      boolPtr(true),
			description:   "CLICOLOR_FORCE forces color output",
		},
		{
			name:        "FORCE_COLOR alone enables color",
			forceColor:  "1",
			expected:    boolPtr(true),
			description: "FORCE_COLOR forces color output",
		},
		{
			name:        "CLICOLOR=1 returns nil (use TTY detection)",
			cliColor:    "1",
			expected:    nil,
			description: "CLICOLOR=1 means use TTY detection",
		},
		{
			name:        "No env vars set returns nil",
			expected:    nil,
			description: "Nil return means caller should use TTY detection",
		},
		{
			name:        "CLICOLOR with non-zero value returns nil",
			cliColor:    "yes",
			expected:    nil,
			description: "Only CLICOLOR=0 has special meaning",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Clear all environment variables (t.Setenv handles cleanup automatically)
			t.Setenv("NO_COLOR", "")
			t.Setenv("CLICOLOR", "")
			t.Setenv("CLICOLOR_FORCE", "")
			t.Setenv("FORCE_COLOR", "")

			// Set test-specific environment variables
			if tt.noColor != "" {
				t.Setenv("NO_COLOR", tt.noColor)
			}
			if tt.cliColor != "" {
				t.Setenv("CLICOLOR", tt.cliColor)
			}
			if tt.cliColorForce != "" {
				t.Setenv("CLICOLOR_FORCE", tt.cliColorForce)
			}
			if tt.forceColor != "" {
				t.Setenv("FORCE_COLOR", tt.forceColor)
			}

			result := IsColorEnabled()

			if tt.expected == nil {
				assert.Nil(t, result, tt.description)
			} else {
				assert.NotNil(t, result, "Expected non-nil result: %s", tt.description)
				if result != nil {
					assert.Equal(t, *tt.expected, *result, tt.description)
				}
			}
		})
	}
}

// boolPtr returns a pointer to a bool value.
func boolPtr(b bool) *bool {
	return &b
}
