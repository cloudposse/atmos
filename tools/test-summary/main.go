package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// main is the entry point for the test-summary tool.
func main() {
	setupUsage()

	inputFile := flag.String("input", "", "Input file (JSON from go test -json). Use '-' for stdin")
	format := flag.String("format", formatConsole, "Output format: console, markdown, both, github, or stream")
	outputFile := flag.String("output", "", "Output file (defaults to stdout for console/markdown, test-summary.md for github)")
	coverProfile := flag.String("coverprofile", "", "Coverage profile file for detailed analysis")
	excludeMocks := flag.Bool("exclude-mocks", true, "Exclude mock files from coverage calculations")
	packages := flag.String("packages", "", "Space-separated list of packages to test (for stream mode)")
	testArgs := flag.String("testargs", "", "Additional arguments to pass to go test (for stream mode)")
	show := flag.String("show", "all", "Filter displayed tests: all, failed, passed, skipped (stream mode only)")

	flag.Parse()

	// Auto-detect stream mode when packages are specified but no explicit format
	if *packages != "" && *format == formatConsole {
		*format = formatStream
	}

	// Handle stream mode specially - it runs tests directly
	if *format == formatStream {
		// Default output file for stream mode
		if *outputFile == "" {
			*outputFile = "test-results.json"
		}

		// Get test packages from flags, remaining arguments, or use default
		var testPackages []string
		if *packages != "" {
			testPackages = strings.Fields(*packages)
		} else if len(flag.Args()) > 0 {
			testPackages = flag.Args()
		} else {
			// Default to all packages
			testPackages = []string{"./..."}
		}

		// Use test arguments from flags or defaults
		testArgsStr := *testArgs
		if testArgsStr == "" {
			testArgsStr = "-timeout 40m"
		}

		// Check if we have a TTY for interactive mode
		if isTTY() {
			// Create and run the Bubble Tea program
			model := newTestModel(testPackages, testArgsStr, *outputFile, *coverProfile, *show)
			p := tea.NewProgram(model)
			
			finalModel, err := p.Run()
			if err != nil {
				fmt.Fprintf(os.Stderr, "Failed to run test UI: %v\n", err)
				os.Exit(1)
			}
			
			// Extract exit code from final model
			if m, ok := finalModel.(testModel); ok {
				exitCode := m.GetExitCode()
				os.Exit(exitCode)
			}
			
			os.Exit(0)
		} else {
			// Fallback to simple streaming for CI/non-TTY environments
			exitCode := runSimpleStream(testPackages, testArgsStr, *outputFile, *coverProfile, *show)
			os.Exit(exitCode)
		}
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
		fmt.Fprintf(os.Stderr, "  # Stream mode (runs tests directly):\n")
		fmt.Fprintf(os.Stderr, "  %s -format=stream\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "  %s -format=stream -show=failed\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "  %s -format=stream -packages=\"./...\" -testargs=\"-timeout 60s\"\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "\n  # Process existing results:\n")
		fmt.Fprintf(os.Stderr, "  go test -json ./... | %s -format=markdown\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "  %s -input=test-results.json -format=github -coverprofile=coverage.out\n", os.Args[0])
	}
}

// isTTY checks if we're running in a terminal and Bubble Tea can actually use it
func isTTY() bool {
	// Provide an environment override
	if os.Getenv("FORCE_NO_TTY") != "" {
		return false
	}
	
	// Debug: Force TTY mode for testing (but only if TTY is actually usable)
	if os.Getenv("FORCE_TTY") != "" {
		// Still check if we can actually open /dev/tty
		if tty, err := os.OpenFile("/dev/tty", os.O_RDWR, 0); err == nil {
			tty.Close()
			return true
		}
		// If we can't open /dev/tty, fall back to normal detection
	}
	
	// Check if both stdin and stdout are terminals
	stat, err := os.Stdout.Stat()
	if err != nil {
		return false
	}
	isStdoutTTY := (stat.Mode() & os.ModeCharDevice) == os.ModeCharDevice
	
	stat, err = os.Stdin.Stat()
	if err != nil {
		return false
	}
	isStdinTTY := (stat.Mode() & os.ModeCharDevice) == os.ModeCharDevice
	
	// Most importantly, check if we can actually open /dev/tty
	// This is what Bubble Tea will try to do
	if tty, err := os.OpenFile("/dev/tty", os.O_RDWR, 0); err != nil {
		return false
	} else {
		tty.Close()
	}
	
	return isStdoutTTY && isStdinTTY
}

// runSimpleStream runs tests with simple non-interactive streaming output
func runSimpleStream(testPackages []string, testArgs, outputFile, coverProfile, showFilter string) int {
	// Build go test command args
	args := []string{"test", "-json"}

	// Add coverage if requested
	if coverProfile != "" {
		args = append(args, fmt.Sprintf("-coverprofile=%s", coverProfile))
		args = append(args, "-coverpkg=./...")
	}

	// Add verbose flag
	args = append(args, "-v")

	// Add timeout and other test arguments
	if testArgs != "" {
		extraArgs := strings.Fields(testArgs)
		args = append(args, extraArgs...)
	}

	// Add packages to test
	args = append(args, testPackages...)

	// Create command
	cmd := exec.Command("go", args...)
	cmd.Stderr = os.Stderr
	// Set working directory - if we're in tools/test-summary, go up two levels
	if wd, err := os.Getwd(); err == nil && strings.HasSuffix(wd, "tools/test-summary") {
		cmd.Dir = "../.."
	}

	// Get stdout pipe
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to get stdout pipe: %v\n", err)
		return 1
	}

	// Start the command
	if err := cmd.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to start go test: %v\n", err)
		return 1
	}

	// Create JSON output file
	jsonFile, err := os.Create(outputFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create output file: %v\n", err)
		return 1
	}
	defer jsonFile.Close()

	// Process output
	scanner := bufio.NewScanner(stdout)
	passCount := 0
	failCount := 0
	skipCount := 0
	testBuffers := make(map[string][]string)

	for scanner.Scan() {
		line := scanner.Bytes()

		// Write to JSON file
		jsonFile.Write(line)
		jsonFile.Write([]byte("\n"))

		// Parse and process event
		var event TestEvent
		if err := json.Unmarshal(line, &event); err != nil {
			continue // Skip non-JSON lines
		}

		// Skip package-level events
		if event.Test == "" {
			continue
		}

		switch event.Action {
		case "run":
			// Initialize buffer for this test
			testBuffers[event.Test] = []string{}

		case "output":
			// Buffer the output for potential error display
			if testBuffers[event.Test] != nil {
				testBuffers[event.Test] = append(testBuffers[event.Test], event.Output)
			}

		case "pass":
			passCount++
			if shouldShowTestFallback(showFilter, "pass") {
				fmt.Fprintf(os.Stderr, "%s %s (%.2fs)\n",
					passStyle.Render(checkPass),
					testNameStyle.Render(event.Test),
					event.Elapsed)
			}
			// Clean up buffer
			delete(testBuffers, event.Test)

		case "fail":
			failCount++
			if shouldShowTestFallback(showFilter, "fail") {
				output := fmt.Sprintf("%s %s (%.2fs)",
					failStyle.Render(checkFail),
					testNameStyle.Render(event.Test),
					event.Elapsed)

				// Add error details if present
				if bufferedOutput := testBuffers[event.Test]; len(bufferedOutput) > 0 {
					output += "\n\n"
					for _, line := range bufferedOutput {
						if shouldShowErrorLine(line) {
							output += "    " + line
						}
					}
					output += "\n"
				}
				
				fmt.Fprintf(os.Stderr, "%s\n", output)
			}
			// Clean up buffer
			delete(testBuffers, event.Test)

		case "skip":
			skipCount++
			if shouldShowTestFallback(showFilter, "skip") {
				fmt.Fprintf(os.Stderr, "%s %s\n",
					skipStyle.Render(checkSkip),
					testNameStyle.Render(event.Test))
			}
			// Clean up buffer
			delete(testBuffers, event.Test)
		}
	}

	// Wait for command to complete
	testErr := cmd.Wait()

	// Generate the final summary output
	summaryOutput := generateFinalSummaryForFallback(passCount, failCount, skipCount)
	fmt.Fprint(os.Stderr, summaryOutput)

	// Return appropriate exit code
	if testErr != nil {
		if exitErr, ok := testErr.(*exec.ExitError); ok {
			return exitErr.ExitCode()
		}
		return 1
	}

	if failCount > 0 {
		return 1
	}

	return 0
}

// generateFinalSummaryForFallback creates the formatted final summary output for fallback mode
func generateFinalSummaryForFallback(passCount, failCount, skipCount int) string {
	// Check GitHub step summary environment
	githubSummary := os.Getenv("GITHUB_STEP_SUMMARY")
	var summaryStatus string
	var summaryPath string
	
	if githubSummary == "" {
		summaryStatus = "- GITHUB_STEP_SUMMARY not set (skipped)."
	} else {
		summaryStatus = fmt.Sprintf("Output GitHub step summary to %s", githubSummary)
	}
	
	// Check for markdown summary file
	if _, err := os.Stat("test-summary.md"); err == nil {
		summaryPath = fmt.Sprintf("%s Output markdown summary to test-summary.md", passStyle.Render(checkPass))
	}

	// Calculate total tests
	totalTests := passCount + failCount + skipCount
	
	// Build the summary box
	border := strings.Repeat("─", 40)
	
	var output strings.Builder
	output.WriteString("\n")
	output.WriteString(summaryStatus)
	output.WriteString("\n")
	if summaryPath != "" {
		output.WriteString(summaryPath)
		output.WriteString("\n")
	}
	output.WriteString("\n")
	output.WriteString(border)
	output.WriteString("\n")
	output.WriteString("Test Summary:\n")
	output.WriteString(fmt.Sprintf("  %s Passed:  %d\n", passStyle.Render(checkPass), passCount))
	output.WriteString(fmt.Sprintf("  %s Failed:  %d\n", failStyle.Render(checkFail), failCount))
	output.WriteString(fmt.Sprintf("  %s Skipped: %d\n", skipStyle.Render(checkSkip), skipCount))
	output.WriteString(fmt.Sprintf("  Total:    %d tests\n", totalTests))
	output.WriteString(border)
	output.WriteString("\n")
	
	return output.String()
}

// shouldShowTestFallback checks if a test should be displayed based on the show filter (fallback mode)
func shouldShowTestFallback(showFilter, status string) bool {
	switch showFilter {
	case "all":
		return true
	case "failed":
		return status == "fail"
	case "passed":
		return status == "pass"
	case "skipped":
		return status == "skip"
	default:
		return true // Default to showing all
	}
}
