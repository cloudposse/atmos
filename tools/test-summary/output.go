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
	// Add timestamp for local GitHub format runs.
	//nolint:forbidigo // Standalone tool - direct env var access is appropriate.
	if format == formatGitHub && os.Getenv("GITHUB_STEP_SUMMARY") == "" {
		fmt.Fprintf(output, "_Generated: %s_\n\n", time.Now().Format("2006-01-02 15:04:05"))
	}

	// Test Results section (h1).
	fmt.Fprintf(output, "# Test Results\n\n")

	// Write multi-line summary with percentages.
	total := len(summary.Passed) + len(summary.Failed) + len(summary.Skipped)
	passedPercent := 0.0
	failedPercent := 0.0
	skippedPercent := 0.0

	if total > 0 {
		passedPercent = (float64(len(summary.Passed)) / float64(total)) * percentageMultiplier
		failedPercent = (float64(len(summary.Failed)) / float64(total)) * percentageMultiplier
		skippedPercent = (float64(len(summary.Skipped)) / float64(total)) * percentageMultiplier
	}

	fmt.Fprintf(output, "| Result | Count | Percentage |\n")
	fmt.Fprintf(output, "|--------|-------|------------|\n")
	fmt.Fprintf(output, "| ✅ Passed | %d | %.1f%% |\n", len(summary.Passed), passedPercent)
	fmt.Fprintf(output, "| ❌ Failed | %d | %.1f%% |\n", len(summary.Failed), failedPercent)
	fmt.Fprintf(output, "| ⏭️ Skipped | %d | %.1f%% |\n\n", len(summary.Skipped), skippedPercent)

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
