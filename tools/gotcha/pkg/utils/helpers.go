package utils

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/cloudposse/atmos/tools/gotcha/internal/logger"
	"github.com/cloudposse/atmos/tools/gotcha/internal/tui"
	"github.com/cloudposse/atmos/tools/gotcha/pkg/types"

	"github.com/spf13/viper"
)

// isValidShowFilter validates that the show filter is one of the allowed values.
func IsValidShowFilter(show string) bool {
	validFilters := []string{"all", "failed", "passed", "skipped", "collapsed", "none"}
	for _, valid := range validFilters {
		if show == valid {
			return true
		}
	}
	return false
}

// filterPackages applies include/exclude regex patterns to filter packages.
func FilterPackages(packages []string, includePatterns, excludePatterns string) ([]string, error) {
	// If no packages provided, return as-is
	if len(packages) == 0 {
		return packages, nil
	}

	// Parse include patterns
	var includeRegexes []*regexp.Regexp
	if includePatterns != "" {
		for _, pattern := range strings.Split(includePatterns, ",") {
			pattern = strings.TrimSpace(pattern)
			if pattern != "" {
				regex, err := regexp.Compile(pattern)
				if err != nil {
					return nil, fmt.Errorf("%w: '%s': %v", types.ErrInvalidIncludePattern, pattern, err)
				}
				includeRegexes = append(includeRegexes, regex)
			}
		}
	}

	// Parse exclude patterns
	var excludeRegexes []*regexp.Regexp
	if excludePatterns != "" {
		for _, pattern := range strings.Split(excludePatterns, ",") {
			pattern = strings.TrimSpace(pattern)
			if pattern != "" {
				regex, err := regexp.Compile(pattern)
				if err != nil {
					return nil, fmt.Errorf("%w: '%s': %v", types.ErrInvalidExcludePattern, pattern, err)
				}
				excludeRegexes = append(excludeRegexes, regex)
			}
		}
	}

	// If no patterns specified, return original packages
	if len(includeRegexes) == 0 && len(excludeRegexes) == 0 {
		return packages, nil
	}

	// Filter packages
	var filtered []string
	for _, pkg := range packages {
		// Check include patterns (if any)
		included := len(includeRegexes) == 0 // Default to include if no include patterns
		for _, regex := range includeRegexes {
			if regex.MatchString(pkg) {
				included = true
				break
			}
		}

		// Check exclude patterns (if any)
		excluded := false
		for _, regex := range excludeRegexes {
			if regex.MatchString(pkg) {
				excluded = true
				break
			}
		}

		// Include if it matches include patterns and doesn't match exclude patterns
		if included && !excluded {
			filtered = append(filtered, pkg)
		}
	}

	return filtered, nil
}

// isTTY checks if we're running in a terminal and Bubble Tea can actually use it.
func IsTTY() bool {
	// Provide an environment override
	_ = viper.BindEnv("GOTCHA_FORCE_NO_TTY", "FORCE_NO_TTY")
	if viper.GetString("GOTCHA_FORCE_NO_TTY") != "" {
		return false
	}

	// Debug: Force TTY mode for testing (but only if TTY is actually usable)
	_ = viper.BindEnv("GOTCHA_FORCE_TTY", "FORCE_TTY")
	if viper.GetString("GOTCHA_FORCE_TTY") != "" {
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

// emitAlert outputs a terminal bell (\a) if alert is enabled.
func EmitAlert(enabled bool) {
	if enabled {
		fmt.Fprint(os.Stderr, "\a")
	}
}

// runSimpleStream runs tests with simple non-interactive streaming output.
func RunSimpleStream(testPackages []string, testArgs, outputFile, coverProfile, showFilter string, alert bool, verbosityLevel string) int {
	// Configure colors and initialize styles for stream mode
	profile := tui.ConfigureColors()

	// Debug: Log the detected color profile in CI
	if os.Getenv("CI") != "" || os.Getenv("GITHUB_ACTIONS") != "" {
		logger.GetLogger().Debug("Color profile detected", "profile", tui.ProfileName(profile), "CI", os.Getenv("CI"), "GITHUB_ACTIONS", os.Getenv("GITHUB_ACTIONS"))
	}

	// Build the go test command
	args := []string{"test", "-json"}

	// Add coverage if requested
	if coverProfile != "" {
		args = append(args, fmt.Sprintf("-coverprofile=%s", coverProfile))
	}

	// Add verbose flag
	args = append(args, "-v")

	// Add timeout and other test arguments
	if testArgs != "" {
		// Parse testArgs string into individual arguments
		extraArgs := strings.Fields(testArgs)
		args = append(args, extraArgs...)
	}

	// Add packages to test
	args = append(args, testPackages...)

	// Run the tests
	exitCode := RunTestsWithSimpleStreaming(args, outputFile, showFilter, verbosityLevel)

	// Emit alert at completion
	EmitAlert(alert)
	return exitCode
}

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

// runTestsWithSimpleStreaming runs tests and processes output in real-time.
func RunTestsWithSimpleStreaming(testArgs []string, outputFile, showFilter string, verbosityLevel string) int {
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
	processor := &StreamProcessor{
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

		jsonWriter:     jsonFile,
		showFilter:     showFilter,
		verbosityLevel: verbosityLevel,
		startTime:      time.Now(),
	}

	// Process the stream
	processErr := processor.processStream(stdout)

	// Wait for command to complete
	testErr := cmd.Wait()

	// Print summary regardless of errors
	processor.printSummary()

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

func (p *StreamProcessor) processStream(input io.Reader) error {
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

func (p *StreamProcessor) processEvent(event *types.TestEvent) {
	// We'll collect any package that needs to be displayed
	var packageToDisplay *PackageResult

	p.mu.Lock()

	// Handle package-level events
	if event.Test == "" {
		// Handle package start events
		if event.Action == "start" && event.Package != "" {
			// Create new package result entry
			if _, exists := p.packageResults[event.Package]; !exists {
				p.packageResults[event.Package] = &PackageResult{
					Package:   event.Package,
					StartTime: time.Now(),
					Status:    "running",
					Tests:     make(map[string]*TestResult),
					TestOrder: []string{},
					HasTests:  false,
				}
				p.packageOrder = append(p.packageOrder, event.Package)
				p.activePackages[event.Package] = true

				// Keep legacy tracking for compatibility
				p.currentPackage = event.Package
				p.packageHasTests[event.Package] = false
			}
		} else if event.Action == "skip" && event.Package != "" && event.Test == "" {
			// Package was skipped (usually means no test files)
			if pkg, exists := p.packageResults[event.Package]; exists {
				pkg.Status = "skip"
				pkg.Elapsed = event.Elapsed
				pkg.EndTime = time.Now()
				delete(p.activePackages, event.Package)

				// Mark package for display after lock release
				packageToDisplay = pkg
			}
		} else if event.Action == "output" && event.Package != "" && event.Test == "" {
			// Package-level output (coverage, build errors, etc.)
			if pkg, exists := p.packageResults[event.Package]; exists {
				pkg.Output = append(pkg.Output, event.Output)

				// Check for coverage
				if strings.Contains(event.Output, "coverage:") {
					// Extract coverage information properly
					if strings.Contains(event.Output, "coverage: [no statements]") {
						// No statements to cover
						pkg.Coverage = "0.0%"
					} else if strings.Contains(event.Output, "coverage: [no test files]") {
						// No test files - shouldn't happen with actual tests
						pkg.Coverage = "0.0%"
					} else {
						// Extract percentage from normal coverage output
						if matches := strings.Fields(event.Output); len(matches) >= 2 {
							for i, field := range matches {
								if field == "coverage:" && i+1 < len(matches) {
									coverage := matches[i+1]
									// Keep only valid percentage values
									if strings.HasSuffix(coverage, "%") {
										pkg.Coverage = coverage
									} else {
										// Handle edge cases
										pkg.Coverage = "0.0%"
									}
									break
								}
							}
						}
					}
				}

				// Check for "no test files" message
				if strings.Contains(event.Output, "[no test files]") {
					// Mark for legacy compatibility
					p.packagesWithNoTests[event.Package] = true
				}

				// Check for package-level FAIL in output (e.g., TestMain failures)
				// This catches "FAIL\tpackage.name\t0.123s" which go test outputs
				if strings.Contains(event.Output, "FAIL\t"+event.Package) {
					// Mark package as failed - it likely has tests that failed to run
					if pkg, exists := p.packageResults[event.Package]; exists {
						// Don't override status if already set, but ensure we know tests exist
						if pkg.Status == "running" {
							pkg.Status = "fail"
						}
						pkg.HasTests = true // It has tests, they just failed to run
						// Store the output for display
						pkg.Output = append(pkg.Output, event.Output)
					}
				}
			}
		} else if event.Action == "pass" && event.Package != "" && event.Test == "" {
			// Package passed
			if pkg, exists := p.packageResults[event.Package]; exists {
				pkg.Status = "pass"
				pkg.Elapsed = event.Elapsed
				pkg.EndTime = time.Now()
				delete(p.activePackages, event.Package)

				// Check if package had no tests
				if !pkg.HasTests || p.packagesWithNoTests[event.Package] {
					// Package has no runnable tests
					pkg.HasTests = false
				}

				// Mark package for display after lock release
				packageToDisplay = pkg
			}
		} else if event.Action == "fail" && event.Package != "" && event.Test == "" {
			// Package failed
			if pkg, exists := p.packageResults[event.Package]; exists {
				pkg.Status = "fail"
				pkg.Elapsed = event.Elapsed
				pkg.EndTime = time.Now()
				delete(p.activePackages, event.Package)

				// If no tests were recorded but package failed, it likely has tests that couldn't run
				// (e.g., TestMain failure, compilation error, etc.)
				if len(pkg.Tests) == 0 && !p.packagesWithNoTests[event.Package] {
					pkg.HasTests = true
				}

				// Mark package for display after lock release
				packageToDisplay = pkg
			}
		} else if event.Action == "output" && p.currentTest != "" {
			// Package-level output might contain important command output
			// Append package-level output to the current test's buffer
			if p.buffers[p.currentTest] != nil {
				p.buffers[p.currentTest] = append(p.buffers[p.currentTest], event.Output)
			}
		}

		// Release lock and display package if needed before returning
		p.mu.Unlock()
		if packageToDisplay != nil {
			p.displayPackageResult(packageToDisplay)
		}
		return
	}

	// Mark that this package has tests
	if event.Package != "" && event.Test != "" {
		p.packageHasTests[event.Package] = true
		if pkg, exists := p.packageResults[event.Package]; exists {
			pkg.HasTests = true
		}
	}

	switch event.Action {
	case "run":
		p.currentTest = event.Test

		// Create test result entry in package
		if pkg, exists := p.packageResults[event.Package]; exists {
			test := &TestResult{
				Name:         event.Test,
				FullName:     event.Test,
				Status:       "running",
				Output:       []string{},
				Subtests:     make(map[string]*TestResult),
				SubtestOrder: []string{},
			}

			// Handle subtests
			if strings.Contains(event.Test, "/") {
				parts := strings.SplitN(event.Test, "/", 2)
				parentName := parts[0]
				subtestName := parts[1]

				if parent, ok := pkg.Tests[parentName]; ok {
					test.Parent = parentName
					test.Name = subtestName // Store just the subtest name
					parent.Subtests[event.Test] = test
					parent.SubtestOrder = append(parent.SubtestOrder, event.Test)
				}
				// Note: We don't add subtests to pkg.Tests or pkg.TestOrder
			} else {
				// Top-level test
				pkg.Tests[event.Test] = test
				pkg.TestOrder = append(pkg.TestOrder, event.Test)
				pkg.HasTests = true
			}
		}

		// Keep legacy buffer for compatibility
		if p.buffers[event.Test] == nil {
			p.buffers[event.Test] = []string{}
		}

	case "output":
		// Buffer the output in test result
		if pkg, exists := p.packageResults[event.Package]; exists {
			if test := p.findTest(pkg, event.Test); test != nil {
				test.Output = append(test.Output, event.Output)
			}
		}

		// Keep legacy buffer for compatibility
		if p.buffers[event.Test] == nil {
			p.buffers[event.Test] = []string{}
		}
		p.buffers[event.Test] = append(p.buffers[event.Test], event.Output)

	case "pass":
		// Update test result
		if pkg, exists := p.packageResults[event.Package]; exists {
			if test := p.findTest(pkg, event.Test); test != nil {
				test.Status = "pass"
				test.Elapsed = event.Elapsed
			}
		}

		// Track statistics
		p.passed++

		// Clear buffer
		delete(p.buffers, event.Test)

	case "fail":
		// Update test result
		if pkg, exists := p.packageResults[event.Package]; exists {
			if test := p.findTest(pkg, event.Test); test != nil {
				test.Status = "fail"
				test.Elapsed = event.Elapsed
			}
		}

		// Track statistics
		p.failed++

		// Clear buffer
		delete(p.buffers, event.Test)

	case "skip":
		// Update test result
		if pkg, exists := p.packageResults[event.Package]; exists {
			if test := p.findTest(pkg, event.Test); test != nil {
				test.Status = "skip"
				test.Elapsed = event.Elapsed
			}
		}

		// Track statistics
		p.skipped++

		// Clear buffer
		delete(p.buffers, event.Test)
	}

	// Release lock before doing I/O
	p.mu.Unlock()

	// Display package if needed (after releasing lock to avoid deadlock)
	if packageToDisplay != nil {
		p.displayPackageResult(packageToDisplay)
	}
}

func (p *StreamProcessor) shouldShowTestEvent(action string) bool {
	switch p.showFilter {
	case "all":
		return true
	case "failed":
		return action == "fail"
	case "passed":
		return action == "pass"
	case "skipped":
		return action == "skip"
	case "collapsed", "none":
		return false
	default:
		return true
	}
}

// printSummary prints a final test summary with statistics.
func (p *StreamProcessor) printSummary() {
	total := p.passed + p.failed
	if total == 0 && p.skipped == 0 {
		return
	}

	fmt.Fprintf(os.Stderr, "\n\n")
	fmt.Fprintf(os.Stderr, "%s\n", tui.StatsHeaderStyle.Render("Test Results:"))
	fmt.Fprintf(os.Stderr, "  %s Passed:  %d\n", tui.PassStyle.Render(tui.CheckPass), p.passed)
	fmt.Fprintf(os.Stderr, "  %s Failed:  %d\n", tui.FailStyle.Render(tui.CheckFail), p.failed)
	fmt.Fprintf(os.Stderr, "  Total:     %d\n", total)
	fmt.Fprintf(os.Stderr, "\n")

	// Log completion time as info message
	elapsed := time.Since(p.startTime)
	fmt.Fprintf(os.Stderr, "%s Tests completed in %.2fs\n", tui.DurationStyle.Render("ℹ"), elapsed.Seconds())

	// Ensure output is flushed
	if os.Getenv("CI") != "" || os.Getenv("GITHUB_ACTIONS") != "" {
		os.Stderr.Sync()
	}
}

// generateSubtestProgress creates a visual progress indicator for subtest results.
func (p *StreamProcessor) generateSubtestProgress(passed, total int) string {
	const maxDots = 10 // Maximum number of dots to show for readability

	if total == 0 {
		return ""
	}

	// Determine how many dots to show (actual count up to maxDots)
	dotsToShow := total
	if dotsToShow > maxDots {
		dotsToShow = maxDots
	}

	// Calculate how many dots for passed vs failed
	passedDots := passed
	failedDots := total - passed

	// If we need to scale down to maxDots, do it proportionally
	if total > maxDots {
		passedDots = (passed * maxDots) / total
		failedDots = maxDots - passedDots
	}

	// Build the indicator with colored dots
	var indicator strings.Builder

	// Add green dots for passed tests
	for i := 0; i < passedDots; i++ {
		indicator.WriteString(tui.PassStyle.Render("●"))
	}

	// Add red dots for failed tests
	for i := 0; i < failedDots; i++ {
		indicator.WriteString(tui.FailStyle.Render("●"))
	}

	return indicator.String()
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

// displayPackageResult outputs the buffered results for a completed package.
func (p *StreamProcessor) displayPackageResult(pkg *PackageResult) {
	// Display package header - ▶ icon in white, package name in cyan
	fmt.Fprintf(os.Stderr, "\n▶ %s\n",
		tui.PackageHeaderStyle.Render(pkg.Package))

	// Flush output immediately in CI environments to prevent buffering
	if os.Getenv("CI") != "" || os.Getenv("GITHUB_ACTIONS") != "" {
		os.Stderr.Sync()
	}

	// Check for package-level failures (e.g., TestMain failures)
	if pkg.Status == "fail" && len(pkg.Tests) == 0 {
		// Package failed without running any tests (likely TestMain failure)
		fmt.Fprintf(os.Stderr, "  %s Package failed to run tests\n", tui.FailStyle.Render(tui.CheckFail))

		// Display any package-level output (error messages)
		if len(pkg.Output) > 0 {
			for _, line := range pkg.Output {
				if strings.TrimSpace(line) != "" {
					fmt.Fprintf(os.Stderr, "    %s", line)
				}
			}
		}
		return
	}

	// Check if package has no tests
	if !pkg.HasTests {
		fmt.Fprintf(os.Stderr, " %s\n", tui.DurationStyle.Render("No tests"))
		return
	}

	// Count test results for this package
	var passedCount, failedCount, skippedCount int
	for _, test := range pkg.Tests {
		switch test.Status {
		case "pass":
			passedCount++
		case "fail":
			failedCount++
		case "skip":
			skippedCount++
		}
	}

	// Display tests based on show filter
	for _, testName := range pkg.TestOrder {
		test := pkg.Tests[testName]
		p.displayTest(test, "")
	}

	// Display summary line with test counts and coverage
	totalTests := passedCount + failedCount + skippedCount
	if totalTests > 0 {
		var summaryLine string
		coverageStr := ""
		if pkg.Coverage != "" {
			coverageStr = fmt.Sprintf(" (%s coverage)", pkg.Coverage)
		}

		if failedCount > 0 {
			// Show failure summary
			summaryLine = fmt.Sprintf("  %s %d tests failed, %d passed%s\n",
				tui.FailStyle.Render(tui.CheckFail),
				failedCount,
				passedCount,
				coverageStr)
		} else if passedCount > 0 {
			// All tests passed
			summaryLine = fmt.Sprintf("  %s All %d tests passed%s\n",
				tui.PassStyle.Render(tui.CheckPass),
				passedCount,
				coverageStr)
		} else if skippedCount > 0 {
			// Only skipped tests
			summaryLine = fmt.Sprintf("  %s %d tests skipped%s\n",
				tui.SkipStyle.Render(tui.CheckSkip),
				skippedCount,
				coverageStr)
		}

		if summaryLine != "" {
			fmt.Fprintf(os.Stderr, "\n%s", summaryLine)
		}
	}

	// Flush output after displaying package results
	if os.Getenv("CI") != "" || os.Getenv("GITHUB_ACTIONS") != "" {
		os.Stderr.Sync()
	}
}

// displayTest outputs a single test result with proper formatting.
func (p *StreamProcessor) displayTest(test *TestResult, indent string) {
	// Check if test has failed subtests (for --show=failed filter)
	hasFailedSubtests := false
	if p.showFilter == "failed" && len(test.Subtests) > 0 {
		for _, subtest := range test.Subtests {
			if subtest.Status == "fail" {
				hasFailedSubtests = true
				break
			}
		}
	}

	// Check if we should display this test based on filter
	if !p.shouldShowTestStatus(test.Status) && !hasFailedSubtests {
		return
	}

	// Determine status icon
	var statusIcon string
	switch test.Status {
	case "pass":
		statusIcon = tui.PassStyle.Render(tui.CheckPass)
	case "fail":
		statusIcon = tui.FailStyle.Render(tui.CheckFail)
	case "skip":
		statusIcon = tui.SkipStyle.Render(tui.CheckSkip)
	default:
		return // Don't display running tests
	}

	// Build display line
	var line strings.Builder
	line.WriteString(indent + " ")
	line.WriteString(statusIcon)
	line.WriteString(" ")
	line.WriteString(tui.TestNameStyle.Render(test.Name))

	// Add duration for completed tests
	if test.Elapsed > 0 {
		line.WriteString(" ")
		line.WriteString(tui.DurationStyle.Render(fmt.Sprintf("(%.2fs)", test.Elapsed)))
	}

	// Check if test has subtests
	if len(test.Subtests) > 0 {
		// Calculate subtest statistics
		passed := 0
		failed := 0
		skipped := 0

		for _, subtest := range test.Subtests {
			switch subtest.Status {
			case "pass":
				passed++
			case "fail":
				failed++
			case "skip":
				skipped++
			}
		}

		total := passed + failed + skipped
		if total > 0 {
			// Add mini progress indicator
			miniProgress := p.generateSubtestProgress(passed, total)
			percentage := (passed * 100) / total

			line.WriteString(" ")
			line.WriteString(miniProgress)
			line.WriteString(fmt.Sprintf(" %d%% passed", percentage))
		}
	}

	fmt.Fprintln(os.Stderr, line.String())

	// Display test output for failures (respecting show filter)
	if test.Status == "fail" && len(test.Output) > 0 && p.showFilter != "none" {
		if p.verbosityLevel == "with-output" || p.verbosityLevel == "verbose" {
			// With full output, properly render tabs and maintain formatting
			for _, outputLine := range test.Output {
				formatted := strings.ReplaceAll(outputLine, `\t`, "\t")
				formatted = strings.ReplaceAll(formatted, `\n`, "\n")
				fmt.Fprint(os.Stderr, indent+"    "+formatted)
			}
		} else {
			// Default: show output as-is
			for _, outputLine := range test.Output {
				fmt.Fprint(os.Stderr, indent+"    "+outputLine)
			}
		}
	}

	// Display subtests if test failed or show filter is "all"
	if len(test.Subtests) > 0 && (test.Status == "fail" || p.showFilter == "all") {
		// Display subtest summary for failed tests
		if test.Status == "fail" {
			passed := []*TestResult{}
			failed := []*TestResult{}
			skipped := []*TestResult{}

			for _, subtestName := range test.SubtestOrder {
				subtest := test.Subtests[subtestName]
				switch subtest.Status {
				case "pass":
					passed = append(passed, subtest)
				case "fail":
					failed = append(failed, subtest)
				case "skip":
					skipped = append(skipped, subtest)
				}
			}

			total := len(passed) + len(failed) + len(skipped)
			if total > 0 {
				fmt.Fprintf(os.Stderr, "\n%s    Subtest Summary: %d passed, %d failed of %d total\n",
					indent, len(passed), len(failed), total)

				// Show passed subtests
				if len(passed) > 0 {
					fmt.Fprintf(os.Stderr, "\n%s    %s Passed (%d):\n",
						indent, tui.PassStyle.Render("✔"), len(passed))
					for _, subtest := range passed {
						fmt.Fprintf(os.Stderr, "%s      • %s\n", indent, subtest.Name)
					}
				}

				// Show failed subtests
				if len(failed) > 0 {
					fmt.Fprintf(os.Stderr, "\n%s    %s Failed (%d):\n",
						indent, tui.FailStyle.Render("✘"), len(failed))
					for _, subtest := range failed {
						fmt.Fprintf(os.Stderr, "%s      • %s\n", indent, subtest.Name)
						// Show subtest output if verbosity level is with-output or verbose
						if (p.verbosityLevel == "with-output" || p.verbosityLevel == "verbose") && len(subtest.Output) > 0 {
							for _, outputLine := range subtest.Output {
								formatted := strings.ReplaceAll(outputLine, `\t`, "\t")
								formatted = strings.ReplaceAll(formatted, `\n`, "\n")
								fmt.Fprint(os.Stderr, indent+"        "+formatted)
							}
						}
					}
				}

				// Show skipped subtests
				if len(skipped) > 0 {
					fmt.Fprintf(os.Stderr, "\n%s    %s Skipped (%d):\n",
						indent, tui.SkipStyle.Render("⊘"), len(skipped))
					for _, subtest := range skipped {
						fmt.Fprintf(os.Stderr, "%s      • %s\n", indent, subtest.Name)
					}
				}
			}
		} else if p.showFilter == "all" {
			// For "all" filter, subtests are already shown in mini progress
			// Don't display them again unless specifically requested
		}
	}
}

// shouldShowTestStatus determines if a test with the given status should be displayed.
func (p *StreamProcessor) shouldShowTestStatus(status string) bool {
	switch p.showFilter {
	case "all":
		return true
	case "failed":
		return status == "fail"
	case "passed":
		return status == "pass"
	case "skipped":
		return status == "skip"
	case "collapsed":
		return status == "fail" // Only show failures in collapsed mode
	case "none":
		return false
	default:
		return true
	}
}

// Note: HandleOutput and HandleConsoleOutput have been moved to the output package
// to avoid circular dependencies.
