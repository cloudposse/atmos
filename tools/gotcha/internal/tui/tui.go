package tui

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/cloudposse/atmos/tools/gotcha/pkg/types"

	"github.com/charmbracelet/bubbles/progress"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/viper"
	"golang.org/x/term"
)

// getTerminalWidth gets the current terminal width using golang.org/x/term.
func getTerminalWidth() int {
	width, _, err := term.GetSize(int(os.Stdout.Fd()))
	if err != nil {
		// Fallback to a reasonable default if we can't detect
		return 80
	}
	return width
}

// getDisplayWidth calculates the actual display width of a string, ignoring ANSI escape sequences.
func getDisplayWidth(s string) int {
	width := 0
	i := 0
	runes := []rune(s)

	for i < len(runes) {
		r := runes[i]

		switch r {
		case '\033': // ESC character - start of ANSI escape sequence
			// Skip the entire ANSI escape sequence
			i = skipAnsiSequence(runes, i)
		default:
			// Count printable characters
			if r >= 32 && r < 127 {
				width++
			} else if r > 127 {
				// Basic handling for Unicode - most characters are width 1
				width++
			}
			// Control characters (0-31) don't add to width
			i++
		}
	}

	return width
}

// skipAnsiSequence skips over ANSI escape sequences and returns the next index.
func skipAnsiSequence(runes []rune, start int) int {
	if start >= len(runes) || runes[start] != '\033' {
		return start + 1
	}

	i := start + 1
	if i >= len(runes) {
		return i
	}

	switch runes[i] {
	case '[':
		return skipCSISequence(runes, i)
	case '(', ')':
		return i + 2
	default:
		return i + 1
	}
}

// skipCSISequence handles CSI (Control Sequence Introducer) sequences.
func skipCSISequence(runes []rune, start int) int {
	i := start + 1 // skip '['

	// Skip parameters until we find the final character
	for i < len(runes) && isCSIParameter(runes[i]) {
		i++
	}

	// Skip the final character if present
	if i < len(runes) {
		i++
	}

	return i
}

// isCSIParameter checks if a rune is a valid CSI parameter character.
func isCSIParameter(r rune) bool {
	return (r >= '0' && r <= '9') || r == ';' || r == ' ' || r == '?' || r == '!'
}

// SubtestStats tracks statistics for subtests of a parent test.
type SubtestStats struct {
	passed  []string // names of passed subtests
	failed  []string // names of failed subtests
	skipped []string // names of skipped subtests
}

// PackageResult holds the complete test results for a package.
type PackageResult struct {
	Package   string
	StartTime time.Time
	EndTime   time.Time
	Status    string // "pass", "fail", "skip", "running"
	Tests     map[string]*TestResult
	TestOrder []string // Order in which tests were run
	Coverage  string   // Coverage percentage if available
	Output    []string // Package-level output
	Elapsed   float64
	HasTests  bool // Track if any tests ran
}

// TestResult holds the result of an individual test.
type TestResult struct {
	Name         string
	FullName     string // Full test name including package
	Status       string // "pass", "fail", "skip"
	Elapsed      float64
	Output       []string // All output from the test
	Parent       string   // Parent test name if this is a subtest
	Subtests     map[string]*TestResult
	SubtestOrder []string
	SkipReason   string // Reason why test was skipped (if applicable)
}

// TestModel represents the test UI model.
type TestModel struct {
	// Test tracking
	totalTests            int    // Total number of tests that will run (from "run" events or estimate)
	estimatedTestCount    int    // Original estimate from cache (preserved for display)
	actualTestCount       int    // Actual count of tests from "run" events
	usingEstimate         bool   // Whether we're still using the estimate or have actual count
	completedTests        int    // Number of tests that have completed (pass/fail/skip)
	testFilter            string // Test filter applied via -run flag (if any)
	currentIndex          int  // Legacy counter - will be removed
	currentTest           string
	currentPackage        string          // Current package being tested
	packagesWithNoTests   map[string]bool // Track packages that have "[no test files]" in output
	packageHasTests       map[string]bool // Track if package had any test run events
	packageNoTestsPrinted map[string]bool // Track if we already printed "No tests" for a package
	width                 int
	height                int
	done                  bool
	aborted               bool
	startTime             time.Time
	elapsedTime           time.Duration // Store elapsed time for logging after TUI exits

	// UI components
	spinner  spinner.Model
	progress progress.Model

	// Test execution
	cmd            *exec.Cmd
	outputFile     string
	showFilter     string // "all", "failed", "passed", "skipped"
	alert          bool   // whether to emit terminal bell on completion
	verbosityLevel string // Verbosity level: standard, with-output, minimal, or verbose

	// Results tracking
	passCount      int
	failCount      int
	skipCount      int
	testBuffers    map[string][]string
	subtestOutputs map[string][]string      // Persistent storage for subtest output
	subtestStats   map[string]*SubtestStats // Track subtest statistics per parent test
	bufferMu       sync.Mutex

	// Buffered output
	packageResults    map[string]*PackageResult // Complete package results
	packageOrder      []string                  // Order packages were completed
	activePackages    map[string]bool           // Track which packages are currently running
	displayedPackages map[string]bool           // Track which packages have been displayed

	// JSON output
	jsonFile *os.File

	// Streaming state - persistent across reads
	scanner *bufio.Scanner
	stdout  io.ReadCloser
}

// Bubble Tea messages.
type (
	testStartMsg struct{ test string }
	testPassMsg  struct {
		test    string
		elapsed float64
	}
)

type testFailMsg struct {
	test    string
	elapsed float64
	output  []string
}
type (
	testSkipMsg  struct{ test string }
	testsDoneMsg struct{}
	testErrorMsg struct{ err error }
)

// Message for when subprocess is initialized and ready to stream.
type subprocessReadyMsg struct {
	scanner  *bufio.Scanner
	stdout   io.ReadCloser
	jsonFile *os.File
}

// Message for streaming subprocess output.
type streamOutputMsg struct {
	line []byte
}

// NewTestModel creates a new test model for the TUI.
func NewTestModel(testPackages []string, testArgs, outputFile, coverProfile, showFilter string, alert bool, verbosityLevel string, estimatedTestCount int) TestModel {
	// Create progress bar with default gradient (purple theme used in Atmos)
	p := progress.New(
		progress.WithDefaultGradient(),
		progress.WithWidth(40),
		progress.WithoutPercentage(),
	)

	// Create spinner
	s := spinner.New()
	s.Style = spinnerStyle
	s.Spinner = spinner.Dot

	// Extract test filter from args if present
	var testFilter string
	if testArgs != "" {
		// Look for -run flag in the arguments
		args := strings.Fields(testArgs)
		for i := 0; i < len(args)-1; i++ {
			if args[i] == "-run" {
				testFilter = args[i+1]
				break
			}
		}
	}

	// Build go test command args
	args := []string{"test", "-json"}

	// Add coverage if requested
	if coverProfile != "" {
		args = append(args, fmt.Sprintf("-coverprofile=%s", coverProfile))
	}

	// Add verbose flag
	args = append(args, "-v")

	// Add timeout and other test arguments
	if testArgs != "" {
		extraArgs := strings.Fields(testArgs)
		args = append(args, extraArgs...)
	}

	// Add packages to test
	args = append(args, testPackages...)

	// Create command
	cmd := exec.Command("go", args...)
	cmd.Stderr = os.Stderr
	// Command runs from current directory (which should be repo root)

	return TestModel{
		cmd:                   cmd,
		outputFile:            outputFile,
		showFilter:            showFilter,
		alert:                 alert,
		verbosityLevel:        verbosityLevel,
		spinner:               s,
		progress:              p,
		testBuffers:           make(map[string][]string),
		subtestOutputs:        make(map[string][]string),
		subtestStats:          make(map[string]*SubtestStats),
		packagesWithNoTests:   make(map[string]bool),
		packageHasTests:       make(map[string]bool),
		packageNoTestsPrinted: make(map[string]bool),
		packageResults:        make(map[string]*PackageResult),
		packageOrder:          []string{},
		activePackages:        make(map[string]bool),
		displayedPackages:     make(map[string]bool),
		totalTests:            estimatedTestCount, // Use cached estimate if available, will be updated by "run" events
		estimatedTestCount:    estimatedTestCount, // Preserve original estimate
		usingEstimate:         estimatedTestCount > 0, // Track if we're using an estimate
		completedTests:        0,                   // Will be incremented by pass/fail/skip events
		testFilter:            testFilter,          // Store the test filter for display
		startTime:             time.Now(),
	}
}

func (m *TestModel) Init() tea.Cmd {
	return tea.Batch(
		m.startTestsCmd(),
		m.spinner.Tick,
		m.progress.Init(), // Initialize progress bar animation
	)
}

// startTestsCmd initializes and starts the test subprocess.
func (m *TestModel) startTestsCmd() tea.Cmd {
	return func() tea.Msg {
		// Open JSON output file
		jsonFile, err := os.Create(m.outputFile)
		if err != nil {
			return testErrorMsg{err: fmt.Errorf("failed to create output file: %w", err)}
		}

		// Start the go test command
		stdout, err := m.cmd.StdoutPipe()
		if err != nil {
			jsonFile.Close()
			return testErrorMsg{err: fmt.Errorf("failed to get stdout pipe: %w", err)}
		}

		if err := m.cmd.Start(); err != nil {
			jsonFile.Close()
			return testErrorMsg{err: fmt.Errorf("failed to start go test: %w", err)}
		}

		// Create scanner and return subprocess ready message
		scanner := bufio.NewScanner(stdout)
		return subprocessReadyMsg{
			scanner:  scanner,
			stdout:   stdout,
			jsonFile: jsonFile,
		}
	}
}

// readNextLine creates a command that reads one line using the persistent scanner.
func (m *TestModel) readNextLine() tea.Cmd {
	return func() tea.Msg {
		// Use the scanner from the model - this persists across reads
		if m.scanner != nil && m.scanner.Scan() {
			line := m.scanner.Bytes()

			// Write to JSON file
			if m.jsonFile != nil {
				_, _ = m.jsonFile.Write(line)
				_, _ = m.jsonFile.Write([]byte("\n"))
			}

			// Make a copy since scanner reuses the buffer
			lineCopy := make([]byte, len(line))
			copy(lineCopy, line)

			return streamOutputMsg{line: lineCopy}
		}

		// Scanner finished, close resources and signal completion
		if m.stdout != nil {
			m.stdout.Close()
		}
		if m.jsonFile != nil {
			m.jsonFile.Close()
		}
		return testsDoneMsg{}
	}
}

func (m *TestModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		// Store the size but we use our own terminal width detection
		m.width, m.height = msg.Width, msg.Height
		return m, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "esc", "q":
			m.aborted = true
			m.done = true
			if m.cmd != nil && m.cmd.Process != nil {
				_ = m.cmd.Process.Kill()
			}
			if m.jsonFile != nil {
				m.jsonFile.Close()
			}
			return m, tea.Sequence(
				tea.Printf("\n%s Tests aborted", FailStyle.Render(CheckFail)),
				tea.Quit,
			)
		}

	case subprocessReadyMsg:
		// Store the streaming state in the model
		m.scanner = msg.scanner
		m.stdout = msg.stdout
		m.jsonFile = msg.jsonFile
		// Start reading the first line
		return m, m.readNextLine()

	case streamOutputMsg:
		// Parse the JSON line and convert to appropriate message
		var event types.TestEvent
		if err := json.Unmarshal(msg.line, &event); err != nil {
			// Skip non-JSON lines, continue reading
			return m, tea.Batch(m.readNextLine(), m.spinner.Tick)
		}

		// Convert to appropriate test message and continue streaming
		nextCmd := m.readNextLine()

		// Process event with buffering
		m.processEvent(&event)

		// Check if any packages completed and display them once
		var cmds []tea.Cmd
		cmds = append(cmds, nextCmd, m.spinner.Tick)

		// Update progress bar if we have tests
		if m.totalTests > 0 && m.completedTests > 0 {
			percentFloat := float64(m.completedTests) / float64(m.totalTests)
			// Use SetPercent and ensure we return the animation command
			cmd := m.progress.SetPercent(percentFloat)
			if cmd != nil {
				cmds = append(cmds, cmd)
			}
		}

		for _, pkg := range m.packageOrder {
			if result, exists := m.packageResults[pkg]; exists && result.Status != "running" {
				// Package is complete, check if we've already displayed it
				if !m.displayedPackages[pkg] {
					// Mark as displayed and generate output
					m.displayedPackages[pkg] = true
					output := m.displayPackageResult(result)
					if output != "" {
						// Use tea.Printf to print the output once
						cmds = append(cmds, tea.Printf("%s", output))
					}
				}
			}
		}

		// Continue reading
		return m, tea.Batch(cmds...)

	case testsDoneMsg:
		// Check for incomplete packages (packages that started but never completed)
		for pkgName := range m.activePackages {
			if pkg, exists := m.packageResults[pkgName]; exists {
				if pkg.Status == "running" {
					// Package started but never completed - likely failed
					pkg.Status = "fail"
					pkg.EndTime = time.Now()
					pkg.HasTests = true // Assume it has tests that failed to run
					m.failCount++       // Count as a failure

					// Ensure it's in the package order
					if !contains(m.packageOrder, pkgName) {
						m.packageOrder = append(m.packageOrder, pkgName)
					}
				}
			}
		}

		// Update progress to 100% before marking as done
		var finalCmd tea.Cmd
		if m.totalTests > 0 {
			finalCmd = m.progress.SetPercent(1.0)
		}

		// Generate the final summary output before setting done
		var summaryOutput string
		if !m.aborted {
			summaryOutput = m.generateFinalSummary()
		}

		m.done = true
		if m.jsonFile != nil {
			m.jsonFile.Close()
		}

		// Emit alert if enabled
		emitAlert(m.alert)

		// Don't show final summary if aborted
		if !m.aborted {
			cmds := []tea.Cmd{
				tea.Printf("%s", summaryOutput),
				tea.Quit,
			}
			if finalCmd != nil {
				cmds = append([]tea.Cmd{finalCmd}, cmds...)
			}
			return m, tea.Sequence(cmds...)
		}
		return m, tea.Quit

	case testErrorMsg:
		m.done = true
		if m.jsonFile != nil {
			m.jsonFile.Close()
		}

		// Emit alert if enabled
		emitAlert(m.alert)

		return m, tea.Sequence(
			tea.Printf("%s Error: %v", FailStyle.Render(CheckFail), msg.err),
			tea.Quit,
		)

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd

	case progress.FrameMsg:
		progressModel, cmd := m.progress.Update(msg)
		m.progress = progressModel.(progress.Model)
		return m, cmd
	}

	return m, nil
}

func (m *TestModel) View() string {
	if m.done {
		// Return a newline to clear the progress bar line
		return "\n"
	}

	// Get terminal width for layout calculations
	terminalWidth := getTerminalWidth()
	if terminalWidth == 0 {
		terminalWidth = 80 // Default fallback
	}

	// Build the status line components
	spin := m.spinner.View() + " "

	// Test name with fixed width for stability
	const maxTestWidth = 45
	var info string
	if m.currentTest != "" {
		testName := m.currentTest
		if len(testName) > maxTestWidth {
			testName = testName[:maxTestWidth-3] + "..."
		}
		// Pad test name to exactly maxTestWidth BEFORE styling
		testName = fmt.Sprintf("%-*s", maxTestWidth, testName)
		styledName := TestNameStyle.Render(testName)
		info = fmt.Sprintf("Running %s", styledName)
	} else {
		// Pad "Starting tests..." to match "Running " + maxTestWidth
		padded := fmt.Sprintf("%-*s", maxTestWidth+8, "Starting tests...")
		info = padded
	}

	// Calculate elapsed time
	elapsed := time.Since(m.startTime)
	elapsedSeconds := int(elapsed.Seconds())

	// Calculate buffer size
	bufferSizeKB := m.getBufferSizeKB()

	// Build the ordered status components
	var percentage string
	var testCount string
	
	// Always use estimate if we have one and are still using it
	if m.usingEstimate && m.estimatedTestCount > 0 {
		// Using cached estimate
		if m.completedTests > 0 {
			// Tests are running, show progress against estimate
			percentFloat := float64(m.completedTests) / float64(m.estimatedTestCount)
			percent := int(percentFloat * 100)
			percentage = fmt.Sprintf("%3d%%", percent)
		} else {
			// No tests completed yet
			percentage = "  0%"
		}
		// Show completed/estimated format with tilde prefix (since whole fraction is estimated)
		testCount = fmt.Sprintf("~%d/%d %s", m.completedTests, m.estimatedTestCount, DurationStyle.Render("tests"))
	} else if m.totalTests > 0 {
		// Not using estimate, have actual count
		percentFloat := float64(m.completedTests) / float64(m.totalTests)
		percent := int(percentFloat * 100)
		percentage = fmt.Sprintf("%3d%%", percent)
		testCount = fmt.Sprintf("%4d/%-4d %s", m.completedTests, m.totalTests, DurationStyle.Render("tests"))
	} else {
		// No estimate and no tests discovered yet
		percentage = "  0%"
		testCount = fmt.Sprintf("%-15s", DurationStyle.Render("discovering tests"))
	}

	// Format time and buffer with fixed widths for stability
	timeStr := fmt.Sprintf("%3d%s", elapsedSeconds, DurationStyle.Render("s"))
	bufferStr := fmt.Sprintf("%7.1f%s", bufferSizeKB, DurationStyle.Render("KB"))

	// Calculate the display width of all components except the progress bar
	// We need to account for ANSI color codes not contributing to display width
	spinWidth := getDisplayWidth(spin)
	infoWidth := getDisplayWidth(info)
	percentageWidth := getDisplayWidth(percentage)
	testCountWidth := getDisplayWidth(testCount)
	timeWidth := getDisplayWidth(timeStr)
	bufferWidth := getDisplayWidth(bufferStr)
	
	// Calculate total fixed width (including spaces)
	// spin + info + "  " + [progress] + " " + percentage + " " + testCount + "  " + time + " " + buffer
	fixedWidth := spinWidth + infoWidth + 2 + 1 + percentageWidth + 1 + testCountWidth + 2 + timeWidth + 1 + bufferWidth
	
	// Calculate available width for progress bar (with some padding)
	availableWidth := terminalWidth - fixedWidth - 2 // 2 chars padding for safety
	
	// Set minimum and maximum progress bar width
	const minProgressWidth = 20
	const maxProgressWidth = 100
	
	progressWidth := availableWidth
	if progressWidth < minProgressWidth {
		progressWidth = minProgressWidth
	} else if progressWidth > maxProgressWidth {
		progressWidth = maxProgressWidth
	}
	
	// Update progress bar width if it's different
	if m.progress.Width != progressWidth {
		m.progress.Width = progressWidth
	}
	
	prog := m.progress.View()

	// Assemble the complete status line with fixed spacing
	// All sections are now fixed-width, so no jumping should occur
	statusLine := spin + info + "  " + prog + " " + percentage + " " + testCount + "  " + timeStr + " " + bufferStr

	return statusLine + "\n"
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// shouldShowTest checks if a test should be displayed based on the show filter.
func (m *TestModel) shouldShowTest(status string) bool {
	switch m.showFilter {
	case "all":
		return true
	case "failed":
		return status == "fail"
	case "passed":
		return status == "pass"
	case "skipped":
		return status == "skip"
	case "collapsed":
		return true // In collapsed mode, we still check shouldShowTest but handle display differently
	case "none":
		return false // Show no test output, only final summary
	}
	return false // This should never be reached due to validation
}

// GetElapsedTime returns the total elapsed time for the test run.
func (m *TestModel) GetElapsedTime() time.Duration {
	return m.elapsedTime
}

// GetExitCode returns the appropriate exit code based on test results.
// GetExitCode returns the exit code from the test run.
func (m *TestModel) GetExitCode() int {
	if m.aborted {
		return 130 // Standard exit code for SIGINT (Ctrl+C)
	}
	if m.failCount > 0 {
		return 1
	}
	return 0
}

// IsAborted returns true if the test run was aborted by the user.
func (m *TestModel) IsAborted() bool {
	return m.aborted
}

// generateSubtestProgress creates a visual progress indicator for subtest results.
func (m *TestModel) generateSubtestProgress(passed, total int) string {
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
		indicator.WriteString(PassStyle.Render("●"))
	}

	// Add red dots for failed tests
	for i := 0; i < failedDots; i++ {
		indicator.WriteString(FailStyle.Render("●"))
	}

	return indicator.String()
}

func (m *TestModel) generateFinalSummary() string {
	// Check GitHub step summary environment
	_ = viper.BindEnv("GOTCHA_GITHUB_STEP_SUMMARY", "GITHUB_STEP_SUMMARY")
	githubSummary := viper.GetString("GOTCHA_GITHUB_STEP_SUMMARY")
	var summaryStatus string
	var summaryPath string

	if githubSummary != "" {
		summaryStatus = fmt.Sprintf("GitHub step summary written to %s", githubSummary)
	}

	// Calculate total tests and duration
	totalTests := m.passCount + m.failCount + m.skipCount
	elapsed := time.Since(m.startTime)
	m.elapsedTime = elapsed // Store for logging after TUI exits

	// Build the summary box
	border := strings.Repeat("─", 40)

	var output strings.Builder
	output.WriteString("\n\n")
	if summaryStatus != "" {
		output.WriteString(summaryStatus)
		output.WriteString("\n")
	}
	if summaryPath != "" {
		output.WriteString(summaryPath)
		output.WriteString("\n")
	}
	output.WriteString("\n")
	output.WriteString(border)
	output.WriteString("\n")
	output.WriteString("Test Summary:\n")
	output.WriteString(fmt.Sprintf("  %s Passed:  %d\n", PassStyle.Render(CheckPass), m.passCount))
	output.WriteString(fmt.Sprintf("  %s Failed:  %d\n", FailStyle.Render(CheckFail), m.failCount))
	output.WriteString(fmt.Sprintf("  %s Skipped: %d\n", SkipStyle.Render(CheckSkip), m.skipCount))
	output.WriteString(fmt.Sprintf("  Total:    %d tests\n", totalTests))
	output.WriteString(border)
	output.WriteString("\n")

	return output.String()
}

// getBufferSizeKB calculates the total size of all buffered package results in KB.
func (m *TestModel) getBufferSizeKB() float64 {
	totalBytes := 0

	// Calculate size of all package results and their output
	for _, pkg := range m.packageResults {
		// Package name and status
		totalBytes += len(pkg.Package) + 20 // package header overhead

		// Coverage info
		totalBytes += len(pkg.Coverage)

		// Package-level output
		for _, line := range pkg.Output {
			totalBytes += len(line)
		}

		// All test results
		for _, test := range pkg.Tests {
			totalBytes += len(test.Name) + len(test.Status) + 20 // test header overhead
			for _, output := range test.Output {
				totalBytes += len(output)
			}
			// Subtests
			for _, subtest := range test.Subtests {
				totalBytes += len(subtest.Name) + len(subtest.Status) + 20
				for _, output := range subtest.Output {
					totalBytes += len(output)
				}
			}
		}
	}

	// Also count data from active/incomplete packages
	for pkg, active := range m.activePackages {
		if active {
			totalBytes += len(pkg) + 100 // estimate for active package overhead
		}
	}

	return float64(totalBytes) / 1024.0
}

// emitAlert emits a terminal bell if enabled.
func emitAlert(enabled bool) {
	if enabled {
		fmt.Fprint(os.Stderr, "\a")
	}
}

// processEvent processes a test event and updates the model state.
func (m *TestModel) processEvent(event *types.TestEvent) {
	// Handle package-level events
	if event.Test == "" {
		switch event.Action {
		case "start":
			// New package starting
			if event.Package != "" {
				m.currentPackage = event.Package
				// Initialize package result
				if m.packageResults[event.Package] == nil {
					m.packageResults[event.Package] = &PackageResult{
						Package:   event.Package,
						StartTime: time.Now(),
						Status:    "running",
						Tests:     make(map[string]*TestResult),
						TestOrder: []string{},
						HasTests:  false,
					}
					m.activePackages[event.Package] = true
				}
			}
		case "output":
			// Check for coverage or "no test files" message
			if event.Package != "" {
				if strings.Contains(event.Output, "coverage:") {
					// Extract coverage information
					if strings.Contains(event.Output, "coverage: [no statements]") {
						// No statements to cover
						m.packageResults[event.Package].Coverage = "0.0%"
					} else if strings.Contains(event.Output, "coverage: [no test files]") {
						// No test files - shouldn't happen with actual tests
						m.packageResults[event.Package].Coverage = "0.0%"
					} else {
						// Extract percentage from normal coverage output
						if matches := strings.Fields(event.Output); len(matches) >= 2 {
							for i, field := range matches {
								if field == "coverage:" && i+1 < len(matches) {
									coverage := matches[i+1]
									// Remove any trailing characters
									if strings.HasSuffix(coverage, "%") {
										m.packageResults[event.Package].Coverage = coverage
									} else {
										// Handle edge cases
										m.packageResults[event.Package].Coverage = "0.0%"
									}
									break
								}
							}
						}
					}
				}
				if strings.Contains(event.Output, "[no test files]") {
					m.packagesWithNoTests[event.Package] = true
				}

				// Check for package-level FAIL in output (e.g., TestMain failures)
				// This catches "FAIL\tpackage.name\t0.123s" which go test outputs
				if strings.Contains(event.Output, "FAIL\t"+event.Package) {
					// Mark package as failed - it likely has tests that failed to run
					if pkg := m.packageResults[event.Package]; pkg != nil {
						// Don't override status if already set, but ensure we know tests exist
						if pkg.Status == "running" {
							pkg.Status = "fail"
						}
						pkg.HasTests = true // It has tests, they just failed to run
					}
				}

				// Buffer package-level output
				if pkg := m.packageResults[event.Package]; pkg != nil {
					pkg.Output = append(pkg.Output, event.Output)
				}
			}
		case "skip":
			// Package skipped (no tests to run)
			if event.Package != "" {
				if pkg := m.packageResults[event.Package]; pkg != nil {
					pkg.Status = "skip"
					pkg.EndTime = time.Now()
					pkg.Elapsed = event.Elapsed
					delete(m.activePackages, event.Package)
					if !contains(m.packageOrder, event.Package) {
						m.packageOrder = append(m.packageOrder, event.Package)
					}
				}
			}
		case "pass", "fail":
			// Package completed
			if event.Package != "" {
				if pkg := m.packageResults[event.Package]; pkg != nil {
					pkg.Status = event.Action
					pkg.EndTime = time.Now()
					pkg.Elapsed = event.Elapsed
					delete(m.activePackages, event.Package)
					if !contains(m.packageOrder, event.Package) {
						m.packageOrder = append(m.packageOrder, event.Package)
					}

					// If package failed with no tests recorded, it likely has tests that couldn't run
					// (e.g., TestMain failure, compilation error, etc.)
					if event.Action == "fail" && len(pkg.Tests) == 0 && !m.packagesWithNoTests[event.Package] {
						pkg.HasTests = true
					}
				}
			}
		}
		return
	}

	// Handle test-level events
	if event.Package != "" {
		pkg := m.packageResults[event.Package]
		if pkg == nil {
			// Create package if it doesn't exist (can happen with out-of-order events)
			pkg = &PackageResult{
				Package:   event.Package,
				StartTime: time.Now(),
				Status:    "running",
				Tests:     make(map[string]*TestResult),
				TestOrder: []string{},
				HasTests:  false,
			}
			m.packageResults[event.Package] = pkg
			m.activePackages[event.Package] = true
		}

		// Mark that this package has tests
		pkg.HasTests = true

		// Parse test hierarchy
		var parentTest string
		var isSubtest bool
		if strings.Contains(event.Test, "/") {
			parts := strings.SplitN(event.Test, "/", 2)
			parentTest = parts[0]
			isSubtest = true
		}

		switch event.Action {
		case "run":
			m.currentTest = event.Test
			// Count all tests including subtests for accurate progress
			// Always increment the actual test count
			m.actualTestCount++
			
			if !m.usingEstimate {
				// Not using estimate, update totalTests with actual count
				m.totalTests = m.actualTestCount
			}
			// If using estimate, keep totalTests as the estimate value

			if isSubtest {
				// This is a subtest
				parent := pkg.Tests[parentTest]
				if parent == nil {
					// Parent test might not exist yet due to parallel execution
					parent = &TestResult{
						Name:         parentTest,
						FullName:     parentTest,
						Status:       "running",
						Subtests:     make(map[string]*TestResult),
						SubtestOrder: []string{},
					}
					pkg.Tests[parentTest] = parent
				}

				subtest := &TestResult{
					Name:     event.Test,
					FullName: event.Test,
					Status:   "running",
					Parent:   parentTest,
				}
				parent.Subtests[event.Test] = subtest
				parent.SubtestOrder = append(parent.SubtestOrder, event.Test)
			} else {
				// Top-level test
				test := &TestResult{
					Name:         event.Test,
					FullName:     event.Test,
					Status:       "running",
					Subtests:     make(map[string]*TestResult),
					SubtestOrder: []string{},
				}
				pkg.Tests[event.Test] = test
				pkg.TestOrder = append(pkg.TestOrder, event.Test)
			}

		case "output":
			// Buffer the output
			if isSubtest {
				if parent := pkg.Tests[parentTest]; parent != nil {
					if subtest := parent.Subtests[event.Test]; subtest != nil {
						subtest.Output = append(subtest.Output, event.Output)
						// Capture skip reason if this is a skip output
						if strings.Contains(event.Output, "SKIP:") || strings.Contains(event.Output, "skipping:") {
							subtest.SkipReason = strings.TrimSpace(event.Output)
						}
					}
				}
			} else {
				if test := pkg.Tests[event.Test]; test != nil {
					test.Output = append(test.Output, event.Output)
					// Capture skip reason if this is a skip output
					if strings.Contains(event.Output, "SKIP:") || strings.Contains(event.Output, "skipping:") {
						test.SkipReason = strings.TrimSpace(event.Output)
					}
				}
			}

		case "pass", "fail", "skip":
			// Count all tests including subtests for accurate progress
			m.completedTests++
			
			// Check if we should switch from estimate to actual count
			if m.usingEstimate && m.actualTestCount > 0 {
				// Only switch from estimate to actual when we're confident:
				// 1. If actual count exceeds the estimate (estimate was too low)
				// 2. If we've completed a significant portion of the estimated tests
				if m.actualTestCount > m.estimatedTestCount {
					// Actual count exceeded estimate, switch to actual
					m.usingEstimate = false
					m.totalTests = m.actualTestCount
				} else if m.completedTests > int(float64(m.estimatedTestCount) * 0.9) {
					// We've completed 90% of estimated tests, likely near the end
					// Switch to actual count for accuracy
					m.usingEstimate = false
					m.totalTests = m.actualTestCount
				}
				// Otherwise keep showing the estimate to avoid jarring updates
			}

			// Update counts
			switch event.Action {
			case "pass":
				m.passCount++
			case "fail":
				m.failCount++
			case "skip":
				m.skipCount++
			}

			// Progress will be updated in Update method via streamOutputMsg

			// Update test result
			if isSubtest {
				if parent := pkg.Tests[parentTest]; parent != nil {
					if subtest := parent.Subtests[event.Test]; subtest != nil {
						subtest.Status = event.Action
						subtest.Elapsed = event.Elapsed
					}
					// Update parent subtest stats
					if m.subtestStats[parentTest] == nil {
						m.subtestStats[parentTest] = &SubtestStats{}
					}
					switch event.Action {
					case "pass":
						m.subtestStats[parentTest].passed = append(m.subtestStats[parentTest].passed, event.Test)
					case "fail":
						m.subtestStats[parentTest].failed = append(m.subtestStats[parentTest].failed, event.Test)
					case "skip":
						m.subtestStats[parentTest].skipped = append(m.subtestStats[parentTest].skipped, event.Test)
					}
				}
			} else {
				if test := pkg.Tests[event.Test]; test != nil {
					test.Status = event.Action
					test.Elapsed = event.Elapsed
					// Skip reason is already captured from output events
				}
			}

			// Progress is now updated in View() method to avoid duplication
		}
	}
}

// displayPackageResult generates the display output for a completed package.
func (m *TestModel) displayPackageResult(pkg *PackageResult) string {
	var output strings.Builder

	// Package header
	// Display package header - ▶ icon in white, package name in cyan
	output.WriteString(fmt.Sprintf("\n▶ %s\n\n", PackageHeaderStyle.Render(pkg.Package)))

	// Check for "No tests"
	// Check for package-level failures (e.g., TestMain failures)
	if pkg.Status == "fail" && len(pkg.Tests) == 0 {
		// Package failed without running any tests (likely TestMain failure)
		output.WriteString(fmt.Sprintf("  %s Package failed to run tests\n", FailStyle.Render(CheckFail)))

		// Display any package-level output (error messages)
		if len(pkg.Output) > 0 {
			for _, line := range pkg.Output {
				if strings.TrimSpace(line) != "" {
					output.WriteString(fmt.Sprintf("    %s", line))
				}
			}
		}
		return output.String()
	}

	if pkg.Status == "skip" || m.packagesWithNoTests[pkg.Package] || !pkg.HasTests {
		// Show more specific message if a filter is applied
		if m.testFilter != "" {
			output.WriteString(fmt.Sprintf("  %s\n", DurationStyle.Render("No tests matching filter")))
		} else {
			output.WriteString(fmt.Sprintf("  %s\n", DurationStyle.Render("No tests")))
		}
		return output.String()
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

	// Display tests in order
	for _, testName := range pkg.TestOrder {
		test := pkg.Tests[testName]
		if test == nil {
			continue
		}

		// Check if test has failed subtests (for --show=failed filter)
		hasFailedSubtests := false
		if m.showFilter == "failed" && len(test.Subtests) > 0 {
			for _, subtest := range test.Subtests {
				if subtest.Status == "fail" {
					hasFailedSubtests = true
					break
				}
			}
		}

		// Check if we should display this test based on filter
		if !m.shouldShowTest(test.Status) && !hasFailedSubtests && m.showFilter != "collapsed" {
			continue
		}

		// Display test result
		m.displayTest(&output, test)
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
			summaryLine = fmt.Sprintf("  %s %d tests failed, %d passed%s",
				FailStyle.Render(CheckFail),
				failedCount,
				passedCount,
				coverageStr)
		} else if passedCount > 0 {
			// All tests passed
			summaryLine = fmt.Sprintf("  %s All %d tests passed%s",
				PassStyle.Render(CheckPass),
				passedCount,
				coverageStr)
		} else if skippedCount > 0 {
			// Only skipped tests
			summaryLine = fmt.Sprintf("  %s %d tests skipped%s",
				SkipStyle.Render(CheckSkip),
				skippedCount,
				coverageStr)
		}

		if summaryLine != "" {
			output.WriteString(fmt.Sprintf("\n%s\n", summaryLine))
		}
	}

	return output.String()
}

// displayTest adds a test's display output to the builder.
func (m *TestModel) displayTest(output *strings.Builder, test *TestResult) {
	// Check if this test has subtests
	hasSubtests := len(test.Subtests) > 0

	// Build the test display
	var styledIcon string
	switch test.Status {
	case "pass":
		styledIcon = PassStyle.Render(CheckPass)
	case "fail":
		styledIcon = FailStyle.Render(CheckFail)
	case "skip":
		styledIcon = SkipStyle.Render(CheckSkip)
	default:
		return // Don't display running tests
	}

	// Display the test
	output.WriteString(fmt.Sprintf("  %s %s", styledIcon, TestNameStyle.Render(test.Name)))

	// Add duration if available
	if test.Elapsed > 0 {
		output.WriteString(fmt.Sprintf(" %s", DurationStyle.Render(fmt.Sprintf("(%.2fs)", test.Elapsed))))
	}

	// Add skip reason if available
	if test.Status == "skip" && test.SkipReason != "" {
		output.WriteString(fmt.Sprintf(" %s", DurationStyle.Render(fmt.Sprintf("[%s]", test.SkipReason))))
	}

	// Add subtest progress indicator if it has subtests
	if hasSubtests && m.subtestStats[test.Name] != nil {
		stats := m.subtestStats[test.Name]
		totalSubtests := len(stats.passed) + len(stats.failed) + len(stats.skipped)

		if totalSubtests > 0 {
			miniProgress := m.generateSubtestProgress(len(stats.passed), totalSubtests)
			percentage := (len(stats.passed) * 100) / totalSubtests
			output.WriteString(fmt.Sprintf(" %s %d%% passed", miniProgress, percentage))
		}
	}

	output.WriteString("\n")

	// Show test output for failed tests if not in collapsed mode
	if test.Status == "fail" && m.showFilter != "collapsed" && len(test.Output) > 0 {
		output.WriteString("\n")
		if m.verbosityLevel == "with-output" || m.verbosityLevel == "verbose" {
			// With full output, properly render tabs and maintain formatting
			for _, line := range test.Output {
				// Replace literal \t with actual tabs and \n with newlines
				formatted := strings.ReplaceAll(line, `\t`, "\t")
				formatted = strings.ReplaceAll(formatted, `\n`, "\n")
				output.WriteString("    " + formatted)
			}
		} else {
			// Default: show output as-is
			for _, line := range test.Output {
				output.WriteString("    " + line)
			}
		}
		output.WriteString("\n")
	}

	// Show detailed subtest results for failed parent tests
	if test.Status == "fail" && hasSubtests && m.showFilter != "collapsed" {
		stats := m.subtestStats[test.Name]
		if stats != nil {
			totalSubtests := len(stats.passed) + len(stats.failed) + len(stats.skipped)
			if totalSubtests > 0 {
				output.WriteString(fmt.Sprintf("\n    Subtest Summary: %d passed, %d failed of %d total\n",
					len(stats.passed), len(stats.failed), totalSubtests))

				// Show passed subtests
				if len(stats.passed) > 0 {
					output.WriteString(fmt.Sprintf("\n    %s Passed (%d):\n", PassStyle.Render("✔"), len(stats.passed)))
					for _, name := range stats.passed {
						// Extract just the subtest name, not the full path
						parts := strings.SplitN(name, "/", 2)
						subtestName := name
						if len(parts) > 1 {
							subtestName = parts[1]
						}
						output.WriteString(fmt.Sprintf("      • %s\n", subtestName))
					}
				}

				// Show failed subtests with their output
				if len(stats.failed) > 0 {
					output.WriteString(fmt.Sprintf("\n    %s Failed (%d):\n", FailStyle.Render("✘"), len(stats.failed)))
					for _, name := range stats.failed {
						// Extract just the subtest name
						parts := strings.SplitN(name, "/", 2)
						subtestName := name
						if len(parts) > 1 {
							subtestName = parts[1]
						}
						output.WriteString(fmt.Sprintf("      • %s\n", subtestName))

						// Show subtest output if available
						if subtest := test.Subtests[name]; subtest != nil && len(subtest.Output) > 0 {
							if m.verbosityLevel == "with-output" || m.verbosityLevel == "verbose" {
								// With full output, properly render tabs and maintain formatting
								for _, line := range subtest.Output {
									formatted := strings.ReplaceAll(line, `\t`, "\t")
									formatted = strings.ReplaceAll(formatted, `\n`, "\n")
									output.WriteString("        " + formatted)
								}
							} else {
								for _, line := range subtest.Output {
									output.WriteString("        " + line)
								}
							}
						}
					}
				}

				// Show skipped subtests if any
				if len(stats.skipped) > 0 {
					output.WriteString(fmt.Sprintf("\n    %s Skipped (%d):\n", SkipStyle.Render("⊘"), len(stats.skipped)))
					for _, name := range stats.skipped {
						parts := strings.SplitN(name, "/", 2)
						subtestName := name
						if len(parts) > 1 {
							subtestName = parts[1]
						}
						output.WriteString(fmt.Sprintf("      • %s\n", subtestName))
					}
				}
			}
		}
	}
}

// contains checks if a string is in a slice.
func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}
