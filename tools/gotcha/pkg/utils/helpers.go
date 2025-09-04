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
func RunSimpleStream(testPackages []string, testArgs, outputFile, coverProfile, showFilter string, alert bool) int {
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
	exitCode := RunTestsWithSimpleStreaming(args, outputFile, showFilter)

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

// StreamProcessor handles real-time test output with buffering.
type StreamProcessor struct {
	mu           sync.Mutex
	buffers      map[string][]string
	subtestStats map[string]*SubtestStats // Track subtest statistics per parent test
	jsonWriter   io.Writer
	showFilter   string
	startTime    time.Time
	currentTest    string // Track current test for package-level output
	currentPackage string // Track current package being tested
	packagesWithNoTests map[string]bool // Track packages that have no test files
	packageHasTests map[string]bool // Track if package had any test run events
	packageNoTestsPrinted map[string]bool // Track if we already printed "No tests" for a package
	// Statistics tracking
	passed  int
	failed  int
	skipped int
}

// runTestsWithSimpleStreaming runs tests and processes output in real-time.
func RunTestsWithSimpleStreaming(testArgs []string, outputFile, showFilter string) int {
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
		packagesWithNoTests: make(map[string]bool),
		packageHasTests: make(map[string]bool),
		packageNoTestsPrinted: make(map[string]bool),
		jsonWriter:   jsonFile,
		showFilter:   showFilter,
		startTime:    time.Now(),
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
	}

	return scanner.Err()
}

func (p *StreamProcessor) processEvent(event *types.TestEvent) {
	p.mu.Lock()
	defer p.mu.Unlock()


	// Handle package-level events
	if event.Test == "" {
		// Handle package start events
		if event.Action == "start" && event.Package != "" {
			// Check if this is a new package
			if p.currentPackage != event.Package {
				p.currentPackage = event.Package
				// Initialize tracking for this package
				p.packageHasTests[event.Package] = false
				// Print package header with arrow and styled package name
				fmt.Fprintf(os.Stderr, "\n▶ %s\n\n", 
					tui.PackageHeaderStyle.Render(event.Package))
			}
		} else if event.Action == "skip" && event.Package != "" && event.Test == "" {
			// Package was skipped (usually means no test files)
			if !p.packageNoTestsPrinted[event.Package] {
				fmt.Fprintf(os.Stderr, "  %s\n", 
					tui.DurationStyle.Render("No tests"))
				p.packageNoTestsPrinted[event.Package] = true
			}
		} else if event.Action == "output" && event.Package != "" && event.Test == "" {
			// Check for "no test files" message in output
			if strings.Contains(event.Output, "[no test files]") {
				// Mark that this package has no tests
				p.packagesWithNoTests[event.Package] = true
			}
		} else if event.Action == "pass" && event.Package != "" && event.Test == "" {
			// When a package passes, check if we need to show "No tests"
			if !p.packageNoTestsPrinted[event.Package] {
				if p.packagesWithNoTests[event.Package] {
					// Package had "[no test files]" in output
					// If this isn't the current package, we need to show which package this is for
					if p.currentPackage != event.Package {
						fmt.Fprintf(os.Stderr, "\n  %s for %s\n", 
							tui.DurationStyle.Render("No tests"),
							tui.PackageHeaderStyle.Render(event.Package))
					} else {
						fmt.Fprintf(os.Stderr, "  %s\n", 
							tui.DurationStyle.Render("No tests"))
					}
					p.packageNoTestsPrinted[event.Package] = true
				} else if hasTests, exists := p.packageHasTests[event.Package]; exists && !hasTests {
					// Package passed but no tests were run
					// If this isn't the current package, we need to show which package this is for
					if p.currentPackage != event.Package {
						fmt.Fprintf(os.Stderr, "\n  %s for %s\n", 
							tui.DurationStyle.Render("No tests"),
							tui.PackageHeaderStyle.Render(event.Package))
					} else {
						fmt.Fprintf(os.Stderr, "  %s\n", 
							tui.DurationStyle.Render("No tests"))
					}
					p.packageNoTestsPrinted[event.Package] = true
				}
			}
		} else if event.Action == "output" && p.currentTest != "" {
			// Package-level output might contain important command output
			// Append package-level output to the current test's buffer
			if p.buffers[p.currentTest] != nil {
				p.buffers[p.currentTest] = append(p.buffers[p.currentTest], event.Output)
			}
		}
		return
	}

	// Mark that this package has tests
	if event.Package != "" && event.Test != "" {
		p.packageHasTests[event.Package] = true
	}

	switch event.Action {
	case "run":
		p.currentTest = event.Test
		// Initialize buffer for this test (only if doesn't exist to preserve early output)
		if p.buffers[event.Test] == nil {
			p.buffers[event.Test] = []string{}
		}
		// Don't show "Running..." messages in non-TTY mode to avoid clutter

	case "output":
		// Buffer the output
		// Create buffer if it doesn't exist (can happen with subtests or out-of-order events)
		if p.buffers[event.Test] == nil {
			p.buffers[event.Test] = []string{}
		}
		p.buffers[event.Test] = append(p.buffers[event.Test], event.Output)

	case "pass":
		// Track statistics
		p.passed++

		// Track subtest statistics
		if strings.Contains(event.Test, "/") {
			// This is a subtest - update parent's stats
			parts := strings.SplitN(event.Test, "/", 2)
			parentTest := parts[0]
			subtestName := parts[1]

			if p.subtestStats[parentTest] == nil {
				p.subtestStats[parentTest] = &SubtestStats{}
			}
			p.subtestStats[parentTest].passed = append(p.subtestStats[parentTest].passed, subtestName)
		}

		// Show success with actual test name
		if p.shouldShowTestEvent("pass") {
			fmt.Fprintf(os.Stderr, " %s %s %s\n",
				tui.PassStyle.Render(tui.CheckPass),
				tui.TestNameStyle.Render(event.Test),
				tui.DurationStyle.Render(fmt.Sprintf("(%.2fs)", event.Elapsed)))
		}
		// Clear buffer
		delete(p.buffers, event.Test)

	case "fail":
		// Track statistics
		p.failed++

		// Track subtest statistics
		if strings.Contains(event.Test, "/") {
			// This is a subtest - update parent's stats
			parts := strings.SplitN(event.Test, "/", 2)
			parentTest := parts[0]
			subtestName := parts[1]

			if p.subtestStats[parentTest] == nil {
				p.subtestStats[parentTest] = &SubtestStats{}
			}
			p.subtestStats[parentTest].failed = append(p.subtestStats[parentTest].failed, subtestName)
		}

		// Check if this is a parent test with subtests
		if !strings.Contains(event.Test, "/") && p.subtestStats[event.Test] != nil {
			stats := p.subtestStats[event.Test]
			totalSubtests := len(stats.passed) + len(stats.failed) + len(stats.skipped)

			// Generate mini progress indicator
			miniProgress := p.generateSubtestProgress(len(stats.passed), totalSubtests)
			percentage := 0
			if totalSubtests > 0 {
				percentage = (len(stats.passed) * 100) / totalSubtests
			}

			// Display parent test with subtest summary in line
			// Use pass style if all subtests passed, fail style otherwise
			var statusIcon string
			if percentage == 100 && len(stats.failed) == 0 {
				statusIcon = tui.PassStyle.Render(tui.CheckPass)
			} else {
				statusIcon = tui.FailStyle.Render(tui.CheckFail)
			}
			fmt.Fprintf(os.Stderr, " %s %s %s %s %d%% passed\n",
				statusIcon,
				tui.TestNameStyle.Render(event.Test),
				tui.DurationStyle.Render(fmt.Sprintf("(%.2fs)", event.Elapsed)),
				miniProgress,
				percentage)

			// Add detailed subtest summary
			if totalSubtests > 0 {
				fmt.Fprintf(os.Stderr, "\n    Subtest Summary: %d passed, %d failed of %d total\n",
					len(stats.passed), len(stats.failed), totalSubtests)

				// Show passed subtests
				if len(stats.passed) > 0 {
					fmt.Fprintf(os.Stderr, "\n    %s Passed (%d):\n",
						tui.PassStyle.Render("✔"), len(stats.passed))
					for _, name := range stats.passed {
						fmt.Fprintf(os.Stderr, "      • %s\n", name)
					}
				}

				// Show failed subtests
				if len(stats.failed) > 0 {
					fmt.Fprintf(os.Stderr, "\n    %s Failed (%d):\n",
						tui.FailStyle.Render("✘"), len(stats.failed))
					for _, name := range stats.failed {
						fmt.Fprintf(os.Stderr, "      • %s\n", name)
					}
				}

				// Show skipped subtests if any
				if len(stats.skipped) > 0 {
					fmt.Fprintf(os.Stderr, "\n    %s Skipped (%d):\n",
						tui.SkipStyle.Render("⊘"), len(stats.skipped))
					for _, name := range stats.skipped {
						fmt.Fprintf(os.Stderr, "      • %s\n", name)
					}
				}
			}
		} else {
			// Regular test or subtest - display normally
			fmt.Fprintf(os.Stderr, " %s %s %s\n",
				tui.FailStyle.Render(tui.CheckFail),
				tui.TestNameStyle.Render(event.Test),
				tui.DurationStyle.Render(fmt.Sprintf("(%.2fs)", event.Elapsed)))
		}

		// Show buffered error output (only for tests without subtests or subtests themselves)
		if strings.Contains(event.Test, "/") || p.subtestStats[event.Test] == nil {
			output, exists := p.buffers[event.Test]

			// If no output found, check for subtest output (parent test might have no direct output)
			if !exists || len(output) == 0 {
				testPrefix := event.Test + "/"
				for testName, testOutput := range p.buffers {
					if strings.HasPrefix(testName, testPrefix) && len(testOutput) > 0 {
						output = append(output, testOutput...)
					}
				}
			}

			// Display ALL output for failed tests (including command output)
			for _, line := range output {
				fmt.Fprint(os.Stderr, "    "+line)
			}
		}

		delete(p.buffers, event.Test)

	case "skip":
		// Track statistics
		p.skipped++

		// Track subtest statistics
		if strings.Contains(event.Test, "/") {
			// This is a subtest - update parent's stats
			parts := strings.SplitN(event.Test, "/", 2)
			parentTest := parts[0]
			subtestName := parts[1]

			if p.subtestStats[parentTest] == nil {
				p.subtestStats[parentTest] = &SubtestStats{}
			}
			p.subtestStats[parentTest].skipped = append(p.subtestStats[parentTest].skipped, subtestName)
		}

		// Show skip with actual test name
		if p.shouldShowTestEvent("skip") {
			fmt.Fprintf(os.Stderr, " %s %s\n",
				tui.SkipStyle.Render(tui.CheckSkip),
				tui.TestNameStyle.Render(event.Test))
		}
		delete(p.buffers, event.Test)
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
	total := p.passed + p.failed + p.skipped
	if total == 0 {
		return
	}

	fmt.Fprintf(os.Stderr, "\n\n")
	fmt.Fprintf(os.Stderr, "%s\n", tui.StatsHeaderStyle.Render("Test Results:"))
	fmt.Fprintf(os.Stderr, "  %s Passed:  %d\n", tui.PassStyle.Render(tui.CheckPass), p.passed)
	if p.failed > 0 {
		fmt.Fprintf(os.Stderr, "  %s Failed:  %d\n", tui.FailStyle.Render(tui.CheckFail), p.failed)
	}
	if p.skipped > 0 {
		fmt.Fprintf(os.Stderr, "  %s Skipped: %d\n", tui.SkipStyle.Render(tui.CheckSkip), p.skipped)
	}
	fmt.Fprintf(os.Stderr, "  Total:     %d\n", total)
	fmt.Fprintf(os.Stderr, "\n")

	// Log completion time as info message
	elapsed := time.Since(p.startTime)
	fmt.Fprintf(os.Stderr, "%s Tests completed in %.2fs\n", tui.DurationStyle.Render("ℹ"), elapsed.Seconds())
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

// Note: HandleOutput and HandleConsoleOutput have been moved to the output package
// to avoid circular dependencies.
