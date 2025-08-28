// Package main provides a tool to parse Go test JSON output and generate summaries.
package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	// Format constants.
	formatConsole  = "console"
	formatMarkdown = "markdown"
	formatGitHub   = "github"
	formatBoth     = "both"

	// File handling constants.
	stdinMarker        = "-"
	stdoutPath         = "stdout"
	defaultSummaryFile = "test-summary.md"
	filePermissions    = 0o644

	// Coverage threshold constants.
	coverageHighThreshold = 80.0
	coverageMedThreshold  = 50.0
	base10BitSize         = 64
)

// TestEvent represents a single event from go test -json output.
type TestEvent struct {
	Time    time.Time `json:"Time"`
	Action  string    `json:"Action"`
	Package string    `json:"Package"`
	Test    string    `json:"Test"`
	Output  string    `json:"Output"`
	Elapsed float64   `json:"Elapsed"`
}

// TestResult represents the final result of a single test.
type TestResult struct {
	Package  string
	Test     string
	Status   string
	Duration float64
}

// TestSummary holds all test results and metadata.
type TestSummary struct {
	Failed   []TestResult
	Skipped  []TestResult
	Passed   []TestResult
	Coverage string
	ExitCode int
}

func main() {
	inputFile := flag.String("input", stdinMarker, "JSON test results file (- for stdin)")
	format := flag.String("format", formatBoth, "Output format: console, markdown, both, github")
	outputFile := flag.String("output", "", "Output file (default: stdout for markdown, test-summary.md for github)")

	setupUsage()
	flag.Parse()

	exitCode := run(*inputFile, *format, *outputFile)
	os.Exit(exitCode)
}

func run(inputFile, format, outputFile string) int {
	// Open input.
	input := os.Stdin
	if inputFile != stdinMarker {
		file, err := os.Open(inputFile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error opening input file: %v\n", err)
			return 1
		}
		defer file.Close()
		input = file
	}

	// Parse and process.
	summary, consoleOutput := parseTestJSON(input)

	// Output based on format.
	exitCode := summary.ExitCode
	switch format {
	case formatConsole:
		fmt.Print(consoleOutput)
	case formatMarkdown:
		output := outputFile
		if output == "" {
			output = stdinMarker
		}
		if err := writeSummary(summary, formatMarkdown, output); err != nil {
			fmt.Fprintf(os.Stderr, "Error writing summary: %v\n", err)
			exitCode = 1
		}
	case formatGitHub:
		if err := writeSummary(summary, formatGitHub, outputFile); err != nil {
			fmt.Fprintf(os.Stderr, "Error writing summary: %v\n", err)
			exitCode = 1
		}
	case formatBoth:
		fmt.Print(consoleOutput)
		output := outputFile
		if output == "" {
			output = stdinMarker
		}
		if err := writeSummary(summary, formatMarkdown, output); err != nil {
			fmt.Fprintf(os.Stderr, "Error writing summary: %v\n", err)
			exitCode = 1
		}
	default:
		fmt.Fprintf(os.Stderr, "Error: Invalid format '%s'. Use: %s, %s, %s, or %s\n",
			format, formatConsole, formatMarkdown, formatBoth, formatGitHub)
		exitCode = 1
	}

	return exitCode
}

func setupUsage() {
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: test-summary [options]\n\n")
		fmt.Fprintf(os.Stderr, "Parse Go test JSON output and generate summaries.\n\n")
		fmt.Fprintf(os.Stderr, "Options:\n")
		flag.PrintDefaults()
		fmt.Fprintf(os.Stderr, "\nExamples:\n")
		fmt.Fprintf(os.Stderr, "  # Pipe test output through for console display\n")
		fmt.Fprintf(os.Stderr, "  go test -json ./... | test-summary -format=%s\n\n", formatConsole)
		fmt.Fprintf(os.Stderr, "  # Generate markdown summary from file\n")
		fmt.Fprintf(os.Stderr, "  test-summary -input=test-results.json -format=%s\n\n", formatMarkdown)
		fmt.Fprintf(os.Stderr, "  # Generate GitHub Actions summary\n")
		fmt.Fprintf(os.Stderr, "  test-summary -input=test-results.json -format=%s\n", formatGitHub)
	}
}

func parseTestJSON(input io.Reader) (*TestSummary, string) {
	scanner := bufio.NewScanner(input)
	var console strings.Builder
	results := make(map[string]*TestResult)
	summary := &TestSummary{}
	coverageRe := regexp.MustCompile(`coverage:\s+([\d.]+)%\s+of\s+statements`)

	for scanner.Scan() {
		line := scanner.Text()
		processLine(line, &console, results, summary, coverageRe)
	}

	categorizeResults(results, summary)
	sortResults(summary)

	return summary, console.String()
}

func processLine(line string, console *strings.Builder, results map[string]*TestResult, summary *TestSummary, coverageRe *regexp.Regexp) {
	var event TestEvent
	if err := json.Unmarshal([]byte(line), &event); err != nil {
		// Not JSON, pass through.
		console.WriteString(line + "\n")
		return
	}

	// Output for console.
	if event.Output != "" {
		console.WriteString(event.Output)

		// Check for coverage in output.
		if matches := coverageRe.FindStringSubmatch(event.Output); len(matches) > 1 {
			summary.Coverage = matches[1] + "%"
		}
	}

	// Track test results.
	if event.Test != "" && (event.Action == "pass" || event.Action == "fail" || event.Action == "skip") {
		recordTestResult(&event, results, summary)
	}
}

func recordTestResult(event *TestEvent, results map[string]*TestResult, summary *TestSummary) {
	result := TestResult{
		Package:  event.Package,
		Test:     event.Test,
		Status:   event.Action,
		Duration: event.Elapsed,
	}

	key := fmt.Sprintf("%s/%s", event.Package, event.Test)
	results[key] = &result

	if event.Action == "fail" {
		summary.ExitCode = 1
	}
}

func categorizeResults(results map[string]*TestResult, summary *TestSummary) {
	for _, result := range results {
		switch result.Status {
		case "fail":
			summary.Failed = append(summary.Failed, *result)
		case "skip":
			summary.Skipped = append(summary.Skipped, *result)
		case "pass":
			summary.Passed = append(summary.Passed, *result)
		}
	}
}

func sortResults(summary *TestSummary) {
	sortTests := func(tests []TestResult) {
		sort.Slice(tests, func(i, j int) bool {
			if tests[i].Package == tests[j].Package {
				return tests[i].Test < tests[j].Test
			}
			return tests[i].Package < tests[j].Package
		})
	}
	sortTests(summary.Failed)
	sortTests(summary.Skipped)
	sortTests(summary.Passed)
}

func writeSummary(summary *TestSummary, format, outputFile string) error {
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

func openOutput(format, outputFile string) (io.Writer, string, error) {
	switch format {
	case formatGitHub:
		return openGitHubOutput(outputFile)
	case formatMarkdown:
		if outputFile == stdinMarker || outputFile == "" {
			return os.Stdout, stdoutPath, nil
		}
		file, err := os.Create(outputFile)
		if err != nil {
			return nil, "", fmt.Errorf("failed to create output file: %w", err)
		}
		return file, outputFile, nil
	default:
		return os.Stdout, stdoutPath, nil
	}
}

func openGitHubOutput(outputFile string) (io.Writer, string, error) {
	//nolint:forbidigo // This is a standalone tool, not part of Atmos core - os.Getenv is appropriate here.
	githubSummary := os.Getenv("GITHUB_STEP_SUMMARY")
	if githubSummary != "" {
		// Running in GitHub Actions.
		file, err := os.OpenFile(githubSummary, os.O_APPEND|os.O_CREATE|os.O_WRONLY, filePermissions)
		if err != nil {
			return nil, "", fmt.Errorf("failed to open GITHUB_STEP_SUMMARY file: %w", err)
		}
		return file, githubSummary, nil
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

func writeMarkdownContent(output io.Writer, summary *TestSummary, format string) {
	// Add timestamp for local GitHub format runs.
	//nolint:forbidigo // Standalone tool - direct env var access is appropriate.
	if format == formatGitHub && os.Getenv("GITHUB_STEP_SUMMARY") == "" {
		fmt.Fprintf(output, "_Generated: %s_\n\n", time.Now().Format("2006-01-02 15:04:05"))
	}

	fmt.Fprintf(output, "## Test Results\n\n")

	// Add coverage if available.
	if summary.Coverage != "" {
		writeCoverageSection(output, summary.Coverage)
	}

	// Write summary line.
	total := len(summary.Passed) + len(summary.Failed) + len(summary.Skipped)
	fmt.Fprintf(output, "**Summary:** %d tests • ✅ %d passed • ❌ %d failed • ⏭️ %d skipped\n\n",
		total, len(summary.Passed), len(summary.Failed), len(summary.Skipped))

	// Write test sections.
	writeFailedTests(output, summary.Failed)
	writeSkippedTests(output, summary.Skipped)
	writePassedTests(output, summary.Passed)
}

func writeCoverageSection(output io.Writer, coverage string) {
	coverageFloat, _ := strconv.ParseFloat(strings.TrimSuffix(coverage, "%"), base10BitSize)
	emoji := "🔴" // red for < 50%.
	if coverageFloat >= coverageHighThreshold {
		emoji = "🟢" // green for >= 80%.
	} else if coverageFloat >= coverageMedThreshold {
		emoji = "🟡" // yellow for 50-79%.
	}
	fmt.Fprintf(output, "**Coverage:** %s %s of statements\n\n", emoji, coverage)
}

func writeFailedTests(output io.Writer, failed []TestResult) {
	if len(failed) == 0 {
		return
	}

	fmt.Fprintf(output, "### ❌ Failed Tests (%d)\n\n", len(failed))
	fmt.Fprintf(output, "| Test | Package | Duration |\n")
	fmt.Fprintf(output, "|------|---------|----------|\n")

	for _, test := range failed {
		pkg := shortPackage(test.Package)
		fmt.Fprintf(output, "| `%s` | %s | %.2fs |\n", test.Test, pkg, test.Duration)
	}

	fmt.Fprintf(output, "\n**Run locally to reproduce:**\n")
	fmt.Fprintf(output, "```bash\n")
	for _, test := range failed {
		fmt.Fprintf(output, "go test %s -run ^%s$ -v\n", test.Package, test.Test)
	}
	fmt.Fprintf(output, "```\n\n")
}

func writeSkippedTests(output io.Writer, skipped []TestResult) {
	if len(skipped) == 0 {
		return
	}

	fmt.Fprintf(output, "### ⏭️ Skipped Tests (%d)\n\n", len(skipped))
	fmt.Fprintf(output, "| Test | Package |\n")
	fmt.Fprintf(output, "|------|---------|)\n")

	for _, test := range skipped {
		pkg := shortPackage(test.Package)
		fmt.Fprintf(output, "| `%s` | %s |\n", test.Test, pkg)
	}
	fmt.Fprintf(output, "\n")
}

func writePassedTests(output io.Writer, passed []TestResult) {
	if len(passed) == 0 {
		return
	}

	fmt.Fprintf(output, "### ✅ Passed Tests (%d)\n\n", len(passed))
	fmt.Fprintf(output, "<details>\n")
	fmt.Fprintf(output, "<summary>Click to show all passing tests</summary>\n\n")
	fmt.Fprintf(output, "| Test | Package | Duration |\n")
	fmt.Fprintf(output, "|------|---------|----------|\n")

	for _, test := range passed {
		pkg := shortPackage(test.Package)
		fmt.Fprintf(output, "| `%s` | %s | %.2fs |\n", test.Test, pkg, test.Duration)
	}

	fmt.Fprintf(output, "\n</details>\n\n")
}

func shortPackage(pkg string) string {
	// Shorten package name for readability.
	// github.com/cloudposse/atmos/cmd -> cmd
	parts := strings.Split(pkg, "/")
	if len(parts) > 0 {
		return parts[len(parts)-1]
	}
	return pkg
}
