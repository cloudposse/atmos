package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/editorconfig-checker/editorconfig-checker/v3/pkg/config"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/data"
	iolib "github.com/cloudposse/atmos/pkg/io"
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

// TestRunMainLogic_DryRun tests the dry-run mode of runMainLogic.
// This covers the data.Writeln(file) call at line 172.
func TestRunMainLogic_DryRun(t *testing.T) {
	// Initialize I/O context for data package.
	ioCtx, err := iolib.NewContext()
	require.NoError(t, err)
	data.InitWriter(ioCtx)

	// Create a temp directory for test files.
	tmpDir := t.TempDir()

	// Create an .editorconfig file.
	editorConfigContent := `root = true

[*]
indent_style = space
indent_size = 2
`
	editorConfigPath := filepath.Join(tmpDir, ".editorconfig")
	err = os.WriteFile(editorConfigPath, []byte(editorConfigContent), 0o644)
	require.NoError(t, err)

	// Create a test file to be discovered.
	testFilePath := filepath.Join(tmpDir, "test.txt")
	err = os.WriteFile(testFilePath, []byte("test content\n"), 0o644)
	require.NoError(t, err)

	// Change to the temp directory.
	t.Chdir(tmpDir)

	// Save and restore the original currentConfig.
	originalConfig := currentConfig
	defer func() { currentConfig = originalConfig }()

	// Create a new config with DryRun enabled.
	cfg := config.NewConfig([]string{".editorconfig"})
	cfg.DryRun = true
	currentConfig = cfg

	// Run the main logic - should not panic and should return early after dry-run output.
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("runMainLogic panicked: %v", r)
		}
	}()

	runMainLogic()
}
