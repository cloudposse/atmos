package cmd

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestParseConfigPaths tests the pure parseConfigPaths function.
func TestParseConfigPaths(t *testing.T) {
	tests := []struct {
		name     string
		flagSet  bool
		flagVal  string
		expected []string
	}{
		{
			name:     "no flag set - returns defaults",
			flagSet:  false,
			expected: []string{".editorconfig", ".editorconfig-checker.json", ".ecrc"},
		},
		{
			name:     "flag set with single path",
			flagSet:  true,
			flagVal:  "custom.ecrc",
			expected: []string{"custom.ecrc"},
		},
		{
			name:     "flag set with multiple paths",
			flagSet:  true,
			flagVal:  ".ecrc,custom.json,.editorconfig",
			expected: []string{".ecrc", "custom.json", ".editorconfig"},
		},
		{
			name:     "flag set with empty string",
			flagSet:  true,
			flagVal:  "",
			expected: []string{""},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := &cobra.Command{}
			cmd.Flags().String("config", "", "config paths")

			if tt.flagSet {
				err := cmd.Flags().Set("config", tt.flagVal)
				require.NoError(t, err)
			}

			result := parseConfigPaths(cmd)

			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestInitConfig is an integration test that exercises initializeConfig with side effects.
// This test verifies the function doesn't panic but doesn't validate all behavior
// since initializeConfig modifies module-level variables.
//
// Note: More comprehensive testing would require further refactoring per
// https://linear.app/cloudposse/issue/DEV-3094
//
// Integration test coverage exists in validate-editorconfig.yaml.
func TestInitConfig(t *testing.T) {
	// Call function with no assertions - test passes if no panic occurs.
	initializeConfig(editorConfigCmd)
}
