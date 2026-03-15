package cmd

import (
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/editorconfig-checker/editorconfig-checker/v3/pkg/config"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/data"
	iolib "github.com/cloudposse/atmos/pkg/io"
	"github.com/cloudposse/atmos/pkg/schema"
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
	ioCtx, err := iolib.NewContext()
	require.NoError(t, err)
	data.InitWriter(ioCtx)
	t.Cleanup(data.Reset)

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

	// Capture stdout to assert dry-run output contains discovered files.
	r, w, err := os.Pipe()
	require.NoError(t, err)
	defer func() { _ = r.Close() }()

	originalStdout := os.Stdout
	os.Stdout = w
	t.Cleanup(func() { os.Stdout = originalStdout })

	require.NotPanics(t, runMainLogic)

	require.NoError(t, w.Close())
	output, err := io.ReadAll(r)
	require.NoError(t, err)
	assert.Contains(t, string(output), filepath.Base(testFilePath))
}

// TestCheckVersion tests the version checking logic.
func TestCheckVersion(t *testing.T) {
	tests := []struct {
		name        string
		config      config.Config
		expectError bool
	}{
		{
			name: "no config file exists",
			config: config.Config{
				Path:    "/nonexistent/path",
				Version: "",
			},
			expectError: false,
		},
		{
			name: "empty version in config",
			config: config.Config{
				Path:    ".",
				Version: "",
			},
			expectError: false,
		},
		{
			name: "version mismatch",
			config: config.Config{
				Path:    ".", // Current directory exists
				Version: "999.999.999",
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := checkVersion(tt.config)
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// TestReplaceAtmosConfigInConfig tests that atmos config values are properly applied.
func TestReplaceAtmosConfigInConfig(t *testing.T) {
	// Reset module-level variables before test.
	originalConfigFilePaths := configFilePaths
	originalTmpExclude := tmpExclude
	originalInitEditorConfig := initEditorConfig
	originalCliConfig := cliConfig
	defer func() {
		configFilePaths = originalConfigFilePaths
		tmpExclude = originalTmpExclude
		initEditorConfig = originalInitEditorConfig
		cliConfig = originalCliConfig
	}()

	tests := []struct {
		name        string
		atmosConfig schema.AtmosConfiguration
		flagChanged map[string]bool
		setup       func(*cobra.Command)
		validate    func(t *testing.T)
	}{
		{
			name: "applies config file paths from atmos config",
			atmosConfig: schema.AtmosConfiguration{
				Validate: schema.Validate{
					EditorConfig: schema.EditorConfig{
						ConfigFilePaths: []string{".custom-editorconfig"},
					},
				},
			},
			validate: func(t *testing.T) {
				assert.Equal(t, []string{".custom-editorconfig"}, configFilePaths)
			},
		},
		{
			name: "applies exclude patterns from atmos config",
			atmosConfig: schema.AtmosConfiguration{
				Validate: schema.Validate{
					EditorConfig: schema.EditorConfig{
						Exclude: []string{"vendor/**", "node_modules/**"},
					},
				},
			},
			validate: func(t *testing.T) {
				assert.Equal(t, "vendor/**,node_modules/**", tmpExclude)
			},
		},
		{
			name: "applies init flag from atmos config",
			atmosConfig: schema.AtmosConfiguration{
				Validate: schema.Validate{
					EditorConfig: schema.EditorConfig{
						Init: true,
					},
				},
			},
			validate: func(t *testing.T) {
				assert.True(t, initEditorConfig)
			},
		},
		{
			name: "applies ignore defaults from atmos config",
			atmosConfig: schema.AtmosConfiguration{
				Validate: schema.Validate{
					EditorConfig: schema.EditorConfig{
						IgnoreDefaults: true,
					},
				},
			},
			validate: func(t *testing.T) {
				assert.True(t, cliConfig.IgnoreDefaults)
			},
		},
		{
			name: "applies dry run from atmos config",
			atmosConfig: schema.AtmosConfiguration{
				Validate: schema.Validate{
					EditorConfig: schema.EditorConfig{
						DryRun: true,
					},
				},
			},
			validate: func(t *testing.T) {
				assert.True(t, cliConfig.DryRun)
			},
		},
		{
			name: "applies disable flags from atmos config",
			atmosConfig: schema.AtmosConfiguration{
				Validate: schema.Validate{
					EditorConfig: schema.EditorConfig{
						DisableTrimTrailingWhitespace: true,
						DisableEndOfLine:              true,
						DisableInsertFinalNewline:     true,
						DisableIndentation:            true,
						DisableIndentSize:             true,
						DisableMaxLineLength:          true,
					},
				},
			},
			validate: func(t *testing.T) {
				assert.True(t, cliConfig.Disable.TrimTrailingWhitespace)
				assert.True(t, cliConfig.Disable.EndOfLine)
				assert.True(t, cliConfig.Disable.InsertFinalNewline)
				assert.True(t, cliConfig.Disable.Indentation)
				assert.True(t, cliConfig.Disable.IndentSize)
				assert.True(t, cliConfig.Disable.MaxLineLength)
			},
		},
		{
			name: "applies no color from atmos config",
			atmosConfig: schema.AtmosConfiguration{
				Settings: schema.AtmosSettings{
					Terminal: schema.Terminal{
						NoColor: true,
					},
				},
			},
			validate: func(t *testing.T) {
				assert.True(t, cliConfig.NoColor)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset state.
			configFilePaths = nil
			tmpExclude = ""
			initEditorConfig = false

			cmd := &cobra.Command{}
			addPersistentFlags(cmd)

			if tt.setup != nil {
				tt.setup(cmd)
			}

			replaceAtmosConfigInConfig(cmd, tt.atmosConfig)
			tt.validate(t)
		})
	}
}
