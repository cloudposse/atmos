package output

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/cloudposse/atmos/tools/gotcha/internal/logger"
	"github.com/cloudposse/atmos/tools/gotcha/internal/markdown"
	"github.com/cloudposse/atmos/tools/gotcha/internal/tui"
	"github.com/cloudposse/atmos/tools/gotcha/pkg/config"
	"github.com/cloudposse/atmos/tools/gotcha/pkg/constants"
	"github.com/cloudposse/atmos/tools/gotcha/pkg/types"
)

// writeSummary writes the test summary in the specified format.
func WriteSummary(summary *types.TestSummary, format, outputFile string) error {
	if format == constants.FormatGitHub {
		return writeGitHubSummary(summary, outputFile)
	}

	// For other formats, use the original logic.
	output, outputPath, err := openOutput(format, outputFile)
	if err != nil {
		return err
	}
	// Handle closing for files that need it.
	if closer, ok := output.(io.Closer); ok && output != os.Stdout {
		defer closer.Close()
	}
	// Write the markdown content.
	markdown.WriteContent(output, summary, format)
	// Log success message for file outputs.
	if outputPath != constants.StdoutPath && outputPath != "" {
		absPath, _ := filepath.Abs(outputPath)
		if fileInfo, err := os.Stat(outputPath); err == nil {
			fmt.Fprintf(os.Stderr, "%s Markdown summary written to %s (%d bytes)\n", tui.PassStyle.Render(tui.CheckPass), absPath, fileInfo.Size())
		} else {
			fmt.Fprintf(os.Stderr, "%s Markdown summary written to %s\n", tui.PassStyle.Render(tui.CheckPass), absPath)
		}
	}
	return nil
}

// writeGitHubSummary handles GitHub-specific summary writing.
func writeGitHubSummary(summary *types.TestSummary, outputFile string) error {
	// 1. Write to GITHUB_STEP_SUMMARY (if available).
	githubWriter, githubPath, err := openGitHubOutput("")
	if err == nil {
		defer func() {
			if closer, ok := githubWriter.(io.Closer); ok {
				closer.Close()
			}
		}()
		markdown.WriteContent(githubWriter, summary, constants.FormatGitHub)
		if githubPath != "" {
			if fileInfo, err := os.Stat(githubPath); err == nil {
				fmt.Fprintf(os.Stderr, "%s GitHub step summary written to %s (%d bytes)\n", tui.PassStyle.Render(tui.CheckPass), githubPath, fileInfo.Size())
			} else {
				fmt.Fprintf(os.Stderr, "%s GitHub step summary written to %s\n", tui.PassStyle.Render(tui.CheckPass), githubPath)
			}
		}
	}

	// 2. ALWAYS write to a regular file for persistence.
	regularFile := outputFile
	if regularFile == "" {
		regularFile = "test-summary.md" // Default file if none specified.
	}

	file, err := os.Create(regularFile)
	if err != nil {
		return fmt.Errorf("failed to create output file: %w", err)
	}
	defer file.Close()

	markdown.WriteContent(file, summary, constants.FormatGitHub)
	absPath, _ := filepath.Abs(regularFile)
	if fileInfo, err := os.Stat(regularFile); err == nil {
		fmt.Fprintf(os.Stderr, "%s Markdown summary written to %s (%d bytes)\n", tui.PassStyle.Render(tui.CheckPass), absPath, fileInfo.Size())
	} else {
		fmt.Fprintf(os.Stderr, "%s Markdown summary written to %s\n", tui.PassStyle.Render(tui.CheckPass), absPath)
	}

	return nil
}

// openOutput opens the appropriate output destination.
func openOutput(format, outputFile string) (io.Writer, string, error) {
	if format == constants.FormatGitHub {
		return openGitHubOutput(outputFile)
	}

	if outputFile == "" || outputFile == constants.StdinMarker {
		return os.Stdout, constants.StdoutPath, nil
	}

	file, err := os.Create(outputFile)
	if err != nil {
		return nil, "", fmt.Errorf("failed to create output file: %w", err)
	}

	return file, outputFile, nil
}

// openGitHubOutput handles GitHub-specific output logic.
func openGitHubOutput(outputFile string) (io.Writer, string, error) {
	githubStepSummary := config.GetGitHubStepSummary()

	if githubStepSummary != "" {
		// Running in GitHub Actions - write to GITHUB_STEP_SUMMARY.
		file, err := os.OpenFile(githubStepSummary, os.O_APPEND|os.O_WRONLY, constants.FilePermissions)
		if err != nil {
			return nil, "", fmt.Errorf("failed to open GITHUB_STEP_SUMMARY: %w", err)
		}
		return file, githubStepSummary, nil
	}

	// Running locally - use test-summary.md or specified file.
	defaultFile := "test-summary.md"
	if outputFile != "" {
		defaultFile = outputFile
	}
	file, err := os.Create(defaultFile)
	if err != nil {
		return nil, "", fmt.Errorf("failed to create summary file: %w", err)
	}
	// Inform the user.
	logger.GetLogger().Info("GITHUB_STEP_SUMMARY not set (skipped)")
	return file, defaultFile, nil
}

// HandleOutput handles writing output in the specified format.
func HandleOutput(summary *types.TestSummary, format, outputFile string, generateSummary bool) error {
	switch format {
	case "terminal":
		return HandleConsoleOutput(summary)
	case "markdown":
		if generateSummary {
			// Use test-summary.md in current directory if no output file specified
			if outputFile == "" {
				outputFile = "test-summary.md"
			}
			return WriteSummary(summary, format, outputFile)
		}
		return nil
	case "github":
		if generateSummary {
			// Use test-summary.md in current directory if no output file specified
			if outputFile == "" {
				outputFile = "test-summary.md"
			}
			return WriteSummary(summary, format, outputFile)
		}
		return nil
	case "both":
		if err := HandleConsoleOutput(summary); err != nil {
			return err
		}
		if generateSummary {
			// Use test-summary.md in current directory if no output file specified
			if outputFile == "" {
				outputFile = "test-summary.md"
			}
			return WriteSummary(summary, "markdown", outputFile)
		}
		return nil
	}
	return fmt.Errorf("%w: %s", types.ErrUnsupportedFormat, format)
}

// HandleConsoleOutput writes console-formatted output.
func HandleConsoleOutput(summary *types.TestSummary) error {
	// This function is called by the parse command to display test results.
	// Currently it's just a placeholder that doesn't properly display test results.
	// The actual display logic with mini indicators is in pkg/stream/display.go
	// which is used by the stream command but not by parse.
	// 
	// TODO: Refactor to use the same display logic as stream command
	// to show parent tests with mini indicators for subtests.
	//
	// For now, just output basic information
	if len(summary.Failed) > 0 {
		fmt.Print("test failed")
	} else if len(summary.Passed) > 0 {
		fmt.Print("tests passed")
	} else if len(summary.Skipped) > 0 {
		fmt.Print("tests skipped")
	} else {
		fmt.Print("no tests found")
	}

	return nil
}
