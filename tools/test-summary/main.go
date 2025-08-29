// Package main provides a tool to parse Go test JSON output and generate summaries.
package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
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
	coverageMedThreshold  = 40.0
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

// CoverageFunction represents function-level coverage information.
type CoverageFunction struct {
	File     string
	Function string
	Coverage float64
}

// CoverageData holds coverage analysis results.
type CoverageData struct {
	StatementCoverage string
	FunctionCoverage  []CoverageFunction
	FilteredFiles     []string // Files excluded from coverage
}

// TestSummary holds all test results and metadata.
type TestSummary struct {
	Failed       []TestResult
	Skipped      []TestResult
	Passed       []TestResult
	Coverage     string // Legacy statement coverage
	CoverageData *CoverageData
	ExitCode     int
}

func main() {
	inputFile := flag.String("input", stdinMarker, "JSON test results file (- for stdin)")
	format := flag.String("format", formatBoth, "Output format: console, markdown, both, github")
	outputFile := flag.String("output", "", "Output file (default: stdout for markdown, test-summary.md for github)")
	coverProfile := flag.String("coverprofile", "", "Coverage profile file for detailed analysis")
	excludeMocks := flag.Bool("exclude-mocks", true, "Exclude mock files from coverage calculations")
	setupUsage()
	flag.Parse()
	exitCode := run(*inputFile, *format, *outputFile, *coverProfile, *excludeMocks)
	os.Exit(exitCode)
}

func run(inputFile, format, outputFile, coverProfile string, excludeMocks bool) int {
	// Open input.
	input, err := openInput(inputFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error opening input file: %v\n", err)
		return 1
	}
	if input != os.Stdin {
		if file, ok := input.(io.Closer); ok {
			defer file.Close()
		}
	}
	// Parse and process.
	summary, consoleOutput := parseTestJSON(input)

	// Process coverage profile if provided.
	if coverProfile != "" {
		coverageData, err := parseCoverageProfile(coverProfile, excludeMocks)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: Failed to parse coverage profile: %v\n", err)
		} else {
			summary.CoverageData = coverageData
		}
	}

	// Handle output based on format.
	return handleOutput(format, outputFile, summary, consoleOutput)
}

func parseCoverageProfile(profileFile string, excludeMocks bool) (*CoverageData, error) {
	// First parse the raw coverage profile to understand file coverage
	statementCoverage, filteredFiles, err := parseStatementCoverage(profileFile, excludeMocks)
	if err != nil {
		return nil, err
	}

	// Then get function-level coverage using go tool cover -func
	functions, err := getFunctionCoverage(profileFile, excludeMocks)
	if err != nil {
		// Log warning but continue with statement coverage only
		fmt.Fprintf(os.Stderr, "Warning: Failed to get function coverage: %v\n", err)
		functions = []CoverageFunction{}
	}

	return &CoverageData{
		StatementCoverage: statementCoverage,
		FunctionCoverage:  functions,
		FilteredFiles:     filteredFiles,
	}, nil
}

func parseStatementCoverage(profileFile string, excludeMocks bool) (string, []string, error) {
	file, err := os.Open(profileFile)
	if err != nil {
		return "", nil, fmt.Errorf("failed to open coverage profile: %w", err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	var filteredFiles []string
	mockRegex := regexp.MustCompile(`mock_.*\.go|.*mock\.go|.*/mock/.*\.go`)

	// Skip the first line (mode: set, atomic, etc.)
	if scanner.Scan() {
		// First line is mode declaration
	}

	totalStatements := 0
	coveredStatements := 0

	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}

		// Parse coverage line: file:startLine.startCol,endLine.endCol numStmts count
		parts := strings.Fields(line)
		if len(parts) < 3 {
			continue
		}

		filePath := strings.Split(parts[0], ":")[0]

		// Filter out mock files if requested
		if excludeMocks && mockRegex.MatchString(filePath) {
			if !contains(filteredFiles, filePath) {
				filteredFiles = append(filteredFiles, filePath)
			}
			continue
		}

		numStmts, err := strconv.Atoi(parts[1])
		if err != nil {
			continue
		}

		count, err := strconv.Atoi(parts[2])
		if err != nil {
			continue
		}

		totalStatements += numStmts
		if count > 0 {
			coveredStatements += numStmts
		}
	}

	if err := scanner.Err(); err != nil {
		return "", nil, fmt.Errorf("error reading coverage profile: %w", err)
	}

	// Calculate overall statement coverage
	statementCoverage := "0.0%"
	if totalStatements > 0 {
		coverage := float64(coveredStatements) / float64(totalStatements) * 100
		statementCoverage = fmt.Sprintf("%.1f%%", coverage)
	}

	return statementCoverage, filteredFiles, nil
}

func getFunctionCoverage(profileFile string, excludeMocks bool) ([]CoverageFunction, error) {
	// Use go tool cover -func to get function-level coverage
	cmd := exec.Command("go", "tool", "cover", "-func="+profileFile)
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to run go tool cover -func: %w", err)
	}

	var functions []CoverageFunction
	mockRegex := regexp.MustCompile(`mock_.*\.go|.*mock\.go|.*/mock/.*\.go`)
	funcRegex := regexp.MustCompile(`^(.+?):(\d+):\s+(.+?)\s+(\d+\.\d+)%`)

	scanner := bufio.NewScanner(strings.NewReader(string(output)))
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "total:") {
			// Skip the total line
			continue
		}

		matches := funcRegex.FindStringSubmatch(line)
		if len(matches) < 5 {
			continue
		}

		file := matches[1]
		functionName := matches[3]
		coverageStr := matches[4]

		// Filter out mock files if requested
		if excludeMocks && mockRegex.MatchString(file) {
			continue
		}

		coverage, err := strconv.ParseFloat(coverageStr, 64)
		if err != nil {
			continue
		}

		functions = append(functions, CoverageFunction{
			File:     file,
			Function: functionName,
			Coverage: coverage,
		})
	}

	// Sort functions by file then function name
	sort.Slice(functions, func(i, j int) bool {
		if functions[i].File == functions[j].File {
			return functions[i].Function < functions[j].Function
		}
		return functions[i].File < functions[j].File
	})

	return functions, nil
}

func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

func openInput(inputFile string) (io.Reader, error) {
	if inputFile == stdinMarker {
		return os.Stdin, nil
	}
	return os.Open(inputFile)
}

func handleOutput(format, outputFile string, summary *TestSummary, consoleOutput string) int {
	exitCode := summary.ExitCode
	switch format {
	case formatConsole:
		fmt.Print(consoleOutput)
	case formatMarkdown:
		exitCode = handleMarkdownOutput(outputFile, summary, exitCode)
	case formatGitHub:
		if err := writeSummary(summary, formatGitHub, outputFile); err != nil {
			fmt.Fprintf(os.Stderr, "Error writing summary: %v\n", err)
			return 1
		}
	case formatBoth:
		fmt.Print(consoleOutput)
		exitCode = handleMarkdownOutput(outputFile, summary, exitCode)
	default:
		fmt.Fprintf(os.Stderr, "Error: Invalid format '%s'. Use: %s, %s, %s, or %s\n",
			format, formatConsole, formatMarkdown, formatBoth, formatGitHub)
		return 1
	}
	return exitCode
}

func handleMarkdownOutput(outputFile string, summary *TestSummary, exitCode int) int {
	output := outputFile
	if output == "" {
		output = stdinMarker
	}
	if err := writeSummary(summary, formatMarkdown, output); err != nil {
		fmt.Fprintf(os.Stderr, "Error writing summary: %v\n", err)
		return 1
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
		fmt.Fprintf(os.Stderr, "  test-summary -input=test-results.json -format=%s\n\n", formatGitHub)
		fmt.Fprintf(os.Stderr, "  # Generate summary with coverage profile and mock filtering\n")
		fmt.Fprintf(os.Stderr, "  test-summary -input=test-results.json -coverprofile=coverage.out -format=%s\n", formatMarkdown)
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
	if format == formatGitHub {
		// For GitHub format, write to both GITHUB_STEP_SUMMARY and a persistent file

		// 1. Write to GITHUB_STEP_SUMMARY (if available)
		githubWriter, githubPath, err := openGitHubOutput("")
		if err == nil {
			defer func() {
				if closer, ok := githubWriter.(io.Closer); ok {
					closer.Close()
				}
			}()
			writeMarkdownContent(githubWriter, summary, format)
			if githubPath != "" {
				fmt.Fprintf(os.Stderr, "✅ GitHub Step Summary written\n")
			}
		}

		// 2. ALWAYS write to a regular file for persistence
		regularFile := outputFile
		if regularFile == "" {
			regularFile = "test-summary.md" // Default file for PR comments
		}

		file, err := os.Create(regularFile)
		if err != nil {
			return fmt.Errorf("failed to create output file: %w", err)
		}
		defer file.Close()

		writeMarkdownContent(file, summary, format)
		absPath, _ := filepath.Abs(regularFile)
		fmt.Fprintf(os.Stderr, "✅ Test summary written to: %s\n", absPath)

		return nil
	}

	// For other formats, use the original logic
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
	if summary.CoverageData != nil {
		writeCoverageSectionWithDetails(output, summary.CoverageData)
	} else if summary.Coverage != "" {
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

func writeCoverageSectionWithDetails(output io.Writer, coverageData *CoverageData) {
	coverageFloat, _ := strconv.ParseFloat(strings.TrimSuffix(coverageData.StatementCoverage, "%"), base10BitSize)
	emoji := "🔴" // red for < 40%.
	if coverageFloat >= coverageHighThreshold {
		emoji = "🟢" // green for >= 80%.
	} else if coverageFloat >= coverageMedThreshold {
		emoji = "🟡" // yellow for 40-79%.
	}
	fmt.Fprintf(output, "**Coverage:** %s %s of statements", emoji, coverageData.StatementCoverage)

	// Add information about filtered files
	if len(coverageData.FilteredFiles) > 0 {
		fmt.Fprintf(output, " (excluded %d mock files)", len(coverageData.FilteredFiles))
	}
	fmt.Fprintf(output, "\n\n")

	// Show function-level coverage if available
	if len(coverageData.FunctionCoverage) > 0 {
		writeFunctionCoverageDetails(output, coverageData.FunctionCoverage)
	}

	// Show filtered files if any
	if len(coverageData.FilteredFiles) > 0 {
		fmt.Fprintf(output, "<details>\n")
		fmt.Fprintf(output, "<summary>Excluded mock files (%d)</summary>\n\n", len(coverageData.FilteredFiles))
		for _, file := range coverageData.FilteredFiles {
			fmt.Fprintf(output, "- `%s`\n", file)
		}
		fmt.Fprintf(output, "\n</details>\n\n")
	}
}

func writeFunctionCoverageDetails(output io.Writer, functions []CoverageFunction) {
	if len(functions) == 0 {
		return
	}

	// Calculate function coverage statistics
	totalFunctions := len(functions)
	coveredFunctions := 0
	uncoveredFunctions := []CoverageFunction{}

	for _, fn := range functions {
		if fn.Coverage > 0 {
			coveredFunctions++
		} else {
			uncoveredFunctions = append(uncoveredFunctions, fn)
		}
	}

	functionCoveragePercent := float64(coveredFunctions) / float64(totalFunctions) * 100

	fmt.Fprintf(output, "**Function Coverage:** %.1f%% (%d/%d functions covered)\n\n",
		functionCoveragePercent, coveredFunctions, totalFunctions)

	// Show uncovered functions if any
	if len(uncoveredFunctions) > 0 {
		fmt.Fprintf(output, "<details>\n")
		fmt.Fprintf(output, "<summary>❌ Uncovered Functions (%d)</summary>\n\n", len(uncoveredFunctions))
		fmt.Fprintf(output, "| Function | File |\n")
		fmt.Fprintf(output, "|----------|------|\n")
		for _, fn := range uncoveredFunctions {
			file := shortPackage(fn.File)
			fmt.Fprintf(output, "| `%s` | %s |\n", fn.Function, file)
		}
		fmt.Fprintf(output, "\n</details>\n\n")
	}
}

func writeCoverageSection(output io.Writer, coverage string) {
	coverageFloat, _ := strconv.ParseFloat(strings.TrimSuffix(coverage, "%"), base10BitSize)
	emoji := "🔴" // red for < 40%.
	if coverageFloat >= coverageHighThreshold {
		emoji = "🟢" // green for >= 80%.
	} else if coverageFloat >= coverageMedThreshold {
		emoji = "🟡" // yellow for 40-79%.
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
	fmt.Fprintf(output, "|------|--------|\n")
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
