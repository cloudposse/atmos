package stream

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"sync"
	"time"

	"github.com/cloudposse/atmos/tools/gotcha/internal/logger"
	"github.com/cloudposse/atmos/tools/gotcha/pkg/config"
	"github.com/cloudposse/atmos/tools/gotcha/pkg/constants"
	"github.com/cloudposse/atmos/tools/gotcha/pkg/output"
	"github.com/cloudposse/atmos/tools/gotcha/pkg/types"
)

// PackageResult stores complete information about a tested package.
type PackageResult struct {
	Package           string
	StartTime         time.Time
	EndTime           time.Time
	Status            string // "pass", "fail", "skip", "running"
	Tests             map[string]*TestResult
	TestOrder         []string // Maintain test execution order
	Coverage          string   // Legacy: single coverage value (for backward compatibility)
	StatementCoverage string   // Statement coverage percentage (e.g., "75.2%")
	FunctionCoverage  string   // Function coverage percentage (e.g., "80.0%")
	Output            []string // Package-level output (build errors, etc.)
	Elapsed           float64
	HasTests          bool
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
	SkipReason   string // Reason for skipping (extracted from output)
}

// StreamProcessor handles real-time test output with buffering.
type StreamProcessor struct {
	mu             sync.Mutex
	buffers        map[string][]string
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

	// TestReporter for handling display
	reporter TestReporter

	// Writer for output management
	writer *output.Writer
}

// NewStreamProcessor creates a new stream processor.
func NewStreamProcessor(jsonWriter io.Writer, showFilter, testFilter, verbosityLevel string) *StreamProcessor {
	writer := output.New()
	return &StreamProcessor{
		buffers: make(map[string][]string),

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
		writer:         writer,

		// Create default stream reporter for backward compatibility
		reporter: NewStreamReporter(writer, showFilter, testFilter, verbosityLevel),
	}
}

// NewStreamProcessorWithReporter creates a new stream processor with a custom reporter.
func NewStreamProcessorWithReporter(jsonWriter io.Writer, reporter TestReporter) *StreamProcessor {
	return &StreamProcessor{
		buffers: make(map[string][]string),

		// New buffered output fields
		packageResults: make(map[string]*PackageResult),
		packageOrder:   []string{},
		activePackages: make(map[string]bool),

		// Legacy fields (will be removed)
		packagesWithNoTests:   make(map[string]bool),
		packageHasTests:       make(map[string]bool),
		packageNoTestsPrinted: make(map[string]bool),

		jsonWriter: jsonWriter,
		startTime:  time.Now(),
		reporter:   reporter,
		writer:     output.New(),
	}
}

// ProcessStream reads and processes the test output stream.
func (p *StreamProcessor) ProcessStream(input io.Reader) error {
	// Write to debug file if specified
	if debugFile := config.GetDebugFile(); debugFile != "" {
		if f, err := os.OpenFile(debugFile, os.O_CREATE|os.O_APPEND|os.O_WRONLY, constants.DefaultFilePerms); err == nil {
			fmt.Fprintf(f, "\n=== STREAM MODE STARTED ===\n")
			fmt.Fprintf(f, "Time: %s\n", time.Now().Format(time.RFC3339))
			fmt.Fprintf(f, "Show filter: %s\n", p.showFilter)
			fmt.Fprintf(f, "===========================\n")
			f.Close()
		}
	}

	scanner := bufio.NewScanner(input)

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

		// Output is automatically flushed due to line buffering on stderr
		// We don't need explicit Sync() calls which can cause issues with pipes
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
		if p.reporter != nil {
			p.reporter.OnPackageComplete(pkg)
		}
	}

	return scanner.Err()
}

// PrintSummary prints a final test summary with statistics.
func (p *StreamProcessor) PrintSummary() {
	// FIX: Must hold lock when accessing shared maps to prevent concurrent read panic
	p.mu.Lock()
	passed := p.passed
	failed := p.failed
	skipped := p.skipped
	elapsed := time.Since(p.startTime)
	p.mu.Unlock()

	// Use reporter to finalize if available
	if p.reporter != nil {
		p.reporter.Finalize(passed, failed, skipped, elapsed)
	}
}

// TestExecutionResult contains the exit code and reason for test execution.
type TestExecutionResult struct {
	ExitCode   int
	ExitReason string
}

// lastExitReason stores the last exit reason from test execution for retrieval by the caller.
var lastExitReason string

// GetLastExitReason returns the last exit reason from test execution.
func GetLastExitReason() string {
	return lastExitReason
}

// RunTestsWithSimpleStreaming runs tests and processes output in real-time.
func RunTestsWithSimpleStreaming(testArgs []string, outputFile, showFilter string, verbosityLevel string) int {
	// Create a writer for output management
	writer := output.New()

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

	// Capture stderr while also displaying it
	var stderrBuffer bytes.Buffer
	cmd.Stderr = io.MultiWriter(writer.UI, &stderrBuffer)

	// Set platform-specific command attributes for proper process group handling
	setPlatformSpecificCmd(cmd)

	// Get stdout pipe
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return 1
	}

	// Start the command
	if err := cmd.Start(); err != nil {
		return 1
	}

	// Setup signal handling for Ctrl+C (platform-specific)
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, getInterruptSignals()...)

	// Track if we've been interrupted
	var interrupted bool

	// Handle signals in a goroutine
	go func() {
		<-sigChan
		interrupted = true

		// Print abort message
		writer.PrintUI("\n\n\033[1;31m✗ Test run aborted\033[0m\n")

		// Kill the test process
		if cmd.Process != nil {
			// Kill the process group (platform-specific implementation)
			killProcessGroup(cmd.Process.Pid)
			// Also kill the main process
			_ = cmd.Process.Kill()
		}
	}()

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

	// Stop listening for signals
	signal.Stop(sigChan)
	close(sigChan)

	// If interrupted, return with exit code 130 (standard for SIGINT)
	if interrupted {
		return ExitCodeInterrupted
	}

	// Determine exit code and reason first
	var exitCode int
	var exitReason string
	capturedStderr := stderrBuffer.String()

	// Return processing error if any
	switch {
	case processErr != nil:
		exitCode = 1
		exitReason = fmt.Sprintf("Processing error: %v", processErr)
	case testErr != nil:
		// Handle test command exit code - pass through unmodified
		if exitErr, ok := testErr.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
			switch {
			case exitCode == 1 && processor.failed == 0:
				// Tests passed but go test exited with 1 - analyze stderr for root cause
				exitReason = analyzeProcessFailure(capturedStderr, exitCode)
			case processor.failed > 0:
				exitReason = fmt.Sprintf("%d tests failed, go test exited with code %d", processor.failed, exitCode)
			default:
				exitReason = fmt.Sprintf("'go test' exited with code %d", exitCode)
			}
		} else {
			exitCode = 1
			exitReason = fmt.Sprintf("Test execution error: %v", testErr)
		}
	default:
		exitCode = 0
		exitReason = fmt.Sprintf("All %d tests passed successfully", processor.passed)
	}

	// Store the exit reason for retrieval by the caller
	lastExitReason = exitReason

	// Log the exit reason BEFORE printing summary
	// This ensures log messages appear before the formatted test output
	log := logger.GetLogger()
	if exitCode != 0 {
		// Check if this is the specific case where tests passed but go test failed
		if processor.failed == 0 && processor.passed > 0 {
			// This is likely a parsing issue or test setup problem, not a test failure
			log.Warn("Test process exited with non-zero code but no tests failed",
				"exitCode", exitCode,
				"testsRun", processor.passed,
				"testsSkipped", processor.skipped,
				"hint", "Check for panics, TestMain failures, or build errors")
		} else {
			// Log as error since this is a genuine test failure
			log.Error("Test run failed", "exitCode", exitCode, "reason", exitReason)
		}
	} else if verbosityLevel == "verbose" || verbosityLevel == "with-output" {
		log.Debug("Test run completed successfully", "exitCode", 0, "reason", exitReason)
	}

	// Print summary after logging
	processor.PrintSummary()

	return exitCode
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

// analyzeProcessFailure analyzes stderr output to determine the root cause of process failure.
func analyzeProcessFailure(stderr string, exitCode int) string {
	// Check for common failure patterns in stderr
	switch {
	case strings.Contains(stderr, "[setup failed]"):
		// TestMain or init failure
		if strings.Contains(stderr, "TestMain") || strings.Contains(stderr, "func TestMain") {
			return fmt.Sprintf("TestMain failed with exit code %d (check TestMain implementation - ensure it calls os.Exit(m.Run()))", exitCode)
		}
		return fmt.Sprintf("Test setup failed with exit code %d (possible TestMain or init() issue)", exitCode)

	case strings.Contains(stderr, "panic:"):
		// Extract panic message if possible
		lines := strings.Split(stderr, "\n")
		for _, line := range lines {
			if strings.Contains(line, "panic:") {
				panicMsg := strings.TrimSpace(strings.TrimPrefix(line, "panic:"))
				return fmt.Sprintf("Test process panicked: %s (exit code %d)", panicMsg, exitCode)
			}
		}
		return fmt.Sprintf("Test process panicked with exit code %d", exitCode)

	case strings.Contains(stderr, "[build failed]"):
		// Extract package name if possible
		lines := strings.Split(stderr, "\n")
		for _, line := range lines {
			if strings.Contains(line, "[build failed]") {
				// Format: FAIL	github.com/cloudposse/atmos/tools/gotcha/test/testutil/ptyrunner [build failed]
				parts := strings.Fields(line)
				if len(parts) >= 2 && parts[0] == "FAIL" {
					pkg := parts[1]
					return fmt.Sprintf("Build failed for package %s (exit code %d)", pkg, exitCode)
				}
			}
		}
		return fmt.Sprintf("Build failed with exit code %d", exitCode)

	case strings.Contains(stderr, "undefined:") || strings.Contains(stderr, "cannot find") || strings.Contains(stderr, "declared and not used"):
		return fmt.Sprintf("Build/compilation error with exit code %d (check for undefined symbols or missing dependencies)", exitCode)

	case strings.Contains(stderr, "log.Fatal") || strings.Contains(stderr, "logger.Fatal"):
		return fmt.Sprintf("Test called log.Fatal or logger.Fatal (exit code %d)", exitCode)

	case strings.Contains(stderr, "os.Exit"):
		return fmt.Sprintf("Test called os.Exit(%d) directly", exitCode)

	default:
		// Generic process failure
		return fmt.Sprintf("Test process failed with exit code %d (no test failures detected - possible process-level issue)", exitCode)
	}
}
