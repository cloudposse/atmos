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
	"github.com/charmbracelet/log"
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

// TestModel represents the test UI model.
type TestModel struct {
	// Test tracking
	totalTests   int
	currentIndex int
	currentTest  string
	width        int
	height       int
	done         bool
	aborted      bool
	startTime    time.Time

	// UI components
	spinner  spinner.Model
	progress progress.Model

	// Test execution
	cmd        *exec.Cmd
	outputFile string
	showFilter string // "all", "failed", "passed", "skipped"
	alert      bool   // whether to emit terminal bell on completion

	// Results tracking
	passCount   int
	failCount   int
	skipCount   int
	testBuffers map[string][]string
	bufferMu    sync.Mutex

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
func NewTestModel(testPackages []string, testArgs, outputFile, coverProfile, showFilter string, totalTests int, alert bool) TestModel {
	// Create progress bar
	p := progress.New(
		progress.WithDefaultGradient(),
		progress.WithWidth(40),
		progress.WithoutPercentage(),
	)

	// Create spinner
	s := spinner.New()
	s.Style = spinnerStyle
	s.Spinner = spinner.Dot

	// Build go test command args
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
		cmd:         cmd,
		outputFile:  outputFile,
		showFilter:  showFilter,
		alert:       alert,
		spinner:     s,
		progress:    p,
		testBuffers: make(map[string][]string),
		totalTests:  totalTests,
		startTime:   time.Now(),
	}
}

func (m *TestModel) Init() tea.Cmd {
	return tea.Batch(
		m.startTestsCmd(),
		m.spinner.Tick,
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

		// Skip package-level events for most actions
		if event.Test == "" {
			return m, tea.Batch(m.readNextLine(), m.spinner.Tick) // Continue reading
		}

		// Convert to appropriate test message and continue streaming
		nextCmd := m.readNextLine()

		switch event.Action {
		case "run":
			m.currentTest = event.Test
			m.totalTests++
			m.bufferMu.Lock()
			m.testBuffers[event.Test] = []string{}
			m.bufferMu.Unlock()
			// Batch next command with spinner tick to keep UI updating
			return m, tea.Batch(nextCmd, m.spinner.Tick)

		case "output":
			// Buffer the output for potential error display
			m.bufferMu.Lock()
			if m.testBuffers[event.Test] != nil {
				m.testBuffers[event.Test] = append(m.testBuffers[event.Test], event.Output)
			}
			m.bufferMu.Unlock()
			// Batch next command with spinner tick to keep UI updating
			return m, tea.Batch(nextCmd, m.spinner.Tick)

		case "pass":
			m.passCount++
			m.currentIndex++

			// Update progress
			var progressCmd tea.Cmd
			if m.totalTests > 0 {
				progressCmd = m.progress.SetPercent(float64(m.currentIndex) / float64(m.totalTests))
			}

			var displayCmd tea.Cmd
			// Only show if filter allows it
			// Don't print passed tests immediately to avoid overwriting progress bar
			// The progress bar shows the current status
			if m.shouldShowTest("pass") && m.showFilter == "passed" {
				// Only show passed tests if explicitly requested
				output := fmt.Sprintf("%s %s %s\n",
					PassStyle.Render(CheckPass),
					TestNameStyle.Render(event.Test),
					DurationStyle.Render(fmt.Sprintf("(%.2fs)", event.Elapsed)))
				displayCmd = tea.Printf("%s", output)
			}

			// Clean up buffer
			m.bufferMu.Lock()
			delete(m.testBuffers, event.Test)
			m.bufferMu.Unlock()

			// Continue reading and optionally show progress/display
			cmds := []tea.Cmd{nextCmd, m.spinner.Tick}
			if progressCmd != nil {
				cmds = append(cmds, progressCmd)
			}
			if displayCmd != nil {
				cmds = append(cmds, displayCmd)
			}
			return m, tea.Batch(cmds...)

		case "fail":
			// Get buffered output for this test
			m.bufferMu.Lock()
			bufferedOutput := m.testBuffers[event.Test]
			output := make([]string, len(bufferedOutput))
			copy(output, bufferedOutput)
			m.bufferMu.Unlock()

			m.failCount++
			m.currentIndex++

			// Update progress
			var progressCmd tea.Cmd
			if m.totalTests > 0 {
				progressCmd = m.progress.SetPercent(float64(m.currentIndex) / float64(m.totalTests))
			}

			var displayCmd tea.Cmd
			// Only show if filter allows it
			if m.shouldShowTest("fail") {
				// Start with newline to separate from progress bar
				displayOutput := fmt.Sprintf("\n%s %s %s",
					FailStyle.Render(CheckFail),
					TestNameStyle.Render(event.Test),
					DurationStyle.Render(fmt.Sprintf("(%.2fs)", event.Elapsed)))

				// Add error details if present
				if len(output) > 0 {
					displayOutput += "\n\n"
					for _, line := range output {
						// Show all error output
						displayOutput += "    " + line
					}
				}
				// Single newline - test name line is the visual separator
				displayOutput += "\n"

				displayCmd = tea.Printf("%s", displayOutput)
			}

			// Clean up buffer
			m.bufferMu.Lock()
			delete(m.testBuffers, event.Test)
			m.bufferMu.Unlock()

			// Continue reading and optionally show progress/display
			cmds := []tea.Cmd{nextCmd, m.spinner.Tick}
			if progressCmd != nil {
				cmds = append(cmds, progressCmd)
			}
			if displayCmd != nil {
				cmds = append(cmds, displayCmd)
			}
			return m, tea.Batch(cmds...)

		case "skip":
			m.skipCount++
			m.currentIndex++

			// Update progress
			var progressCmd tea.Cmd
			if m.totalTests > 0 {
				progressCmd = m.progress.SetPercent(float64(m.currentIndex) / float64(m.totalTests))
			}

			var displayCmd tea.Cmd
			// Only show if filter allows it
			// Don't print skipped tests immediately to avoid overwriting progress bar
			if m.shouldShowTest("skip") && m.showFilter == "skipped" {
				// Only show skipped tests if explicitly requested
				output := fmt.Sprintf("%s %s\n",
					SkipStyle.Render(CheckSkip),
					TestNameStyle.Render(event.Test))
				displayCmd = tea.Printf("%s", output)
			}

			// Clean up buffer
			m.bufferMu.Lock()
			delete(m.testBuffers, event.Test)
			m.bufferMu.Unlock()

			// Continue reading and optionally show progress/display
			cmds := []tea.Cmd{nextCmd, m.spinner.Tick}
			if progressCmd != nil {
				cmds = append(cmds, progressCmd)
			}
			if displayCmd != nil {
				cmds = append(cmds, displayCmd)
			}
			return m, tea.Batch(cmds...)

		default:
			return m, nextCmd
		}

	case testsDoneMsg:
		m.done = true
		if m.jsonFile != nil {
			m.jsonFile.Close()
		}

		// Emit alert if enabled
		emitAlert(m.alert)

		// Don't show final summary if aborted
		if !m.aborted {
			// Generate the final summary output
			summaryOutput := m.generateFinalSummary()
			return m, tea.Sequence(
				tea.Printf("%s", summaryOutput),
				tea.Quit,
			)
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
		newModel, cmd := m.progress.Update(msg)
		if newModel, ok := newModel.(progress.Model); ok {
			m.progress = newModel
		}
		return m, cmd
	}

	return m, nil
}

func (m *TestModel) View() string {
	if m.done {
		return ""
	}

	// Build the status line components
	spin := m.spinner.View() + " "

	var info string
	if m.currentTest != "" {
		info = fmt.Sprintf("Running %s...", TestNameStyle.Render(m.currentTest))
	} else {
		info = "Starting tests..."
	}

	prog := m.progress.View()

	// Calculate elapsed time
	elapsed := time.Since(m.startTime)
	elapsedSeconds := int(elapsed.Seconds())

	// Calculate buffer size
	bufferSizeKB := m.getBufferSizeKB()

	var count string
	if m.totalTests > 0 {
		count = fmt.Sprintf(" %d/%d (%ds) %.1fKB", m.currentIndex, m.totalTests, elapsedSeconds, bufferSizeKB)
	} else {
		count = fmt.Sprintf(" %d (%ds) %.1fKB", m.currentIndex, elapsedSeconds, bufferSizeKB)
	}

	// Use our own terminal width detection instead of Bubble Tea's
	terminalWidth := getTerminalWidth()
	// Use display width to ignore ANSI color codes
	usedWidth := getDisplayWidth(spin) + getDisplayWidth(prog) + getDisplayWidth(count)
	availableWidth := max(0, terminalWidth-usedWidth)

	// Truncate info if necessary, but always show something
	if getDisplayWidth(info) > availableWidth {
		if availableWidth > 10 {
			info = info[:availableWidth-3] + "..."
		} else if availableWidth > 3 {
			// Show abbreviated version
			info = "Run..."
		} else {
			// Very narrow terminal, show minimal info
			info = "..."
		}
	}

	// Calculate gap using our terminal width
	gap := ""
	if terminalWidth > 0 {
		gapWidth := max(0, terminalWidth-getDisplayWidth(spin)-getDisplayWidth(info)-getDisplayWidth(prog)-getDisplayWidth(count))
		gap = strings.Repeat(" ", gapWidth)
	}

	return spin + info + gap + prog + count + "\n"
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

// generateFinalSummary creates the formatted final summary output.
func (m *TestModel) generateFinalSummary() string {
	// Check GitHub step summary environment
	_ = viper.BindEnv("GOTCHA_GITHUB_STEP_SUMMARY", "GITHUB_STEP_SUMMARY")
	githubSummary := viper.GetString("GOTCHA_GITHUB_STEP_SUMMARY")
	var summaryStatus string
	var summaryPath string

	if githubSummary == "" {
		// Log the skipped status using the logger
		logger := log.New(os.Stderr)
		logger.Info("GITHUB_STEP_SUMMARY not set (skipped)")
		summaryStatus = "" // Don't include in the summary output string
	} else {
		summaryStatus = fmt.Sprintf("GitHub step summary written to %s", githubSummary)
	}

	// Calculate total tests and duration (approximate)
	totalTests := m.passCount + m.failCount + m.skipCount

	// Build the summary box
	border := strings.Repeat("─", 40)

	var output strings.Builder
	output.WriteString("\n")
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

// getBufferSizeKB calculates the total size of all test buffers in KB.
func (m *TestModel) getBufferSizeKB() float64 {
	m.bufferMu.Lock()
	defer m.bufferMu.Unlock()

	totalBytes := 0
	for _, lines := range m.testBuffers {
		for _, line := range lines {
			totalBytes += len(line) + 1 // +1 for newline character
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
