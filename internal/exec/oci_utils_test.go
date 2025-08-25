package exec

import (
	"os"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
)

// TestBindEnv tests the Viper environment binding function
func TestBindEnv(t *testing.T) {
	tests := []struct {
		name     string
		key      string
		envVars  []string
		setEnv   map[string]string
		expected string
	}{
		{
			name:     "Single environment variable",
			key:      "test_key",
			envVars:  []string{"TEST_VAR"},
			setEnv:   map[string]string{"TEST_VAR": "test_value"},
			expected: "test_value",
		},
		{
			name:     "Multiple environment variables with fallback",
			key:      "test_key",
			envVars:  []string{"PRIMARY_VAR", "FALLBACK_VAR"},
			setEnv:   map[string]string{"FALLBACK_VAR": "fallback_value"},
			expected: "fallback_value",
		},
		{
			name:     "No environment variables set",
			key:      "test_key",
			envVars:  []string{"MISSING_VAR"},
			setEnv:   map[string]string{},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set up environment variables
			for key, value := range tt.setEnv {
				os.Setenv(key, value)
				defer os.Unsetenv(key)
			}

			// Create Viper instance
			v := viper.New()
			bindEnv(v, tt.key, tt.envVars...)

			// Test that the value is accessible
			result := v.GetString(tt.key)
			assert.Equal(t, tt.expected, result)
		})
	}
}
