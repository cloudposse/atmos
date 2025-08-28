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
	var (
		inputFile  = flag.String("input", "-", "JSON test results file (- for stdin)")
		format     = flag.String("format", "both", "Output format: console, markdown, both, github")
		outputFile = flag.String("output", "", "Output file (default: stdout for markdown, test-summary.md for github)")
	)
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: test-summary [options]\n\n")
		fmt.Fprintf(os.Stderr, "Parse Go test JSON output and generate summaries.\n\n")
		fmt.Fprintf(os.Stderr, "Options:\n")
		flag.PrintDefaults()
		fmt.Fprintf(os.Stderr, "\nExamples:\n")
		fmt.Fprintf(os.Stderr, "  # Pipe test output through for console display\n")
		fmt.Fprintf(os.Stderr, "  go test -json ./... | test-summary -format=console\n\n")
		fmt.Fprintf(os.Stderr, "  # Generate markdown summary from file\n")
		fmt.Fprintf(os.Stderr, "  test-summary -input=test-results.json -format=markdown\n\n")
		fmt.Fprintf(os.Stderr, "  # Generate GitHub Actions summary\n")
		fmt.Fprintf(os.Stderr, "  test-summary -input=test-results.json -format=github\n")
	}
	flag.Parse()

	// Open input.
	input := os.Stdin
	if *inputFile != "-" {
		file, err := os.Open(*inputFile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error opening input file: %v\n", err)
			os.Exit(1)
		}
		defer file.Close()
		input = file
	}

	// Parse and process.
	summary, consoleOutput := parseTestJSON(input)

	// Output based on format.
	switch *format {
	case "console":
		fmt.Print(consoleOutput)
	case "markdown":
		output := *outputFile
		if output == "" {
			output = "-" // Default to stdout.
		}
		if err := writeSummary(summary, "markdown", output); err != nil {
			fmt.Fprintf(os.Stderr, "Error writing summary: %v\n", err)
			os.Exit(1)
		}
	case "github":
		output := *outputFile
		if err := writeSummary(summary, "github", output); err != nil {
			fmt.Fprintf(os.Stderr, "Error writing summary: %v\n", err)
			os.Exit(1)
		}
	case "both":
		fmt.Print(consoleOutput)
		output := *outputFile
		if output == "" {
			output = "-" // Default to stdout for markdown.
		}
		if err := writeSummary(summary, "markdown", output); err != nil {
			fmt.Fprintf(os.Stderr, "Error writing summary: %v\n", err)
			os.Exit(1)
		}
	default:
		fmt.Fprintf(os.Stderr, "Error: Invalid format '%s'. Use: console, markdown, both, or github\n", *format)
		os.Exit(1)
	}

	os.Exit(summary.ExitCode)
}

func parseTestJSON(input io.Reader) (*TestSummary, string) {
	scanner := bufio.NewScanner(input)
	var console strings.Builder
	results := make(map[string]*TestResult)
	summary := &TestSummary{}
	coverageRe := regexp.MustCompile(`coverage:\s+([\d.]+)%\s+of\s+statements`)

	for scanner.Scan() {
		line := scanner.Text()
		var event TestEvent

		if err := json.Unmarshal([]byte(line), &event); err != nil {
			// Not JSON, pass through.
			console.WriteString(line + "\n")
			continue
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
	}

	// Sort results into categories.
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

	// Sort each slice for consistent output.
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

	return summary, console.String()
}

func writeSummary(summary *TestSummary, format, outputFile string) error {
	var output io.Writer
	var outputPath string

	if format == "github" {
		// Try to use GitHub summary first.
		githubSummary := os.Getenv("GITHUB_STEP_SUMMARY")
		if githubSummary != "" {
			// Running in GitHub Actions.
			file, err := os.OpenFile(githubSummary, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
			if err != nil {
				return fmt.Errorf("failed to open GITHUB_STEP_SUMMARY file: %w", err)
			}
			defer file.Close()
			output = file
			outputPath = githubSummary
		} else {
			// Running locally - use default file.
			defaultFile := "test-summary.md"
			if outputFile != "" {
				defaultFile = outputFile
			}

			// Create the file in current directory.
			file, err := os.Create(defaultFile)
			if err != nil {
				return fmt.Errorf("failed to create summary file: %w", err)
			}
			defer file.Close()
			output = file
			outputPath = defaultFile

			// Inform the user.
			absPath, _ := filepath.Abs(defaultFile)
			fmt.Fprintf(os.Stderr, "📝 GITHUB_STEP_SUMMARY not set (running locally). Writing summary to: %s\n", absPath)
		}
	} else if outputFile == "-" || outputFile == "" {
		// Write to stdout.
		output = os.Stdout
		outputPath = "stdout"
	} else {
		// Write to specified file.
		file, err := os.Create(outputFile)
		if err != nil {
			return fmt.Errorf("failed to create output file: %w", err)
		}
		defer file.Close()
		output = file
		outputPath = outputFile
	}

	// Write markdown summary.
	total := len(summary.Passed) + len(summary.Failed) + len(summary.Skipped)

	// Add timestamp for local runs.
	if format == "github" && os.Getenv("GITHUB_STEP_SUMMARY") == "" {
		fmt.Fprintf(output, "_Generated: %s_\n\n", time.Now().Format("2006-01-02 15:04:05"))
	}

	fmt.Fprintf(output, "## Test Results\n\n")

	// Add coverage if available.
	if summary.Coverage != "" {
		coverageFloat, _ := strconv.ParseFloat(strings.TrimSuffix(summary.Coverage, "%"), 64)
		emoji := "🔴" // red for < 50%
		if coverageFloat >= 80 {
			emoji = "🟢" // green for >= 80%
		} else if coverageFloat >= 50 {
			emoji = "🟡" // yellow for 50-79%
		}
		fmt.Fprintf(output, "**Coverage:** %s %s of statements\n\n", emoji, summary.Coverage)
	}

	fmt.Fprintf(output, "**Summary:** %d tests • ✅ %d passed • ❌ %d failed • ⏭️ %d skipped\n\n",
		total, len(summary.Passed), len(summary.Failed), len(summary.Skipped))

	// Failed tests first (most important).
	if len(summary.Failed) > 0 {
		fmt.Fprintf(output, "### ❌ Failed Tests (%d)\n\n", len(summary.Failed))
		fmt.Fprintf(output, "| Test | Package | Duration |\n")
		fmt.Fprintf(output, "|------|---------|----------|\n")
		for _, test := range summary.Failed {
			pkg := shortPackage(test.Package)
			fmt.Fprintf(output, "| `%s` | %s | %.2fs |\n", test.Test, pkg, test.Duration)
		}
		fmt.Fprintf(output, "\n**Run locally to reproduce:**\n")
		fmt.Fprintf(output, "```bash\n")
		for _, test := range summary.Failed {
			fmt.Fprintf(output, "go test %s -run ^%s$ -v\n", test.Package, test.Test)
		}
		fmt.Fprintf(output, "```\n\n")
	}

	// Skipped tests.
	if len(summary.Skipped) > 0 {
		fmt.Fprintf(output, "### ⏭️ Skipped Tests (%d)\n\n", len(summary.Skipped))
		fmt.Fprintf(output, "| Test | Package |\n")
		fmt.Fprintf(output, "|------|---------|)\n")
		for _, test := range summary.Skipped {
			pkg := shortPackage(test.Package)
			fmt.Fprintf(output, "| `%s` | %s |\n", test.Test, pkg)
		}
		fmt.Fprintf(output, "\n")
	}

	// Passed tests (collapsible).
	if len(summary.Passed) > 0 {
		fmt.Fprintf(output, "### ✅ Passed Tests (%d)\n\n", len(summary.Passed))
		fmt.Fprintf(output, "<details>\n")
		fmt.Fprintf(output, "<summary>Click to show all passing tests</summary>\n\n")
		fmt.Fprintf(output, "| Test | Package | Duration |\n")
		fmt.Fprintf(output, "|------|---------|----------|\n")
		for _, test := range summary.Passed {
			pkg := shortPackage(test.Package)
			fmt.Fprintf(output, "| `%s` | %s | %.2fs |\n", test.Test, pkg, test.Duration)
		}
		fmt.Fprintf(output, "\n</details>\n\n")
	}

	// Always log success message for file outputs (not stdout).
	if outputPath != "stdout" && outputPath != "" {
		absPath, _ := filepath.Abs(outputPath)
		fmt.Fprintf(os.Stderr, "✅ Test summary written to: %s\n", absPath)
	}

	return nil
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
