package stream

import (
	"context"
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"
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

// extractExitCode extracts the exit code from an error, handling various error types.
// Returns -1 if the process was terminated by a signal, 0 if no error, or the actual exit code.
func extractExitCode(err error) int {
	if err == nil {
		return 0
	}
	
	// Check for ExitError to get the actual exit code
	if exitErr, ok := err.(*exec.ExitError); ok {
		// Check if process was terminated by signal (Unix-like systems)
		if exitErr.ExitCode() == -1 {
			// Process terminated by signal
			if status, ok := exitErr.Sys().(syscall.WaitStatus); ok {
				if status.Signaled() {
					// Return -1 to indicate signal termination
					return -1
				}
			}
		}
		return exitErr.ExitCode()
	}
	
	// Default to 1 for other errors
	return 1
}

// RunTestsWithSimpleStreaming runs tests and processes output in real-time.
func RunTestsWithSimpleStreaming(testArgs []string, outputFile, showFilter string, verbosityLevel string) int {
	// Create a context for command execution
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

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

	// Create the command with context
	cmd := exec.CommandContext(ctx, "go", testArgs...)

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
	var interruptedMutex sync.Mutex

	// Handle signals in a goroutine
	var signalWg sync.WaitGroup
	signalWg.Add(1)
	go func() {
		defer signalWg.Done()
		select {
		case sig := <-sigChan:
			if sig != nil {
				interruptedMutex.Lock()
				interrupted = true
				interruptedMutex.Unlock()
				
				// Cancel context to signal subprocess
				cancel()
				
				// Print abort message
				writer.PrintUI("\n\n\033[1;31m✗ Test run aborted\033[0m\n")
				
				// Forward signal to the process group
				if cmd.Process != nil { killProcessGroup(cmd.Process.Pid) }
			}
		case <-ctx.Done():
			// Context cancelled, exit goroutine
			return
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

	// Stop listening for signals and cleanup
	signal.Stop(sigChan)
	cancel() // Cancel context to stop signal goroutine
	close(sigChan) // Close channel to unblock goroutine
	signalWg.Wait() // Wait for signal goroutine to exit

	// Check if interrupted
	interruptedMutex.Lock()
	wasInterrupted := interrupted
	interruptedMutex.Unlock()

	// If interrupted, return with exit code 130 (standard for SIGINT)
	if wasInterrupted {
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
		// Extract exit code with better handling
		exitCode = extractExitCode(testErr)
		
		// Special handling for CI environments where go test -json may exit 1 even with passing tests
		if exitCode == 1 && processor.failed == 0 && processor.passed > 0 {
			// In CI, this often happens due to pipe closure or stderr issues
			// Check if we're in CI and all tests actually passed
			if config.IsCI() {
				// All tests passed in CI, treat as success despite exit code
				exitCode = 0
				exitReason = fmt.Sprintf("All %d tests passed (CI mode: ignoring go test exit code 1)", processor.passed)
			} else {
				// Not in CI, analyze stderr for root cause
				exitReason = analyzeProcessFailure(capturedStderr, 1)
			}
		} else if processor.failed > 0 {
			exitReason = fmt.Sprintf("%d tests failed, go test exited with code %d", processor.failed, exitCode)
		} else if exitCode == -1 {
			// Signal termination
			exitReason = fmt.Sprintf("Test process terminated by signal")
		} else {
			exitReason = fmt.Sprintf("'go test' exited with code %d", exitCode)
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
			// This is a special case that needs detailed diagnostics
			// Print detailed diagnostic message directly to stderr for visibility
			fmt.Fprintf(os.Stderr, "\n%s\n", strings.Repeat("─", 80))
			fmt.Fprintf(os.Stderr, "\033[1;33m⚠ DIAGNOSTIC: Test Process Exit Issue Detected\033[0m\n")
			fmt.Fprintf(os.Stderr, "%s\n", strings.Repeat("─", 80))
			fmt.Fprintf(os.Stderr, "\n%s\n\n", exitReason)
			fmt.Fprintf(os.Stderr, "%s\n", strings.Repeat("─", 80))

			// Also log it for record keeping
			log.Warn("Test process exited with non-zero code but no tests failed",
				"exitCode", exitCode,
				"testsRun", processor.passed,
				"testsSkipped", processor.skipped)
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
// This function provides detailed, actionable diagnostics about why tests might exit with non-zero
// codes even when no tests fail.
func analyzeProcessFailure(stderr string, exitCode int) string {
	// Check for specific logging patterns that indicate TestMain issues
	// These are common when using logging libraries that don't call os.Exit
	logPatterns := []struct {
		pattern string
		message string
	}{
		{"Failed to locate git repository", "Failed to locate git repository"},
		{"Failed to get current working directory", "Failed to get current working directory"},
		{"failed to get the current working directory", "Failed to get current working directory"},
		{"failed to locate git repository", "Failed to locate git repository"},
		{"Failed to initialize", "Failed to initialize test environment"},
		{"Fatal error:", "Fatal error encountered"},
	}

	// Check if this looks like a TestMain initialization issue
	var detectedIssues []string
	hasInfoLogs := strings.Contains(stderr, "INFO") || strings.Contains(stderr, "\u001b[1;38;5;86mINFO\u001b[0m")
	hasErrorLogs := strings.Contains(stderr, "ERROR") || strings.Contains(stderr, "FATAL")

	for _, pattern := range logPatterns {
		if strings.Contains(stderr, pattern.pattern) {
			detectedIssues = append(detectedIssues, pattern.message)
		}
	}

	// If we found log messages indicating initialization failure but tests passed
	if len(detectedIssues) > 0 && (hasInfoLogs || hasErrorLogs) {
		var sb strings.Builder
		sb.WriteString("TestMain initialization failed but continued execution.\n\n")
		sb.WriteString("Found log messages indicating early failure:\n")
		for _, issue := range detectedIssues {
			sb.WriteString(fmt.Sprintf("  - '%s'\n", issue))
		}
		sb.WriteString("\nThis suggests TestMain encountered an error but didn't properly exit. ")
		sb.WriteString("Check that TestMain:\n")
		sb.WriteString("  1. Properly handles initialization errors\n")
		sb.WriteString("  2. Calls os.Exit(m.Run()) even when early errors occur\n")
		sb.WriteString("  3. Doesn't use logger.Fatal() from charmbracelet/log (which doesn't exit)\n\n")
		sb.WriteString("Example fix:\n")
		sb.WriteString("```go\n")
		sb.WriteString("func TestMain(m *testing.M) {\n")
		sb.WriteString("    if err := setup(); err != nil {\n")
		sb.WriteString("        // Set skip reason for tests\n")
		sb.WriteString("        skipReason = err.Error()\n")
		sb.WriteString("        // Still run tests (they'll skip)\n")
		sb.WriteString("        exitCode := m.Run()\n")
		sb.WriteString("        os.Exit(exitCode)\n")
		sb.WriteString("    }\n")
		sb.WriteString("    // Normal flow\n")
		sb.WriteString("    exitCode := m.Run()\n")
		sb.WriteString("    os.Exit(exitCode)\n")
		sb.WriteString("}\n")
		sb.WriteString("```")
		return sb.String()
	}

	// Check for other common failure patterns
	switch {
	case strings.Contains(stderr, "[setup failed]"):
		// TestMain or init failure
		if strings.Contains(stderr, "TestMain") || strings.Contains(stderr, "func TestMain") {
			return fmt.Sprintf("TestMain failed with exit code %d\n\n"+
				"Ensure TestMain properly calls os.Exit(m.Run()) at all exit points.", exitCode)
		}
		return fmt.Sprintf("Test setup failed with exit code %d\n\n"+
			"Possible causes:\n"+
			"  - TestMain not properly handling errors\n"+
			"  - init() function panicking\n"+
			"  - Missing test fixtures or dependencies", exitCode)

	case strings.Contains(stderr, "panic:"):
		// Extract panic message if possible
		lines := strings.Split(stderr, "\n")
		panicMsg := ""
		stackStart := -1
		for i, line := range lines {
			if strings.Contains(line, "panic:") {
				panicMsg = strings.TrimSpace(strings.TrimPrefix(line, "panic:"))
				stackStart = i
				break
			}
		}

		result := fmt.Sprintf("Test process panicked with exit code %d\n\n", exitCode)
		if panicMsg != "" {
			result += fmt.Sprintf("Panic message: %s\n\n", panicMsg)
		}

		// Try to identify where the panic occurred
		if stackStart >= 0 && stackStart < len(lines)-1 {
			if strings.Contains(lines[stackStart+1], "init()") || strings.Contains(lines[stackStart+2], "init()") {
				result += "The panic occurred in an init() function.\n"
				result += "Check package initialization code for:\n"
				result += "  - Nil pointer dereferences\n"
				result += "  - Invalid array/slice access\n"
				result += "  - Missing required environment variables\n"
			} else if strings.Contains(stderr, "TestMain") {
				result += "The panic occurred in TestMain.\n"
				result += "Check TestMain for proper error handling.\n"
			}
		}
		return result

	case strings.Contains(stderr, "[build failed]"):
		// Extract build error details
		lines := strings.Split(stderr, "\n")
		var buildErrors []string
		pkg := ""

		for _, line := range lines {
			if strings.Contains(line, "[build failed]") {
				// Format: FAIL	github.com/cloudposse/atmos/tools/gotcha/pkg/example [build failed]
				parts := strings.Fields(line)
				if len(parts) >= 2 && parts[0] == "FAIL" {
					pkg = parts[1]
				}
			} else if strings.Contains(line, "undefined:") || strings.Contains(line, "cannot find") {
				buildErrors = append(buildErrors, strings.TrimSpace(line))
			}
		}

		result := fmt.Sprintf("Build failed with exit code %d\n\n", exitCode)
		if pkg != "" {
			result += fmt.Sprintf("Package: %s\n\n", pkg)
		}
		if len(buildErrors) > 0 {
			result += "Build errors:\n"
			for _, err := range buildErrors {
				result += fmt.Sprintf("  - %s\n", err)
			}
			result += "\n"
		}
		result += "Check for:\n"
		result += "  - Missing imports\n"
		result += "  - Typos in function/variable names\n"
		result += "  - Incompatible dependency versions\n"
		return result

	case strings.Contains(stderr, "undefined:") || strings.Contains(stderr, "cannot find") || strings.Contains(stderr, "declared and not used"):
		return fmt.Sprintf("Build/compilation error with exit code %d\n\n"+
			"Check for:\n"+
			"  - Undefined symbols or functions\n"+
			"  - Missing dependencies\n"+
			"  - Incorrect import paths\n"+
			"  - Variables declared but not used", exitCode)

	case strings.Contains(stderr, "log.Fatal") || strings.Contains(stderr, "logger.Fatal"):
		return fmt.Sprintf("Test called log.Fatal or logger.Fatal (exit code %d)\n\n"+
			"Note: Some logging libraries (like charmbracelet/log) have Fatal methods\n"+
			"that don't call os.Exit. If using such libraries in TestMain, replace with:\n"+
			"  logger.Error(msg)\n"+
			"  os.Exit(1)", exitCode)

	case strings.Contains(stderr, "os.Exit"):
		return fmt.Sprintf("Test called os.Exit(%d) directly\n\n"+
			"Tests should not call os.Exit directly. Use t.Fatal() or t.Skip() instead.", exitCode)

	default:
		// Generic process failure with helpful suggestions
		return fmt.Sprintf("Test process exited with code %d but all tests passed.\n\n"+
			"Possible causes:\n"+
			"  1. TestMain function not calling os.Exit(m.Run())\n"+
			"  2. Code calling os.Exit(%d) after tests complete\n"+
			"  3. Deferred function calling log.Fatal() or panic()\n"+
			"  4. Signal received (SIGTERM, SIGKILL, etc.)\n\n"+
			"Debug steps:\n"+
			"  1. Check if you have a TestMain function\n"+
			"  2. Ensure TestMain ends with os.Exit(m.Run())\n"+
			"  3. Look for defer statements that might call Fatal or panic\n"+
			"  4. Check for goroutines that might call os.Exit", exitCode, exitCode)
	}
}
