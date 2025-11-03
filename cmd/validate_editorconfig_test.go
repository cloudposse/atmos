package cmd

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/cloudposse/atmos/pkg/flags"
)

// TestInitConfig is an integration test that exercises initializeConfig with side effects.
// This test verifies the function doesn't panic but doesn't validate all behavior
// since initializeConfig modifies module-level variables.
//
// Note: More comprehensive testing would require further refactoring per
// https://linear.app/cloudposse/issue/DEV-3094
//
// Integration test coverage exists in validate-editorconfig.yaml.
func TestInitConfig(t *testing.T) {
	tests := []struct {
		name    string
		opts    *flags.EditorConfigOptions
		wantErr bool
	}{
		{
			name: "basic initialization with defaults",
			opts: &flags.EditorConfigOptions{
				Format: "default",
			},
			wantErr: false,
		},
		{
			name: "initialization with exclude pattern",
			opts: &flags.EditorConfigOptions{
				Exclude: "*.test",
				Format:  "default",
			},
			wantErr: false,
		},
		{
			name: "initialization with multiple disable flags",
			opts: &flags.EditorConfigOptions{
				DisableTrimTrailingWhitespace: true,
				DisableEndOfLine:              true,
				Format:                        "gcc",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Call function with no assertions - test passes if no panic occurs.
			assert.NotPanics(t, func() {
				initializeConfig(tt.opts)
			})
		})
	}
}

// TestEditorConfigParser verifies the parser is properly initialized.
func TestEditorConfigParser(t *testing.T) {
	// Verify the parser exists and is not nil.
	assert.NotNil(t, editorConfigParser)

	// Verify the editorConfigCmd is properly initialized with the parser.
	assert.NotNil(t, editorConfigCmd)

	// Verify key flags are registered on the command.
	assert.NotNil(t, editorConfigCmd.Flags().Lookup("format"))
	assert.NotNil(t, editorConfigCmd.Flags().Lookup("exclude"))
	assert.NotNil(t, editorConfigCmd.Flags().Lookup("init"))
	assert.NotNil(t, editorConfigCmd.Flags().Lookup("dry-run"))
	assert.NotNil(t, editorConfigCmd.Flags().Lookup("disable-trim-trailing-whitespace"))
}
