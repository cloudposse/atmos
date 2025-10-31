package exec

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetShellArgs(t *testing.T) {
	tests := []struct {
		name         string
		userEnvProbe string
		expected     []string
	}{
		{
			name:         "loginShell returns -l flag",
			userEnvProbe: "loginShell",
			expected:     []string{"-l"},
		},
		{
			name:         "loginInteractiveShell returns -l flag",
			userEnvProbe: "loginInteractiveShell",
			expected:     []string{"-l"},
		},
		{
			name:         "empty string returns nil",
			userEnvProbe: "",
			expected:     nil,
		},
		{
			name:         "interactiveShell returns nil",
			userEnvProbe: "interactiveShell",
			expected:     nil,
		},
		{
			name:         "none returns nil",
			userEnvProbe: "none",
			expected:     nil,
		},
		{
			name:         "unknown value returns nil",
			userEnvProbe: "unknownValue",
			expected:     nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getShellArgs(tt.userEnvProbe)
			assert.Equal(t, tt.expected, result)
		})
	}
}
