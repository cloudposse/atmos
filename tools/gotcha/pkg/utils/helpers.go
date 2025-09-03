package utils

import (
	"bufio"
	"encoding/json"
	"fmt"
	"go/ast"
	goparser "go/parser"
	"go/token"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"

	"github.com/cloudposse/atmos/tools/gotcha/internal/parser"
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

// getTestCount uses AST parsing to quickly count Test and Example functions.
func GetTestCount(testPackages []string, testArgs string) int {

	totalTests := 0
	fset := token.NewFileSet()

	for _, pkg := range testPackages {
		// Handle special package patterns
		var searchDir string
		if pkg == "./..." {
			searchDir = "."
		} else if strings.HasSuffix(pkg, "/...") {
			searchDir = strings.TrimSuffix(pkg, "/...")
		} else {
			searchDir = pkg
		}

		// Walk through directories to find Go test files
		err := filepath.Walk(searchDir, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}

			// Skip non-Go files and non-test files
			if !strings.HasSuffix(path, "_test.go") {
				return nil
			}

			// Skip vendor directories and hidden directories
			if strings.Contains(path, "/vendor/") || strings.Contains(path, "/.") {
				return nil
			}

			// Parse the Go file
			src, err := os.ReadFile(path)
			if err != nil {
				return nil
			}

			file, err := goparser.ParseFile(fset, path, src, goparser.ParseComments)
			if err != nil {
				return nil
			}

			// Count Test and Example functions
			for _, decl := range file.Decls {
				if fn, ok := decl.(*ast.FuncDecl); ok {
					if fn.Name != nil {
						name := fn.Name.Name
						if strings.HasPrefix(name, "Test") || strings.HasPrefix(name, "Example") {
							totalTests++
						}
					}
				}
			}

			return nil
		})
		if err != nil {
		}
	}

	return totalTests
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
func RunSimpleStream(testPackages []string, testArgs, outputFile, coverProfile, showFilter string, totalTests int, alert bool) int {

	// Build the go test command
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

// StreamProcessor handles real-time test output with buffering.
type StreamProcessor struct {
	mu         sync.Mutex
	buffers    map[string][]string
	jsonWriter io.Writer
	showFilter string
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
		buffers:    make(map[string][]string),
		jsonWriter: jsonFile,
		showFilter: showFilter,
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
	// Skip package-level events
	if event.Test == "" {
		return
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	switch event.Action {
	case "run":
		// Initialize buffer for this test
		p.buffers[event.Test] = []string{}
		// Don't show "Running..." messages in non-TTY mode to avoid clutter

	case "output":
		// Buffer the output
		if p.buffers[event.Test] != nil {
			p.buffers[event.Test] = append(p.buffers[event.Test], event.Output)
		}

	case "pass":
		// Track statistics
		p.passed++
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
		// Always show failures regardless of filter
		fmt.Fprintf(os.Stderr, " %s FAILED: %s %s\n",
			tui.FailStyle.Render(tui.CheckFail),
			tui.TestNameStyle.Render(event.Test),
			tui.DurationStyle.Render(fmt.Sprintf("(%.2fs)", event.Elapsed)))

		// Show buffered error output
		if output, exists := p.buffers[event.Test]; exists {
			for _, line := range output {
				// Filter to show only meaningful error lines
				if parser.ShouldShowErrorLine(line) {
					fmt.Fprint(os.Stderr, "    "+line)
				}
			}
		}
		delete(p.buffers, event.Test)

	case "skip":
		// Track statistics
		p.skipped++
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

	fmt.Fprintf(os.Stderr, "\n")
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
}

// Note: HandleOutput and HandleConsoleOutput have been moved to the output package
// to avoid circular dependencies.
