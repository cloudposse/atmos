package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
)

// writeSummary writes the test summary in the specified format.
func writeSummary(summary *TestSummary, format, outputFile string) error {
	if format == formatGitHub {
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
	writeMarkdownContent(output, summary, format)
	// Log success message for file outputs.
	if outputPath != stdoutPath && outputPath != "" {
		absPath, _ := filepath.Abs(outputPath)
		fmt.Fprintf(os.Stderr, "✅ Test summary written to: %s\n", absPath)
	}
	return nil
}

// writeGitHubSummary handles GitHub-specific summary writing.
func writeGitHubSummary(summary *TestSummary, outputFile string) error {
	// 1. Write to GITHUB_STEP_SUMMARY (if available).
	githubWriter, githubPath, err := openGitHubOutput("")
	if err == nil {
		defer func() {
			if closer, ok := githubWriter.(io.Closer); ok {
				closer.Close()
			}
		}()
		writeMarkdownContent(githubWriter, summary, formatGitHub)
		if githubPath != "" {
			fmt.Fprintf(os.Stderr, "✅ GitHub Step Summary written\n")
		}
	}

	// 2. ALWAYS write to a regular file for persistence.
	regularFile := outputFile
	if regularFile == "" {
		regularFile = defaultSummaryFile // Default file for PR comments.
	}

	file, err := os.Create(regularFile)
	if err != nil {
		return fmt.Errorf("failed to create output file: %w", err)
	}
	defer file.Close()

	writeMarkdownContent(file, summary, formatGitHub)
	absPath, _ := filepath.Abs(regularFile)
	fmt.Fprintf(os.Stderr, "✅ Test summary written to: %s\n", absPath)

	return nil
}

// writeMarkdownContent writes the markdown content for test results.
func writeMarkdownContent(output io.Writer, summary *TestSummary, format string) {
	// Add UUID magic comment to prevent duplicate GitHub comments.
	//nolint:forbidigo // Standalone tool - direct env var access is appropriate.
	if uuid := os.Getenv("TEST_SUMMARY_UUID"); uuid != "" {
		fmt.Fprintf(output, "<!-- test-summary-uuid: %s -->\n\n", uuid)
	}

	// Add timestamp for local GitHub format runs.
	//nolint:forbidigo // Standalone tool - direct env var access is appropriate.
	if format == formatGitHub && os.Getenv("GITHUB_STEP_SUMMARY") == "" {
		fmt.Fprintf(output, "_Generated: %s_\n\n", time.Now().Format("2006-01-02 15:04:05"))
	}

	// Test Results section (h1).
	fmt.Fprintf(output, "# Test Results\n\n")

	// Get test counts.
	total := len(summary.Passed) + len(summary.Failed) + len(summary.Skipped)

	// Display test results as shields.io badges - always show all badges.
	if total == 0 {
		fmt.Fprintf(output, "[![No Tests](https://shields.io/badge/NO_TESTS-0-inactive?style=for-the-badge)](#user-content-no-tests)")
	} else {
		fmt.Fprintf(output, "[![Passed](https://shields.io/badge/PASSED-%d-success?style=for-the-badge)](#user-content-passed) ", len(summary.Passed))
		fmt.Fprintf(output, "[![Failed](https://shields.io/badge/FAILED-%d-critical?style=for-the-badge)](#user-content-failed) ", len(summary.Failed))
		fmt.Fprintf(output, "[![Skipped](https://shields.io/badge/SKIPPED-%d-inactive?style=for-the-badge)](#user-content-skipped) ", len(summary.Skipped))
	}
	fmt.Fprintf(output, "\n\n")

	// Write test sections.
	writeFailedTests(output, summary.Failed)
	writeSkippedTests(output, summary.Skipped)
	writePassedTests(output, summary.Passed)

	// Test Coverage section (h1) - moved after test results.
	if summary.CoverageData != nil {
		writeTestCoverageSection(output, summary.CoverageData)
	} else if summary.Coverage != "" {
		writeLegacyCoverageSection(output, summary.Coverage)
	}
}

// openOutput opens the appropriate output destination.
func openOutput(format, outputFile string) (io.Writer, string, error) {
	if format == formatGitHub {
		return openGitHubOutput(outputFile)
	}

	if outputFile == "" || outputFile == stdinMarker {
		return os.Stdout, stdoutPath, nil
	}

	file, err := os.Create(outputFile)
	if err != nil {
		return nil, "", fmt.Errorf("failed to create output file: %w", err)
	}

	return file, outputFile, nil
}

// openGitHubOutput handles GitHub-specific output logic.
func openGitHubOutput(outputFile string) (io.Writer, string, error) {
	//nolint:forbidigo // Standalone tool - direct env var access is appropriate.
	githubStepSummary := os.Getenv("GITHUB_STEP_SUMMARY")

	if githubStepSummary != "" {
		// Running in GitHub Actions - write to GITHUB_STEP_SUMMARY.
		file, err := os.OpenFile(githubStepSummary, os.O_APPEND|os.O_WRONLY, filePermissions)
		if err != nil {
			return nil, "", fmt.Errorf("failed to open GITHUB_STEP_SUMMARY: %w", err)
		}
		return file, githubStepSummary, nil
	}

	// Running locally - use default file.
	defaultFile := defaultSummaryFile
	if outputFile != "" {
		defaultFile = outputFile
	}
	file, err := os.Create(defaultFile)
	if err != nil {
		return nil, "", fmt.Errorf("failed to create summary file: %w", err)
	}
	// Inform the user.
	absPath, _ := filepath.Abs(defaultFile)
	fmt.Fprintf(os.Stderr, "📝 GITHUB_STEP_SUMMARY not set (running locally). Writing summary to: %s\n", absPath)
	return file, defaultFile, nil
}
