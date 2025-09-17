package cmd

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	log "github.com/charmbracelet/log"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/cloudposse/gotcha/cmd/gotcha/constants"
	"github.com/cloudposse/gotcha/internal/output"
	"github.com/cloudposse/gotcha/internal/parser"
	"github.com/cloudposse/gotcha/pkg/config"
	pkgErrors "github.com/cloudposse/gotcha/pkg/errors"
	"github.com/cloudposse/gotcha/pkg/stream"
	"github.com/cloudposse/gotcha/pkg/types"
)

// newParseCmd creates the parse subcommand.
func newParseCmd(logger *log.Logger) *cobra.Command {
	parseCmd := &cobra.Command{
		Use:   "parse <json-file>",
		Short: "Parse existing go test JSON output",
		Long:  `Parse and analyze previously generated go test -json output files.`,
		Example: `  # Process results from stdin with terminal output
  go test -json ./... | gotcha parse
  
  # Process results from file  
  gotcha parse test-results.json
  gotcha parse --input=results.json --format=markdown
  
  # Generate GitHub step summary
  gotcha parse --format=github --output=step-summary.md
  
  # Terminal output plus markdown file
  gotcha parse --coverprofile=coverage.out --format=both`,
		Args:          cobra.ExactArgs(1),
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runParse(cmd, args, logger)
		},
	}

	// Output control flags
	parseCmd.Flags().StringP("format", "f", constants.FormatTerminal, "Output format: terminal, json, markdown")
	parseCmd.Flags().StringP("output", "o", "", "Output file for results")
	parseCmd.Flags().String("coverprofile", "", "Coverage profile file for detailed analysis")
	parseCmd.Flags().Bool("exclude-mocks", true, "Exclude mock files from coverage calculations")
	parseCmd.Flags().Bool("generate-summary", false, "Write test summary to test-summary.md file")
	parseCmd.Flags().String("show", "all", "Test display filter: all, failed, passed, skipped, none")
	parseCmd.Flags().String("verbosity", "normal", "Output verbosity: normal, verbose, with-output")

	// CI Integration flags
	parseCmd.Flags().Bool("ci", false, "CI mode - automatically detect and integrate with CI systems")
	parseCmd.Flags().String("post-comment", "", "GitHub PR comment posting strategy: always|never|adaptive|on-failure|on-skip|<os-name> (default: never)")
	parseCmd.Flags().String("github-token", "", "GitHub token for authentication (defaults to GITHUB_TOKEN env)")
	parseCmd.Flags().String("comment-uuid", "", "UUID for comment identification (defaults to GOTCHA_COMMENT_UUID env)")

	return parseCmd
}

// handleOutputFormat processes the output based on the specified format.
func handleOutputFormat(format, outputFile string, jsonData []byte, summary *types.TestSummary, showFilter, verbosityLevel string, logger *log.Logger) error {
	switch format {
	case constants.FormatTerminal:
		// For terminal output, replay the JSON events through StreamProcessor
		// to get proper display with mini indicators for subtests
		if err := replayWithStreamProcessor(jsonData, showFilter, verbosityLevel); err != nil {
			// Fall back to simple output if replay fails
			logger.Debug("Failed to replay with stream processor, using simple output", "error", err)
			if err := output.HandleConsoleOutput(summary); err != nil {
				return fmt.Errorf("failed to print terminal summary: %w", err)
			}
		}

	case constants.FormatJSON:
		if err := output.WriteSummary(summary, constants.FormatJSON, outputFile); err != nil {
			return fmt.Errorf("failed to write JSON output: %w", err)
		}
		logger.Info("JSON summary written", "file", outputFile)

	case constants.FormatMarkdown:
		if err := output.WriteSummary(summary, constants.FormatMarkdown, outputFile); err != nil {
			return fmt.Errorf("failed to write markdown output: %w", err)
		}
		logger.Info("Markdown summary written", "file", outputFile)

	default:
		return fmt.Errorf("%w: %s", pkgErrors.ErrUnsupportedFormat, format)
	}
	return nil
}

// bindParseFlags binds command flags to viper for environment variable support.
func bindParseFlags(cmd *cobra.Command) {
	_ = viper.BindPFlag(FlagFormat, cmd.Flags().Lookup(FlagFormat))
	_ = viper.BindPFlag(FlagOutput, cmd.Flags().Lookup(FlagOutput))
	_ = viper.BindPFlag(FlagCoverprofile, cmd.Flags().Lookup(FlagCoverprofile))
	_ = viper.BindPFlag(FlagExcludeMocks, cmd.Flags().Lookup(FlagExcludeMocks))
	_ = viper.BindPFlag(FlagGenerateSummary, cmd.Flags().Lookup(FlagGenerateSummary))
	_ = viper.BindPFlag(FlagShow, cmd.Flags().Lookup(FlagShow))
	_ = viper.BindPFlag(FlagVerbosity, cmd.Flags().Lookup(FlagVerbosity))
	_ = viper.BindPFlag(FlagPostComment, cmd.Flags().Lookup(FlagPostComment))
	_ = viper.BindEnv(FlagPostComment, "GOTCHA_POST_COMMENT", "POST_COMMENT")
	_ = viper.BindPFlag(FlagGithubToken, cmd.Flags().Lookup(FlagGithubToken))
	_ = viper.BindEnv(FlagGithubToken, "GITHUB_TOKEN")
}

// runParse executes the parse command.
func runParse(cmd *cobra.Command, args []string, logger *log.Logger) error {
	inputFile := args[0]

	// Bind flags to viper for environment variable support
	bindParseFlags(cmd)

	// Get output settings
	format := viper.GetString(FlagFormat)
	outputFile := viper.GetString(FlagOutput)
	coverprofile := viper.GetString(FlagCoverprofile)
	excludeMocks := viper.GetBool(FlagExcludeMocks)
	generateSummary := viper.GetBool(FlagGenerateSummary)
	showFilter := viper.GetString(FlagShow)
	verbosityLevel := viper.GetString(FlagVerbosity)

	// Get CI settings
	ciMode, _ := cmd.Flags().GetBool("ci")
	postStrategy := viper.GetString(FlagPostComment)

	// Check if post-comment flag was actually set by the user
	postFlagPresent := cmd.Flags().Changed(FlagPostComment) || viper.IsSet(FlagPostComment)

	// Normalize the posting strategy
	postStrategy = normalizePostingStrategy(postStrategy, postFlagPresent)

	// Auto-detect CI mode if not explicitly set
	if !ciMode && config.IsCI() {
		ciMode = true
		logger.Debug("CI mode auto-detected",
			"CI", viper.GetBool("ci"),
			"GITHUB_ACTIONS", viper.GetBool("github.actions"))
	}

	// Read the input file
	jsonData, err := os.ReadFile(inputFile)
	if err != nil {
		return fmt.Errorf("failed to read input file: %w", err)
	}

	// Parse the test events for summary data
	jsonReader := bytes.NewReader(jsonData)
	summary, err := parser.ParseTestJSON(jsonReader, coverprofile, excludeMocks)
	if err != nil {
		return fmt.Errorf("failed to parse JSON output: %w", err)
	}

	// Note: Metadata fields would need to be added to types.TestSummary if needed

	// Determine output file if not specified
	if outputFile == "" {
		baseName := strings.TrimSuffix(filepath.Base(inputFile), filepath.Ext(inputFile))
		switch format {
		case constants.FormatJSON:
			outputFile = fmt.Sprintf("%s-summary.json", baseName)
		case constants.FormatMarkdown:
			outputFile = fmt.Sprintf("%s-summary.md", baseName)
		default:
			// Terminal output doesn't need a file
		}
	}

	// Handle output format
	if err := handleOutputFormat(format, outputFile, jsonData, summary, showFilter, verbosityLevel, logger); err != nil {
		return err
	}

	// Generate summary file if requested
	if generateSummary {
		summaryFile := "test-summary.md"
		if err := output.WriteSummary(summary, constants.FormatMarkdown, summaryFile); err != nil {
			logger.Error("Failed to write test summary", "error", err)
		} else {
			logger.Info("Test summary written", "file", summaryFile)
		}
	}

	// Handle CI comment posting if enabled
	logger.Debug("Checking if should post comment",
		"ciMode", ciMode,
		"postStrategy", postStrategy,
		"passed", len(summary.Passed),
		"failed", len(summary.Failed),
		"skipped", len(summary.Skipped))

	shouldPost := shouldPostComment(postStrategy, summary)
	logger.Debug("Should post decision", "shouldPost", shouldPost)

	if ciMode && shouldPost {
		logger.Info("Attempting to post GitHub comment", "strategy", postStrategy)
		// Post comment to CI system
		if err := postGitHubComment(summary, cmd, logger); err != nil {
			// Log error but don't fail the command
			logger.Error("Failed to post CI comment", "error", err)
		}
	} else {
		logger.Debug("Not posting comment", "ciMode", ciMode, "shouldPost", shouldPost)
	}

	// Return with appropriate exit code based on test results
	if len(summary.Failed) > 0 {
		return &testFailureError{
			code:        1,
			testsFailed: len(summary.Failed),
			testsPassed: len(summary.Passed),
		}
	}

	return nil
}

// replayWithStreamProcessor replays JSON test events through the StreamProcessor
// to display tests with proper formatting including mini indicators for subtests.
func replayWithStreamProcessor(jsonData []byte, showFilter, verbosityLevel string) error {
	// Create a dummy writer for JSON output (required by StreamProcessor)
	var jsonBuffer bytes.Buffer

	// Create a stream processor
	processor := stream.NewStreamProcessor(&jsonBuffer, showFilter, "", verbosityLevel)

	// Create a reader for the JSON data
	reader := bytes.NewReader(jsonData)

	// Process the stream
	if err := processor.ProcessStream(reader); err != nil {
		return fmt.Errorf("failed to process stream: %w", err)
	}

	// Print the final summary
	processor.PrintSummary()

	return nil
}
