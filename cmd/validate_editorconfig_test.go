package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"testing"

	"github.com/editorconfig-checker/editorconfig-checker/v3/pkg/config"
	er "github.com/editorconfig-checker/editorconfig-checker/v3/pkg/error"
	"github.com/editorconfig-checker/editorconfig-checker/v3/pkg/outputformat"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/ci"
	"github.com/cloudposse/atmos/pkg/schema"
	validateReport "github.com/cloudposse/atmos/pkg/validation"
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
	require.NoError(t, initializeConfig(editorConfigCmd))
}

// TestRunEditorConfigDryRun tests the dry-run path in runEditorConfig.
func TestRunEditorConfigDryRun(t *testing.T) {
	// Save original state.
	originalConfig := currentConfig
	originalCliConfig := cliConfig
	defer func() {
		currentConfig = originalConfig
		cliConfig = originalCliConfig
	}()

	// Create a minimal config for dry-run mode.
	cfg := config.NewConfig([]string{})
	cfg.DryRun = true
	currentConfig = cfg
	cliConfig = config.Config{DryRun: true}

	// This should not panic and should list files (if any match).
	// In dry-run mode, runEditorConfig just lists files without validation.
	require.NoError(t, runEditorConfig(editorConfigCmd))
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
	originalEditorConfigSARIF := editorConfigSARIF
	defer func() {
		configFilePaths = originalConfigFilePaths
		tmpExclude = originalTmpExclude
		initEditorConfig = originalInitEditorConfig
		cliConfig = originalCliConfig
		editorConfigSARIF = originalEditorConfigSARIF
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

			require.NoError(t, replaceAtmosConfigInConfig(cmd, tt.atmosConfig))
			tt.validate(t)
		})
	}
}

func TestConfigureEditorConfigFormat(t *testing.T) {
	original := cliConfig
	t.Cleanup(func() { cliConfig = original })

	for _, format := range []string{"default", "gcc", "codeclimate", "github-actions"} {
		t.Run(format, func(t *testing.T) {
			isSARIF, err := configureEditorConfigFormat(format)
			require.NoError(t, err)
			assert.False(t, isSARIF)
		})
	}
	isSARIF, err := configureEditorConfigFormat("sarif")
	require.NoError(t, err)
	assert.True(t, isSARIF)
	isSARIF, err = configureEditorConfigFormat("rich")
	require.NoError(t, err)
	assert.False(t, isSARIF)
	assert.True(t, editorConfigRich)
	_, err = configureEditorConfigFormat("xml")
	assert.Error(t, err)
}

func TestEditorConfigDiagnosticsNormalizesLocations(t *testing.T) {
	report := editorConfigDiagnostics([]er.ValidationErrors{{
		FilePath: "example.tf",
		Errors: []er.ValidationError{
			{LineNumber: -1, Message: errors.New("missing final newline")},
			{LineNumber: 4, AdditionalIdenticalErrorCount: 2, Message: errors.New("trailing whitespace")},
		},
	}})
	require.Len(t, report.Diagnostics, 2)
	assert.Equal(t, "editorconfig", report.Diagnostics[0].Source)
	assert.Zero(t, report.Diagnostics[0].Line)
	assert.Equal(t, 4, report.Diagnostics[1].Line)
	assert.Equal(t, 6, report.Diagnostics[1].EndLine)
}

func TestRunEditorConfigRichOutput(t *testing.T) {
	originalAtmosConfig := atmosConfig
	originalCurrentConfig := currentConfig
	originalCLIConfig := cliConfig
	originalPaths := configFilePaths
	originalExclude := tmpExclude
	originalInit := initEditorConfig
	originalSARIF := editorConfigSARIF
	originalRich := editorConfigRich
	originalFormat := format
	t.Cleanup(func() {
		atmosConfig = originalAtmosConfig
		currentConfig = originalCurrentConfig
		cliConfig = originalCLIConfig
		configFilePaths = originalPaths
		tmpExclude = originalExclude
		initEditorConfig = originalInit
		editorConfigSARIF = originalSARIF
		editorConfigRich = originalRich
		format = originalFormat
	})

	project := t.TempDir()
	t.Chdir(project)
	require.NoError(t, os.WriteFile(".editorconfig", []byte("root = true\n[*]\nend_of_line = lf\ninsert_final_newline = true\ntrim_trailing_whitespace = true\n"), 0o600))
	require.NoError(t, os.WriteFile("valid.txt", []byte("valid\n"), 0o600))
	atmosConfig = schema.AtmosConfiguration{}
	cliConfig = config.Config{}
	configFilePaths = nil
	tmpExclude = ""
	initEditorConfig = false

	command := &cobra.Command{}
	addPersistentFlags(command)
	require.NoError(t, command.ParseFlags(nil))
	require.NoError(t, command.Flags().Set("format", "rich"))
	var output bytes.Buffer
	command.SetOut(&output)

	require.NoError(t, runEditorConfig(command))
	assert.Contains(t, output.String(), "EditorConfig validation passed")

	output.Reset()
	require.NoError(t, os.WriteFile("invalid.txt", []byte("invalid  \n"), 0o600))
	err := runEditorConfig(command)
	require.Error(t, err)
	assert.Equal(t, 1, errUtils.GetExitCode(err))
	assert.Contains(t, output.String(), "Trailing whitespace")

	output.Reset()
	require.NoError(t, os.Remove("invalid.txt"))
	require.NoError(t, command.Flags().Set("format", "sarif"))
	require.NoError(t, runEditorConfig(command))
	assert.Contains(t, output.String(), `"version": "2.1.0"`)
}

func TestRequestedEditorConfigFormatPrecedence(t *testing.T) {
	originalFormat := format
	t.Cleanup(func() { format = originalFormat })
	config := schema.AtmosConfiguration{Validate: schema.Validate{EditorConfig: schema.EditorConfig{Format: "gcc"}}}
	cmd := &cobra.Command{}
	addPersistentFlags(cmd)
	// Merge persistent flags into cmd.Flags() the way cobra does during
	// execution, so Set/Changed below see the format flag.
	require.NoError(t, cmd.ParseFlags(nil))

	t.Setenv("ATMOS_VALIDATE_FORMAT", "sarif")
	assert.Equal(t, "sarif", requestedEditorConfigFormat(cmd, config))
	format = "codeclimate"
	require.NoError(t, cmd.Flags().Set("format", format))
	assert.Equal(t, "codeclimate", requestedEditorConfigFormat(cmd, config))
}

func TestEmitEditorConfigCI(t *testing.T) {
	originalConfig := atmosConfig
	originalCLIConfig := cliConfig
	originalAnnotate := editorConfigAnnotate
	originalReportSARIF := editorConfigReportSARIF
	t.Cleanup(func() {
		atmosConfig = originalConfig
		cliConfig = originalCLIConfig
		editorConfigAnnotate = originalAnnotate
		editorConfigReportSARIF = originalReportSARIF
	})

	resultUploads := true
	atmosConfig = schema.AtmosConfiguration{CI: schema.CIConfig{
		Enabled: true,
		Results: schema.CIResultsConfig{Enabled: &resultUploads},
	}}
	cliConfig.Format = outputformat.Default
	var annotations []ci.Annotation
	var uploads []ci.SARIFReport
	editorConfigAnnotate = func(items []ci.Annotation) error {
		annotations = append(annotations, items...)
		return nil
	}
	editorConfigReportSARIF = func(_ context.Context, report ci.SARIFReport) error {
		uploads = append(uploads, report)
		return nil
	}
	cmd := &cobra.Command{}
	cmd.Flags().Bool("ci", false, "")
	require.NoError(t, cmd.Flags().Set("ci", "true"))

	emitEditorConfigCI(cmd, validateReport.Report{Diagnostics: []validateReport.Diagnostic{{
		RuleID: "editorconfig", Severity: validateReport.SeverityError,
		Message: "wrong indentation", File: "context.tf", Line: 4,
	}}})
	require.Len(t, annotations, 1)
	assert.Equal(t, 4, annotations[0].StartLine)
	require.Len(t, uploads, 1)
	assert.Equal(t, "validate-editorconfig", uploads[0].Category)
	var document struct {
		Version string `json:"version"`
		Runs    []struct {
			Results []struct {
				RuleID string `json:"ruleId"`
			} `json:"results"`
		} `json:"runs"`
	}
	require.NoError(t, json.Unmarshal(uploads[0].Body, &document))
	assert.Equal(t, "2.1.0", document.Version)
	require.Len(t, document.Runs, 1)
	require.Len(t, document.Runs[0].Results, 1)
	assert.Equal(t, "editorconfig", document.Runs[0].Results[0].RuleID)
}

// TestEditorConfigCmdCIFlagRegisteredThroughStandardParser guards against
// regressing back to direct viper.BindPFlag/viper.BindEnv calls: the "ci"
// flag must be registered on editorConfigCmd and its ATMOS_CI/CI env vars
// must resolve through Viper for pkg/ci.ModeEnabled's fallback branch.
func TestEditorConfigCmdCIFlagRegisteredThroughStandardParser(t *testing.T) {
	flag := editorConfigCmd.PersistentFlags().Lookup("ci")
	require.NotNil(t, flag, "expected the ci flag to be registered on editorConfigCmd")
	assert.Equal(t, "false", flag.DefValue)

	t.Setenv("ATMOS_CI", "true")
	assert.True(t, ci.ModeEnabled(&cobra.Command{}), "expected ATMOS_CI env var to resolve through Viper via the standard parser binding")
}
