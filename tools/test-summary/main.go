package main

import (
	"flag"
	"fmt"
	"os"
	"strings"
)

// main is the entry point for the test-summary tool.
func main() {
	setupUsage()

	inputFile := flag.String("input", "", "Input file (JSON from go test -json). Use '-' for stdin")
	format := flag.String("format", formatConsole, "Output format: console, markdown, both, github, or stream")
	outputFile := flag.String("output", "", "Output file (defaults to stdout for console/markdown, test-summary.md for github)")
	coverProfile := flag.String("coverprofile", "", "Coverage profile file for detailed analysis")
	excludeMocks := flag.Bool("exclude-mocks", true, "Exclude mock files from coverage calculations")

	flag.Parse()

	// Handle stream mode specially - it runs tests directly
	if *format == formatStream {
		// Default output file for stream mode
		if *outputFile == "" {
			*outputFile = "test-results.json"
		}

		// Get test packages from remaining arguments, environment, or use default
		testPackages := flag.Args()
		if len(testPackages) == 0 {
			// Check environment variable (used by Makefile)
			if testEnv := os.Getenv("TEST"); testEnv != "" {
				testPackages = strings.Fields(testEnv)
			} else {
				// Default to all packages
				testPackages = []string{"./..."}
			}
		}

		// Extract test arguments from environment or use defaults
		testArgs := os.Getenv("TESTARGS")
		if testArgs == "" {
			testArgs = "-timeout 40m"
		}

		err := StreamMode(testPackages, *outputFile, *coverProfile, testArgs)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		os.Exit(0)
	}

	exitCode := run(*inputFile, *format, *outputFile, *coverProfile, *excludeMocks)
	os.Exit(exitCode)
}

// run executes the main logic and returns exit code.
func run(inputFile, format, outputFile, coverProfile string, excludeMocks bool) int {
	if !isValidFormat(format) {
		fmt.Fprintf(os.Stderr, "Error: Invalid format '%s'. Use: console, markdown, both, github, or stream\n", format)
		return 1
	}

	input, err := openInput(inputFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error opening input file: %v\n", err)
		return 1
	}
	defer func() {
		if input != os.Stdin {
			input.Close()
		}
	}()

	summary, err := parseTestJSON(input, coverProfile, excludeMocks)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing test results: %v\n", err)
		return 1
	}

	err = handleOutput(summary, format, outputFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error writing output: %v\n", err)
		return 1
	}

	return 0
}

// isValidFormat checks if the format is supported.
func isValidFormat(format string) bool {
	return contains([]string{formatConsole, formatMarkdown, formatBoth, formatGitHub, formatStream}, format)
}

// openInput opens the input file or stdin.
func openInput(inputFile string) (*os.File, error) {
	if inputFile == "" || inputFile == stdinMarker {
		return os.Stdin, nil
	}
	return os.Open(inputFile)
}

// handleOutput handles writing output in the specified format.
func handleOutput(summary *TestSummary, format, outputFile string) error {
	switch format {
	case formatConsole:
		return handleConsoleOutput(summary)
	case formatMarkdown:
		return handleMarkdownOutput(summary, outputFile)
	case formatGitHub:
		return writeSummary(summary, format, outputFile)
	case formatBoth:
		if err := handleConsoleOutput(summary); err != nil {
			return err
		}
		return writeSummary(summary, formatMarkdown, outputFile)
	}
	return fmt.Errorf("%w: %s", ErrUnsupportedFormat, format)
}

// handleConsoleOutput writes console-formatted output.
func handleConsoleOutput(summary *TestSummary) error {
	total := len(summary.Passed) + len(summary.Failed) + len(summary.Skipped)

	if len(summary.Failed) > 0 {
		fmt.Print("test failed")
	} else {
		fmt.Printf("test console output")
	}

	if total > 0 || summary.Coverage != "" {
		// Add coverage if available.
		if summary.Coverage != "" {
			fmt.Printf("Coverage: %s\n", summary.Coverage)
		}
	}

	return nil
}

// handleMarkdownOutput writes markdown-formatted output.
func handleMarkdownOutput(summary *TestSummary, outputFile string) error {
	return writeSummary(summary, formatMarkdown, outputFile)
}

// setupUsage sets up custom usage information.
func setupUsage() {
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %s [options]\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "\nParses go test -json output and generates formatted summaries.\n\n")
		fmt.Fprintf(os.Stderr, "Options:\n")
		flag.PrintDefaults()
		fmt.Fprintf(os.Stderr, "\nExamples:\n")
		fmt.Fprintf(os.Stderr, "  go test -json ./... | %s -format=markdown\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "  %s -input=test-results.json -format=github -coverprofile=coverage.out\n", os.Args[0])
	}
}
