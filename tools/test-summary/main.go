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
	// Test display limits.
	maxTestsInChangedPackages = 200
	maxSlowestTests           = 20
	maxTotalTestsShown        = 250
	minTestsForSmartDisplay   = 100
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

	// Test Results section (h1).
	fmt.Fprintf(output, "# Test Results\n\n")

	// Write multi-line summary with percentages.
	total := len(summary.Passed) + len(summary.Failed) + len(summary.Skipped)
	passedPercent := 0.0
	failedPercent := 0.0
	skippedPercent := 0.0

	if total > 0 {
		passedPercent = (float64(len(summary.Passed)) / float64(total)) * 100
		failedPercent = (float64(len(summary.Failed)) / float64(total)) * 100
		skippedPercent = (float64(len(summary.Skipped)) / float64(total)) * 100
	}

	fmt.Fprintf(output, "Summary: %d tests\n", total)
	fmt.Fprintf(output, "✅ %d passed (%.1f%%)\n", len(summary.Passed), passedPercent)
	fmt.Fprintf(output, "❌ %d failed (%.1f%%)\n", len(summary.Failed), failedPercent)
	fmt.Fprintf(output, "⏭️ %d skipped (%.1f%%)\n\n", len(summary.Skipped), skippedPercent)

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

func writeTestCoverageSection(output io.Writer, coverageData *CoverageData) {
	fmt.Fprintf(output, "# Test Coverage\n\n")

	// Build statement coverage details.
	coverageFloat, _ := strconv.ParseFloat(strings.TrimSuffix(coverageData.StatementCoverage, "%"), base10BitSize)
	emoji := "🔴" // red for < 40%.
	if coverageFloat >= coverageHighThreshold {
		emoji = "🟢" // green for >= 80%.
	} else if coverageFloat >= coverageMedThreshold {
		emoji = "🟡" // yellow for 40-79%.
	}

	statementDetails := emoji
	if len(coverageData.FilteredFiles) > 0 {
		statementDetails += fmt.Sprintf(" (excluded %d mock files)", len(coverageData.FilteredFiles))
	}

	// Calculate function coverage statistics.
	totalFunctions := len(coverageData.FunctionCoverage)
	coveredFunctions := 0
	for _, fn := range coverageData.FunctionCoverage {
		if fn.Coverage > 0 {
			coveredFunctions++
		}
	}
	functionCoveragePercent := 0.0
	if totalFunctions > 0 {
		functionCoveragePercent = (float64(coveredFunctions) / float64(totalFunctions)) * 100
	}
	functionDetails := fmt.Sprintf("%d/%d functions covered", coveredFunctions, totalFunctions)

	// Write coverage table.
	fmt.Fprintf(output, "| Metric | Coverage | Details |\n")
	fmt.Fprintf(output, "|--------|----------|----------|\n")
	fmt.Fprintf(output, "| Statement Coverage | %s | %s |\n", coverageData.StatementCoverage, statementDetails)
	fmt.Fprintf(output, "| Function Coverage | %.1f%% | %s |\n\n", functionCoveragePercent, functionDetails)

	// Show uncovered functions from changed files only.
	if len(coverageData.FunctionCoverage) > 0 {
		writePRFilteredUncoveredFunctions(output, coverageData.FunctionCoverage)
	}

	// Show filtered files if any.
	if len(coverageData.FilteredFiles) > 0 {
		fmt.Fprintf(output, "<details>\n")
		fmt.Fprintf(output, "<summary>Excluded mock files (%d)</summary>\n\n", len(coverageData.FilteredFiles))
		for _, file := range coverageData.FilteredFiles {
			fmt.Fprintf(output, "- `%s`\n", file)
		}
		fmt.Fprintf(output, "\n</details>\n\n")
	}
}

func writePRFilteredUncoveredFunctions(output io.Writer, functions []CoverageFunction) {
	if len(functions) == 0 {
		return
	}

	// Get changed files for PR filtering.
	changedFiles := getChangedFiles()
	if len(changedFiles) == 0 {
		// No changed files detected, skip this section.
		return
	}

	// Create set of changed files for faster lookup.
	changedFileSet := make(map[string]bool)
	for _, file := range changedFiles {
		changedFileSet[file] = true
	}

	// Filter uncovered functions to only those in changed files.
	var uncoveredInPR []CoverageFunction
	totalUncovered := 0

	for _, fn := range functions {
		if fn.Coverage == 0 {
			totalUncovered++
			// Check if this function's file is in the changed files.
			for changedFile := range changedFileSet {
				if strings.Contains(fn.File, changedFile) || strings.Contains(changedFile, fn.File) {
					uncoveredInPR = append(uncoveredInPR, fn)
					break
				}
			}
		}
	}

	// Only show if there are uncovered functions in PR files.
	if len(uncoveredInPR) > 0 {
		fmt.Fprintf(output, "<details>\n")
		fmt.Fprintf(output, "<summary>❌ Uncovered Functions in This PR (%d of %d)</summary>\n\n", len(uncoveredInPR), totalUncovered)
		fmt.Fprintf(output, "| Function | File |\n")
		fmt.Fprintf(output, "|----------|------|\n")
		for _, fn := range uncoveredInPR {
			file := shortPackage(fn.File)
			fmt.Fprintf(output, "| `%s` | %s |\n", fn.Function, file)
		}
		fmt.Fprintf(output, "\n</details>\n\n")
	}
}

func writeLegacyCoverageSection(output io.Writer, coverage string) {
	fmt.Fprintf(output, "# Test Coverage\n\n")
	coverageFloat, _ := strconv.ParseFloat(strings.TrimSuffix(coverage, "%"), base10BitSize)
	emoji := "🔴" // red for < 40%.
	if coverageFloat >= coverageHighThreshold {
		emoji = "🟢" // green for >= 80%.
	} else if coverageFloat >= coverageMedThreshold {
		emoji = "🟡" // yellow for 40-79%.
	}

	fmt.Fprintf(output, "| Metric | Coverage | Details |\n")
	fmt.Fprintf(output, "|--------|----------|----------|\n")
	fmt.Fprintf(output, "| Statement Coverage | %s | %s |\n\n", coverage, emoji)
}

func writeFailedTests(output io.Writer, failed []TestResult) {
	fmt.Fprintf(output, "### ❌ Failed Tests (%d)\n\n", len(failed))

	if len(failed) == 0 {
		fmt.Fprintf(output, "No tests failed 🎉\n\n")
		return
	}

	fmt.Fprintf(output, "<details>\n")
	fmt.Fprintf(output, "<summary>Click to see failed tests</summary>\n\n")
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
	fmt.Fprintf(output, "</details>\n\n")
}

func writeSkippedTests(output io.Writer, skipped []TestResult) {
	if len(skipped) == 0 {
		return
	}
	fmt.Fprintf(output, "### ⏭️ Skipped Tests (%d)\n\n", len(skipped))
	fmt.Fprintf(output, "<details>\n")
	fmt.Fprintf(output, "<summary>Click to see skipped tests</summary>\n\n")
	fmt.Fprintf(output, "| Test | Package |\n")
	fmt.Fprintf(output, "|------|--------|\n")
	for _, test := range skipped {
		pkg := shortPackage(test.Package)
		fmt.Fprintf(output, "| `%s` | %s |\n", test.Test, pkg)
	}
	fmt.Fprintf(output, "\n</details>\n\n")
}

func writePassedTests(output io.Writer, passed []TestResult) {
	if len(passed) == 0 {
		return
	}

	fmt.Fprintf(output, "### ✅ Passed Tests (%d)\n\n", len(passed))

	// For small number of tests, show all in one block.
	if len(passed) < minTestsForSmartDisplay {
		fmt.Fprintf(output, "<details>\n")
		fmt.Fprintf(output, "<summary>Click to show all passing tests</summary>\n\n")
		writeTestTable(output, passed, true)
		fmt.Fprintf(output, "</details>\n\n")
		return
	}

	// For large number of tests, use hybrid strategy.
	changedPackages := getChangedPackages()
	changedTests := filterTestsByPackages(passed, changedPackages)
	slowestTests := getTopSlowestTests(passed, maxSlowestTests)
	packageSummaries := generatePackageSummary(passed)

	testsShown := len(changedTests) + len(slowestTests)
	fmt.Fprintf(output, "> Showing %d of %d passed tests. Full results available locally.\n\n", testsShown, len(passed))

	// Show tests from changed packages.
	if len(changedTests) > 0 {
		fmt.Fprintf(output, "<details>\n")
		fmt.Fprintf(output, "<summary>📝 Tests in Changed Packages (%d)</summary>\n\n", len(changedTests))
		writeTestTable(output, changedTests, true)
		fmt.Fprintf(output, "</details>\n\n")
	}

	// Show slowest tests.
	if len(slowestTests) > 0 {
		fmt.Fprintf(output, "<details>\n")
		fmt.Fprintf(output, "<summary>⏱️ Slowest Tests (%d)</summary>\n\n", len(slowestTests))
		writeTestTable(output, slowestTests, true)
		fmt.Fprintf(output, "</details>\n\n")
	}

	// Show package summary.
	if len(packageSummaries) > 0 {
		fmt.Fprintf(output, "<details>\n")
		fmt.Fprintf(output, "<summary>📊 Package Summary</summary>\n\n")
		fmt.Fprintf(output, "| Package | Tests Passed | Avg Duration | Total Duration |\n")
		fmt.Fprintf(output, "|---------|--------------|--------------|----------------|\n")
		for _, summary := range packageSummaries {
			fmt.Fprintf(output, "| %s | %d | %.3fs | %.2fs |\n",
				summary.Package, summary.TestCount, summary.AvgDuration, summary.TotalDuration)
		}
		fmt.Fprintf(output, "\n</details>\n\n")
	}
}

func writeTestTable(output io.Writer, tests []TestResult, includeDuration bool) {
	if includeDuration {
		fmt.Fprintf(output, "| Test | Package | Duration |\n")
		fmt.Fprintf(output, "|------|---------|----------|\n")
		for _, test := range tests {
			pkg := shortPackage(test.Package)
			fmt.Fprintf(output, "| `%s` | %s | %.2fs |\n", test.Test, pkg, test.Duration)
		}
	} else {
		fmt.Fprintf(output, "| Test | Package |\n")
		fmt.Fprintf(output, "|------|--------|\n")
		for _, test := range tests {
			pkg := shortPackage(test.Package)
			fmt.Fprintf(output, "| `%s` | %s |\n", test.Test, pkg)
		}
	}
	fmt.Fprintf(output, "\n")
}

func getChangedFiles() []string {
	cmd := exec.Command("git", "diff", "--name-only", "origin/main...HEAD")
	output, err := cmd.Output()
	if err != nil {
		// If git command fails, return empty slice (fallback to showing all).
		return []string{}
	}

	var files []string
	scanner := bufio.NewScanner(strings.NewReader(string(output)))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line != "" && strings.HasSuffix(line, ".go") {
			files = append(files, line)
		}
	}
	return files
}

func getChangedPackages() []string {
	files := getChangedFiles()
	packageSet := make(map[string]bool)

	for _, file := range files {
		// Convert file path to package path.
		// e.g., "tools/test-summary/main.go" -> "tools/test-summary"
		dir := filepath.Dir(file)
		if dir != "." {
			packageSet[dir] = true
		}
	}

	var packages []string
	for pkg := range packageSet {
		packages = append(packages, pkg)
	}
	return packages
}

func filterTestsByPackages(tests []TestResult, packages []string) []TestResult {
	if len(packages) == 0 {
		return []TestResult{} // No changed packages, return empty.
	}

	packageSet := make(map[string]bool)
	for _, pkg := range packages {
		packageSet[pkg] = true
	}

	var filtered []TestResult
	for _, test := range tests {
		// Check if test package ends with any of the changed packages.
		for pkg := range packageSet {
			if strings.Contains(test.Package, pkg) {
				filtered = append(filtered, test)
				break
			}
		}
	}
	return filtered
}

func getTopSlowestTests(tests []TestResult, n int) []TestResult {
	if len(tests) <= n {
		return tests
	}

	// Sort by duration descending.
	sorted := make([]TestResult, len(tests))
	copy(sorted, tests)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Duration > sorted[j].Duration
	})

	return sorted[:n]
}

type PackageSummary struct {
	Package       string
	TestCount     int
	AvgDuration   float64
	TotalDuration float64
}

func generatePackageSummary(tests []TestResult) []PackageSummary {
	packageStats := make(map[string]*PackageSummary)

	for _, test := range tests {
		pkg := shortPackage(test.Package)
		if _, exists := packageStats[pkg]; !exists {
			packageStats[pkg] = &PackageSummary{
				Package: pkg,
			}
		}
		stats := packageStats[pkg]
		stats.TestCount++
		stats.TotalDuration += test.Duration
	}

	var summaries []PackageSummary
	for _, stats := range packageStats {
		stats.AvgDuration = stats.TotalDuration / float64(stats.TestCount)
		summaries = append(summaries, *stats)
	}

	// Sort by total duration descending.
	sort.Slice(summaries, func(i, j int) bool {
		return summaries[i].TotalDuration > summaries[j].TotalDuration
	})

	return summaries
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
