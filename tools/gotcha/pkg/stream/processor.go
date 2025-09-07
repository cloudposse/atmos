package stream

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/cloudposse/atmos/tools/gotcha/internal/tui"
	"github.com/cloudposse/atmos/tools/gotcha/pkg/types"
)

// SubtestStats tracks statistics for subtests of a parent test.
type SubtestStats struct {
	passed  []string // names of passed subtests
	failed  []string // names of failed subtests
	skipped []string // names of skipped subtests
}

// PackageResult stores complete information about a tested package.
type PackageResult struct {
	Package   string
	StartTime time.Time
	EndTime   time.Time
	Status    string // "pass", "fail", "skip", "running"
	Tests     map[string]*TestResult
	TestOrder []string // Maintain test execution order
	Coverage  string   // e.g., "coverage: 75.2% of statements"
	Output    []string // Package-level output (build errors, etc.)
	Elapsed   float64
	HasTests  bool
}

// TestResult stores individual test information.
type TestResult struct {
	Name         string
	FullName     string // Full test name including package
	Status       string // "pass", "fail", "skip", "running"
	Elapsed      float64
	Output       []string // All output lines from this test
	Parent       string   // Parent test name for subtests
	Subtests     map[string]*TestResult
	SubtestOrder []string
}

// StreamProcessor handles real-time test output with buffering.
type StreamProcessor struct {
	mu             sync.Mutex
	buffers        map[string][]string
	subtestStats   map[string]*SubtestStats // Track subtest statistics per parent test
	jsonWriter     io.Writer
	showFilter     string
	testFilter     string // Test filter applied via -run flag (if any)
	verbosityLevel string // Verbosity level: standard, with-output, minimal, or verbose
	startTime      time.Time
	currentTest    string // Track current test for package-level output

	// Buffered output fields
	packageResults map[string]*PackageResult // Complete package results
	packageOrder   []string                  // Order packages were started
	activePackages map[string]bool           // Currently running packages

	// Legacy fields for compatibility (will be removed)
	currentPackage        string          // Track current package being tested
	packagesWithNoTests   map[string]bool // Track packages that have no test files
	packageHasTests       map[string]bool // Track if package had any test run events
	packageNoTestsPrinted map[string]bool // Track if we already printed "No tests" for a package

	// Statistics tracking
	passed  int
	failed  int
	skipped int
}

// NewStreamProcessor creates a new stream processor.
func NewStreamProcessor(jsonWriter io.Writer, showFilter, testFilter, verbosityLevel string) *StreamProcessor {
	return &StreamProcessor{
		buffers:      make(map[string][]string),
		subtestStats: make(map[string]*SubtestStats),

		// New buffered output fields
		packageResults: make(map[string]*PackageResult),
		packageOrder:   []string{},
		activePackages: make(map[string]bool),

		// Legacy fields (will be removed)
		packagesWithNoTests:   make(map[string]bool),
		packageHasTests:       make(map[string]bool),
		packageNoTestsPrinted: make(map[string]bool),

		jsonWriter:     jsonWriter,
		showFilter:     showFilter,
		testFilter:     testFilter,
		verbosityLevel: verbosityLevel,
		startTime:      time.Now(),
	}
}

// ProcessStream reads and processes the test output stream.
func (p *StreamProcessor) ProcessStream(input io.Reader) error {
	scanner := bufio.NewScanner(input)

	// Track if we're in CI for periodic flushing
	inCI := os.Getenv("CI") != "" || os.Getenv("GITHUB_ACTIONS") != ""
	lastFlush := time.Now()
	flushInterval := 100 * time.Millisecond // Flush frequently in CI

	for scanner.Scan() {
		line := scanner.Bytes()

		// Write to JSON file
		_, _ = p.jsonWriter.Write(line)
		_, _ = p.jsonWriter.Write([]byte("\n"))

		// Parse and process event
		var event types.TestEvent
		if err := json.Unmarshal(line, &event); err != nil {
			// Skip non-JSON lines
			continue
		}

		p.processEvent(&event)

		// Periodic flush in CI to ensure output appears promptly
		if inCI && time.Since(lastFlush) > flushInterval {
			os.Stderr.Sync()
			lastFlush = time.Now()
		}
	}

	// After processing all events, check for incomplete packages
	// These are packages that started but never completed (no pass/fail/skip event)
	p.mu.Lock()
	var incompletePackages []*PackageResult
	for pkgName := range p.activePackages {
		if pkg, exists := p.packageResults[pkgName]; exists {
			if pkg.Status == "running" {
				// Package started but never completed - likely failed
				pkg.Status = "fail"
				pkg.EndTime = time.Now()
				pkg.HasTests = true // Assume it has tests that failed to run
				incompletePackages = append(incompletePackages, pkg)
				delete(p.activePackages, pkgName)
			}
		}
	}
	p.mu.Unlock()

	// Display incomplete packages after releasing the lock
	for _, pkg := range incompletePackages {
		p.displayPackageResult(pkg)
	}

	return scanner.Err()
}

// PrintSummary prints a final test summary with statistics.
func (p *StreamProcessor) PrintSummary() {
	// FIX: Must hold lock when accessing shared maps to prevent concurrent read panic
	p.mu.Lock()
	defer p.mu.Unlock()

	total := p.passed + p.failed + p.skipped
	if total == 0 {
		return
	}

	// Calculate overall coverage
	var totalCoverage float64
	var packagesWithCoverage int
	for _, pkg := range p.packageResults {
		if pkg.Coverage != "" && pkg.Coverage != "0.0%" {
			// Parse percentage from string like "75.2%"
			coverageStr := strings.TrimSuffix(pkg.Coverage, "%")
			if coverageVal, err := strconv.ParseFloat(coverageStr, 64); err == nil {
				totalCoverage += coverageVal
				packagesWithCoverage++
			}
		}
	}

	fmt.Fprintf(os.Stderr, "\n\n")
	fmt.Fprintf(os.Stderr, "%s\n", tui.StatsHeaderStyle.Render("Test Results:"))
	fmt.Fprintf(os.Stderr, "  %s Passed:  %d\n", tui.PassStyle.Render(tui.CheckPass), p.passed)
	fmt.Fprintf(os.Stderr, "  %s Failed:  %d\n", tui.FailStyle.Render(tui.CheckFail), p.failed)
	fmt.Fprintf(os.Stderr, "  %s Skipped: %d\n", tui.SkipStyle.Render(tui.CheckSkip), p.skipped)
	fmt.Fprintf(os.Stderr, "  Total:     %d\n", total)

	// Display average coverage if available
	if packagesWithCoverage > 0 {
		avgCoverage := totalCoverage / float64(packagesWithCoverage)
		fmt.Fprintf(os.Stderr, "  Coverage:  %.1f%%\n", avgCoverage)
	}

	fmt.Fprintf(os.Stderr, "\n")

	// Log completion time as info message
	elapsed := time.Since(p.startTime)
	fmt.Fprintf(os.Stderr, "%s Tests completed in %.2fs\n", tui.DurationStyle.Render("ℹ"), elapsed.Seconds())

	// Ensure output is flushed
	if os.Getenv("CI") != "" || os.Getenv("GITHUB_ACTIONS") != "" {
		os.Stderr.Sync()
	}

	// Additional defensive flush for Windows to prevent race conditions
	if runtime.GOOS == "windows" {
		os.Stderr.Sync()
		os.Stdout.Sync()
	}
}

// RunTestsWithSimpleStreaming runs tests and processes output in real-time.
func RunTestsWithSimpleStreaming(testArgs []string, outputFile, showFilter string, verbosityLevel string) int {
	// Extract test filter from args if present
	var testFilter string
	for i := 0; i < len(testArgs)-1; i++ {
		if testArgs[i] == "-run" {
			testFilter = testArgs[i+1]
			break
		}
	}

	// Create the command
	cmd := exec.Command("go", testArgs...)
	cmd.Stderr = os.Stderr // Pass through stderr

	// Get stdout pipe
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return 1
	}

	// Start the command
	if err := cmd.Start(); err != nil {
		return 1
	}

	// Create JSON output file
	jsonFile, err := os.Create(outputFile)
	if err != nil {
		return 1
	}
	defer jsonFile.Close()

	// Create processor
	processor := NewStreamProcessor(jsonFile, showFilter, testFilter, verbosityLevel)

	// Process the stream
	processErr := processor.ProcessStream(stdout)

	// Wait for command to complete
	testErr := cmd.Wait()

	// Print summary regardless of errors
	processor.PrintSummary()

	// Return processing error if any
	if processErr != nil {
		return 1
	}

	// Handle test command exit code - pass through unmodified
	if testErr != nil {
		if exitErr, ok := testErr.(*exec.ExitError); ok {
			return exitErr.ExitCode()
		}
		return 1
	}

	return 0
}

// findTest locates a test within the package result hierarchy.
func (p *StreamProcessor) findTest(pkg *PackageResult, testName string) *TestResult {
	if strings.Contains(testName, "/") {
		// This is a subtest
		parts := strings.SplitN(testName, "/", 2)
		parentName := parts[0]

		if parent, exists := pkg.Tests[parentName]; exists {
			if subtest, exists := parent.Subtests[testName]; exists {
				return subtest
			}
		}
		return nil
	}

	// Top-level test
	return pkg.Tests[testName]
}
