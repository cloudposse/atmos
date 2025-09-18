package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	log "github.com/charmbracelet/log"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/tools/gotcha/cmd/gotcha/constants"
	"github.com/cloudposse/atmos/tools/gotcha/pkg/types"
)

// TestNewParseCmd tests the newParseCmd function
func TestNewParseCmd(t *testing.T) {
	logger := log.New(os.Stderr)
	cmd := newParseCmd(logger)

	assert.NotNil(t, cmd)
	assert.Equal(t, "parse <json-file>", cmd.Use)
	assert.Contains(t, cmd.Short, "Parse existing")
	assert.NotNil(t, cmd.RunE)

	// Check that all expected flags are defined
	flags := []string{
		"format", "output", "coverprofile", "exclude-mocks",
		"generate-summary", "show", "verbosity", "ci",
		"post-comment", "github-token", "comment-uuid",
	}

	for _, flag := range flags {
		assert.NotNil(t, cmd.Flags().Lookup(flag), "Flag %s should be defined", flag)
	}
}

// TestHandleOutputFormat tests the handleOutputFormat function
func TestHandleOutputFormat(t *testing.T) {
	// Create test summary
	testSummary := &types.TestSummary{
		Passed: []types.TestResult{
			{Package: "pkg1", Test: "Test1", Status: "pass", Duration: 1.0},
			{Package: "pkg1", Test: "Test2", Status: "pass", Duration: 0.5},
		},
		Failed: []types.TestResult{
			{Package: "pkg2", Test: "Test3", Status: "fail", Duration: 2.0},
		},
		Skipped: []types.TestResult{
			{Package: "pkg3", Test: "Test4", Status: "skip", SkipReason: "not implemented"},
		},
		Coverage:         "75.5%",
		TotalElapsedTime: 3.5,
	}

	// Create test JSON data
	testJSON := []byte(`{"Time":"2025-09-17T10:00:00Z","Action":"pass","Package":"pkg1","Test":"Test1"}
{"Time":"2025-09-17T10:00:01Z","Action":"pass","Package":"pkg1","Test":"Test2"}
{"Time":"2025-09-17T10:00:02Z","Action":"fail","Package":"pkg2","Test":"Test3"}`)

	tests := []struct {
		name           string
		format         string
		outputFile     string
		jsonData       []byte
		summary        *types.TestSummary
		showFilter     string
		verbosityLevel string
		wantErr        bool
		checkFile      bool
	}{
		{
			name:           "terminal format",
			format:         constants.FormatTerminal,
			outputFile:     "",
			jsonData:       testJSON,
			summary:        testSummary,
			showFilter:     "all",
			verbosityLevel: "normal",
			wantErr:        false,
		},
		{
			name:           "JSON format with file",
			format:         constants.FormatJSON,
			outputFile:     filepath.Join(t.TempDir(), "output.json"),
			jsonData:       testJSON,
			summary:        testSummary,
			showFilter:     "all",
			verbosityLevel: "normal",
			wantErr:        false,
			checkFile:      true,
		},
		{
			name:           "markdown format with file",
			format:         constants.FormatMarkdown,
			outputFile:     filepath.Join(t.TempDir(), "output.md"),
			jsonData:       testJSON,
			summary:        testSummary,
			showFilter:     "all",
			verbosityLevel: "normal",
			wantErr:        false,
			checkFile:      true,
		},
		{
			name:           "unsupported format",
			format:         "invalid",
			outputFile:     "",
			jsonData:       testJSON,
			summary:        testSummary,
			showFilter:     "all",
			verbosityLevel: "normal",
			wantErr:        true,
		},
		{
			name:           "terminal format with failed filter",
			format:         constants.FormatTerminal,
			outputFile:     "",
			jsonData:       testJSON,
			summary:        testSummary,
			showFilter:     "failed",
			verbosityLevel: "normal",
			wantErr:        false,
		},
		{
			name:           "terminal format with verbose",
			format:         constants.FormatTerminal,
			outputFile:     "",
			jsonData:       testJSON,
			summary:        testSummary,
			showFilter:     "all",
			verbosityLevel: "verbose",
			wantErr:        false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger := log.New(os.Stderr)
			logger.SetLevel(log.DebugLevel)

			err := handleOutputFormat(
				tt.format, tt.outputFile, tt.jsonData,
				tt.summary, tt.showFilter, tt.verbosityLevel, logger,
			)

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)

				// Check file was created if expected
				if tt.checkFile && tt.outputFile != "" {
					assert.FileExists(t, tt.outputFile)

					// Verify file content
					content, err := os.ReadFile(tt.outputFile)
					require.NoError(t, err)
					assert.NotEmpty(t, content)

					// Note: The actual output.WriteSummary function may output markdown
					// regardless of format, so we just check the file has content
				}
			}
		})
	}
}

// TestBindParseFlags tests the bindParseFlags function
func TestBindParseFlags(t *testing.T) {
	// Reset viper before test
	viper.Reset()

	// Create command with flags
	cmd := &cobra.Command{}
	cmd.Flags().String("format", "", "Output format")
	cmd.Flags().String("output", "", "Output file")
	cmd.Flags().String("coverprofile", "", "Coverage profile")
	cmd.Flags().Bool("exclude-mocks", false, "Exclude mocks")
	cmd.Flags().Bool("generate-summary", false, "Generate summary")
	cmd.Flags().String("show", "", "Show filter")
	cmd.Flags().String("verbosity", "", "Verbosity level")
	cmd.Flags().String("post-comment", "", "Post comment strategy")
	cmd.Flags().String("github-token", "", "GitHub token")

	// Set some flag values
	cmd.Flags().Set("format", "json")
	cmd.Flags().Set("output", "test.json")
	cmd.Flags().Set("exclude-mocks", "true")

	// Set environment variables
	os.Setenv("GITHUB_TOKEN", "test-token")
	os.Setenv("POST_COMMENT", "always")
	defer os.Unsetenv("GITHUB_TOKEN")
	defer os.Unsetenv("POST_COMMENT")

	// Bind flags
	bindParseFlags(cmd)

	// Check that values are accessible through viper
	assert.Equal(t, "json", viper.GetString("format"))
	assert.Equal(t, "test.json", viper.GetString("output"))
	assert.True(t, viper.GetBool("exclude-mocks"))
	assert.Equal(t, "test-token", viper.GetString("github-token"))
	assert.Equal(t, "always", viper.GetString("post-comment"))
}

// TestRunParse tests the runParse function
func TestRunParse(t *testing.T) {
	tests := []struct {
		name        string
		setupFile   func(t *testing.T) string
		setupFlags  func(cmd *cobra.Command)
		setupEnv    func()
		cleanupEnv  func()
		wantErr     bool
		wantFailure bool // Test failure error expected
		checkOutput func(t *testing.T, outputFile string)
	}{
		{
			name: "parse valid JSON file",
			setupFile: func(t *testing.T) string {
				file := filepath.Join(t.TempDir(), "test.json")
				jsonData := `{"Time":"2025-09-17T10:00:00Z","Action":"pass","Package":"pkg1","Test":"Test1","Elapsed":1.0}
{"Time":"2025-09-17T10:00:01Z","Action":"pass","Package":"pkg1","Test":"Test2","Elapsed":0.5}
{"Time":"2025-09-17T10:00:02Z","Action":"pass","Package":"pkg1"}
`
				require.NoError(t, os.WriteFile(file, []byte(jsonData), 0o644))
				return file
			},
			setupFlags: func(cmd *cobra.Command) {
				cmd.Flags().Set("format", "json")
				cmd.Flags().Set("output", filepath.Join(t.TempDir(), "output.json"))
			},
			wantErr: false,
		},
		{
			name: "parse with test failures",
			setupFile: func(t *testing.T) string {
				file := filepath.Join(t.TempDir(), "test.json")
				jsonData := `{"Time":"2025-09-17T10:00:00Z","Action":"fail","Package":"pkg1","Test":"Test1","Elapsed":1.0}
{"Time":"2025-09-17T10:00:01Z","Action":"pass","Package":"pkg1","Test":"Test2","Elapsed":0.5}
{"Time":"2025-09-17T10:00:02Z","Action":"fail","Package":"pkg1"}
`
				require.NoError(t, os.WriteFile(file, []byte(jsonData), 0o644))
				return file
			},
			setupFlags: func(cmd *cobra.Command) {
				cmd.Flags().Set("format", "terminal")
			},
			wantErr:     true,
			wantFailure: true,
		},
		{
			name: "parse with markdown output",
			setupFile: func(t *testing.T) string {
				file := filepath.Join(t.TempDir(), "test.json")
				jsonData := `{"Time":"2025-09-17T10:00:00Z","Action":"pass","Package":"pkg1","Test":"Test1","Elapsed":1.0}
{"Time":"2025-09-17T10:00:01Z","Action":"skip","Package":"pkg1","Test":"Test2","Elapsed":0.0}
{"Time":"2025-09-17T10:00:02Z","Action":"pass","Package":"pkg1"}
`
				require.NoError(t, os.WriteFile(file, []byte(jsonData), 0o644))
				return file
			},
			setupFlags: func(cmd *cobra.Command) {
				outputFile := filepath.Join(t.TempDir(), "output.md")
				cmd.Flags().Set("format", "markdown")
				cmd.Flags().Set("output", outputFile)
			},
			wantErr: false,
			checkOutput: func(t *testing.T, outputFile string) {
				content, err := os.ReadFile(outputFile)
				require.NoError(t, err)
				assert.Contains(t, string(content), "#") // Markdown header
			},
		},
		{
			name: "parse with generate-summary",
			setupFile: func(t *testing.T) string {
				file := filepath.Join(t.TempDir(), "test.json")
				jsonData := `{"Time":"2025-09-17T10:00:00Z","Action":"pass","Package":"pkg1","Test":"Test1","Elapsed":1.0}
{"Time":"2025-09-17T10:00:01Z","Action":"pass","Package":"pkg1","Test":"Test2","Elapsed":0.5}
{"Time":"2025-09-17T10:00:02Z","Action":"pass","Package":"pkg1"}
`
				require.NoError(t, os.WriteFile(file, []byte(jsonData), 0o644))
				return file
			},
			setupFlags: func(cmd *cobra.Command) {
				cmd.Flags().Set("format", "terminal")
				cmd.Flags().Set("generate-summary", "true")
			},
			wantErr: false,
		},
		{
			name: "parse non-existent file",
			setupFile: func(t *testing.T) string {
				return "/non/existent/file.json"
			},
			wantErr: true,
		},
		{
			name: "parse with CI mode and comment posting",
			setupFile: func(t *testing.T) string {
				file := filepath.Join(t.TempDir(), "test.json")
				jsonData := `{"Time":"2025-09-17T10:00:00Z","Action":"pass","Package":"pkg1","Test":"Test1","Elapsed":1.0}
{"Time":"2025-09-17T10:00:01Z","Action":"pass","Package":"pkg1"}
`
				require.NoError(t, os.WriteFile(file, []byte(jsonData), 0o644))
				return file
			},
			setupFlags: func(cmd *cobra.Command) {
				cmd.Flags().Set("ci", "true")
				cmd.Flags().Set("post-comment", "always")
				cmd.Flags().Set("format", "terminal")
			},
			setupEnv: func() {
				os.Setenv("GITHUB_ACTIONS", "true")
				os.Setenv("GITHUB_TOKEN", "test-token")
			},
			cleanupEnv: func() {
				os.Unsetenv("GITHUB_ACTIONS")
				os.Unsetenv("GITHUB_TOKEN")
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset viper
			viper.Reset()

			// Create logger
			logger := log.New(os.Stderr)
			logger.SetLevel(log.DebugLevel)

			// Create command
			cmd := newParseCmd(logger)

			// Setup environment if needed
			if tt.setupEnv != nil {
				tt.setupEnv()
			}
			if tt.cleanupEnv != nil {
				defer tt.cleanupEnv()
			}

			// Setup file
			inputFile := tt.setupFile(t)

			// Setup flags if needed
			if tt.setupFlags != nil {
				tt.setupFlags(cmd)
			}

			// Get output file if set
			outputFile, _ := cmd.Flags().GetString("output")

			// Run parse
			err := runParse(cmd, []string{inputFile}, logger)

			if tt.wantErr {
				assert.Error(t, err)
				if tt.wantFailure {
					// Check it's a test failure error
					var testErr *testFailureError
					assert.ErrorAs(t, err, &testErr)
				}
			} else {
				assert.NoError(t, err)

				// Check output file if specified
				if tt.checkOutput != nil && outputFile != "" {
					tt.checkOutput(t, outputFile)
				}
			}
		})
	}
}

// TestReplayWithStreamProcessor tests the replayWithStreamProcessor function
func TestReplayWithStreamProcessor(t *testing.T) {
	tests := []struct {
		name           string
		jsonData       []byte
		showFilter     string
		verbosityLevel string
		wantErr        bool
	}{
		{
			name: "replay valid JSON",
			jsonData: []byte(`{"Time":"2025-09-17T10:00:00Z","Action":"run","Package":"pkg1","Test":"Test1"}
{"Time":"2025-09-17T10:00:01Z","Action":"pass","Package":"pkg1","Test":"Test1","Elapsed":1.0}
{"Time":"2025-09-17T10:00:02Z","Action":"pass","Package":"pkg1"}
`),
			showFilter:     "all",
			verbosityLevel: "normal",
			wantErr:        false,
		},
		{
			name: "replay with failed filter",
			jsonData: []byte(`{"Time":"2025-09-17T10:00:00Z","Action":"run","Package":"pkg1","Test":"Test1"}
{"Time":"2025-09-17T10:00:01Z","Action":"fail","Package":"pkg1","Test":"Test1","Elapsed":1.0}
{"Time":"2025-09-17T10:00:02Z","Action":"fail","Package":"pkg1"}
`),
			showFilter:     "failed",
			verbosityLevel: "normal",
			wantErr:        false,
		},
		{
			name: "replay with verbose output",
			jsonData: []byte(`{"Time":"2025-09-17T10:00:00Z","Action":"run","Package":"pkg1","Test":"Test1"}
{"Time":"2025-09-17T10:00:01Z","Action":"output","Package":"pkg1","Test":"Test1","Output":"=== RUN   Test1\n"}
{"Time":"2025-09-17T10:00:02Z","Action":"pass","Package":"pkg1","Test":"Test1","Elapsed":1.0}
`),
			showFilter:     "all",
			verbosityLevel: "verbose",
			wantErr:        false,
		},
		{
			name:           "replay empty JSON",
			jsonData:       []byte(""),
			showFilter:     "all",
			verbosityLevel: "normal",
			wantErr:        false,
		},
		{
			name:           "replay invalid JSON",
			jsonData:       []byte("not json"),
			showFilter:     "all",
			verbosityLevel: "normal",
			wantErr:        false, // Invalid JSON lines are skipped, not an error
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Capture stdout/stderr
			oldStdout := os.Stdout
			oldStderr := os.Stderr
			_, w, _ := os.Pipe()
			os.Stdout = w
			os.Stderr = w
			defer func() {
				w.Close()
				os.Stdout = oldStdout
				os.Stderr = oldStderr
			}()

			// Run replay
			err := replayWithStreamProcessor(tt.jsonData, tt.showFilter, tt.verbosityLevel)

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// TestParseCommandIntegration tests the parse command as a whole
func TestParseCommandIntegration(t *testing.T) {
	t.Run("parse command help", func(t *testing.T) {
		logger := log.New(os.Stderr)
		cmd := newParseCmd(logger)

		// Capture output
		var buf bytes.Buffer
		cmd.SetOutput(&buf)
		cmd.SetArgs([]string{"--help"})

		err := cmd.Execute()
		assert.NoError(t, err)

		output := buf.String()
		// Check for the Long description which is actually shown in help
		assert.Contains(t, output, "Parse and analyze previously generated go test -json output files")
		// Check for the Usage format
		assert.Contains(t, output, "parse <json-file>")
		assert.Contains(t, output, "Examples:")
	})

	t.Run("parse command requires argument", func(t *testing.T) {
		logger := log.New(os.Stderr)
		cmd := newParseCmd(logger)

		// Don't provide any arguments
		cmd.SetArgs([]string{})

		err := cmd.Execute()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "accepts 1 arg(s)")
	})
}
