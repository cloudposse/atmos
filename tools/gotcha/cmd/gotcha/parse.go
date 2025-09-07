package cmd

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	log "github.com/charmbracelet/log"
	"github.com/spf13/cobra"

	"github.com/cloudposse/atmos/tools/gotcha/internal/output"
	"github.com/cloudposse/atmos/tools/gotcha/internal/parser"
)

// newParseCmd creates the parse subcommand.
func newParseCmd(logger *log.Logger) *cobra.Command {
	parseCmd := &cobra.Command{
		Use:   "parse <json-file>",
		Short: "Parse existing go test JSON output",
		Long:  `Parse and analyze previously generated go test -json output files.`,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runParse(cmd, args, logger)
		},
	}

	// Output control flags
	parseCmd.Flags().StringP("format", "f", "terminal", "Output format: terminal, json, markdown")
	parseCmd.Flags().StringP("output", "o", "", "Output file for results")

	// CI Integration flags
	parseCmd.Flags().Bool("ci", false, "CI mode - automatically detect and integrate with CI systems")
	parseCmd.Flags().String("post", "", "Post comment strategy: always, on-failure, off")
	parseCmd.Flags().String("comment-uuid", "", "Unique identifier for updating existing CI comment")

	return parseCmd
}

// runParse executes the parse command.
func runParse(cmd *cobra.Command, args []string, logger *log.Logger) error {
	inputFile := args[0]

	// Get output settings
	format, _ := cmd.Flags().GetString("format")
	outputFile, _ := cmd.Flags().GetString("output")

	// Get CI settings
	ciMode, _ := cmd.Flags().GetBool("ci")
	postStrategy, _ := cmd.Flags().GetString("post")

	// Check if post flag was actually set by the user
	postFlagPresent := cmd.Flags().Changed("post")

	// Normalize the posting strategy
	postStrategy = normalizePostingStrategy(postStrategy, postFlagPresent)

	// Auto-detect CI mode if not explicitly set
	if !ciMode && (os.Getenv("CI") != "" || os.Getenv("GITHUB_ACTIONS") != "") {
		ciMode = true
		logger.Debug("CI mode auto-detected", "CI", os.Getenv("CI"), "GITHUB_ACTIONS", os.Getenv("GITHUB_ACTIONS"))
	}

	// Read the input file
	jsonData, err := os.ReadFile(inputFile)
	if err != nil {
		return fmt.Errorf("failed to read input file: %w", err)
	}

	// Parse the test events
	jsonReader := bytes.NewReader(jsonData)
	summary, err := parser.ParseTestJSON(jsonReader, "", false)
	if err != nil {
		return fmt.Errorf("failed to parse JSON output: %w", err)
	}

	// Note: Metadata fields would need to be added to types.TestSummary if needed

	// Determine output file if not specified
	if outputFile == "" {
		baseName := strings.TrimSuffix(filepath.Base(inputFile), filepath.Ext(inputFile))
		switch format {
		case "json":
			outputFile = fmt.Sprintf("%s-summary.json", baseName)
		case "markdown":
			outputFile = fmt.Sprintf("%s-summary.md", baseName)
		default:
			// Terminal output doesn't need a file
		}
	}

	// Handle different output formats
	switch format {
	case "terminal":
		// Display summary to terminal
		if err := output.HandleConsoleOutput(summary); err != nil {
			return fmt.Errorf("failed to print terminal summary: %w", err)
		}

	case "json":
		if err := output.WriteSummary(summary, "json", outputFile); err != nil {
			return fmt.Errorf("failed to write JSON output: %w", err)
		}
		logger.Info("JSON summary written", "file", outputFile)

	case "markdown":
		if err := output.WriteSummary(summary, "markdown", outputFile); err != nil {
			return fmt.Errorf("failed to write markdown output: %w", err)
		}
		logger.Info("Markdown summary written", "file", outputFile)

	default:
		return fmt.Errorf("unsupported format: %s", format)
	}

	// Handle CI comment posting if enabled
	if ciMode && shouldPostComment(postStrategy, summary) {
		// Post comment to CI system
		if err := postGitHubComment(summary, cmd, logger); err != nil {
			// Log error but don't fail the command
			logger.Error("Failed to post CI comment", "error", err)
		}
	}

	// Return with appropriate exit code based on test results
	if len(summary.Failed) > 0 {
		return &testFailureError{code: 1}
	}

	return nil
}