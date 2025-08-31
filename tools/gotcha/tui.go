package main

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

	"github.com/charmbracelet/bubbles/progress"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
)

type testModel struct {
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

// Bubble Tea messages
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

// Message for when subprocess is initialized and ready to stream
type subprocessReadyMsg struct {
	scanner  *bufio.Scanner
	stdout   io.ReadCloser
	jsonFile *os.File
}

// Message for streaming subprocess output
type streamOutputMsg struct {
	line []byte
}

func newTestModel(testPackages []string, testArgs, outputFile, coverProfile, showFilter string, totalTests int) testModel {
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

	return testModel{
		cmd:         cmd,
		outputFile:  outputFile,
		showFilter:  showFilter,
		spinner:     s,
		progress:    p,
		testBuffers: make(map[string][]string),
		totalTests:  totalTests,
		startTime:   time.Now(),
	}
}

func (m testModel) Init() tea.Cmd {
	return tea.Batch(
		m.startTestsCmd(),
		m.spinner.Tick,
	)
}

// startTestsCmd initializes and starts the test subprocess
func (m testModel) startTestsCmd() tea.Cmd {
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

// readNextLine creates a command that reads one line using the persistent scanner
func (m testModel) readNextLine() tea.Cmd {
	return func() tea.Msg {
		// Use the scanner from the model - this persists across reads
		if m.scanner != nil && m.scanner.Scan() {
			line := m.scanner.Bytes()

			// Write to JSON file
			if m.jsonFile != nil {
				m.jsonFile.Write(line)
				m.jsonFile.Write([]byte("\n"))
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

func (m testModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "esc", "q":
			m.aborted = true
			m.done = true
			if m.cmd != nil && m.cmd.Process != nil {
				m.cmd.Process.Kill()
			}
			if m.jsonFile != nil {
				m.jsonFile.Close()
			}
			return m, tea.Sequence(
				tea.Printf("\n%s Tests aborted", failStyle.Render(checkFail)),
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
		var event TestEvent
		if err := json.Unmarshal(msg.line, &event); err != nil {
			// Skip non-JSON lines, continue reading
			return m, tea.Batch(m.readNextLine(), m.spinner.Tick)
		}

		// Skip package-level events for most actions
		if event.Test == "" {
			return m, tea.Batch(m.readNextLine(), m.spinner.Tick) // Continue reading
		}

		// Convert to appropriate test message and continue streaming
		var nextCmd tea.Cmd = m.readNextLine()

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
			if m.shouldShowTest("pass") {
				output := fmt.Sprintf("%s %s %s",
					passStyle.Render(checkPass),
					testNameStyle.Render(event.Test),
					durationStyle.Render(fmt.Sprintf("(%.2fs)", event.Elapsed)))
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
				displayOutput := fmt.Sprintf("%s %s %s",
					failStyle.Render(checkFail),
					testNameStyle.Render(event.Test),
					durationStyle.Render(fmt.Sprintf("(%.2fs)", event.Elapsed)))

				// Add error details if present
				if len(output) > 0 {
					displayOutput += "\n\n"
					for _, line := range output {
						if shouldShowErrorLine(line) {
							displayOutput += "    " + line
						}
					}
					displayOutput += "\n"
				}

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
			if m.shouldShowTest("skip") {
				output := fmt.Sprintf("%s %s",
					skipStyle.Render(checkSkip),
					testNameStyle.Render(event.Test))
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
		return m, tea.Sequence(
			tea.Printf("%s Error: %v", failStyle.Render(checkFail), msg.err),
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

func (m testModel) View() string {
	if m.done {
		return ""
	}

	// Build the status line components
	spin := m.spinner.View() + " "

	var info string
	if m.currentTest != "" {
		info = fmt.Sprintf("Running %s...", testNameStyle.Render(m.currentTest))
	} else {
		info = "Starting tests..."
	}

	prog := m.progress.View()

	// Calculate elapsed time
	elapsed := time.Since(m.startTime)
	elapsedSeconds := int(elapsed.Seconds())

	var count string
	if m.totalTests > 0 {
		count = fmt.Sprintf(" %d/%d      (%ds)", m.currentIndex, m.totalTests, elapsedSeconds)
	} else {
		count = fmt.Sprintf(" %d      (%ds)", m.currentIndex, elapsedSeconds)
	}

	// Calculate available space for the info section
	usedWidth := len(spin) + len(prog) + len(count)
	availableWidth := max(0, m.width-usedWidth)

	// Truncate info if necessary
	if len(info) > availableWidth {
		if availableWidth > 3 {
			info = info[:availableWidth-3] + "..."
		} else {
			info = ""
		}
	}

	// Calculate gap
	gap := ""
	if m.width > 0 {
		gapWidth := max(0, m.width-len(spin)-len(info)-len(prog)-len(count))
		gap = strings.Repeat(" ", gapWidth)
	}

	return spin + info + gap + prog + count
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// shouldShowTest checks if a test should be displayed based on the show filter
func (m testModel) shouldShowTest(status string) bool {
	switch m.showFilter {
	case "all":
		return true
	case "failed":
		return status == "fail"
	case "passed":
		return status == "pass"
	case "skipped":
		return status == "skip"
	}
	return false // This should never be reached due to validation
}

// GetExitCode returns the appropriate exit code based on test results.
func (m testModel) GetExitCode() int {
	if m.aborted {
		return 130 // Standard exit code for SIGINT (Ctrl+C)
	}
	if m.failCount > 0 {
		return 1
	}
	return 0
}

// generateFinalSummary creates the formatted final summary output
func (m testModel) generateFinalSummary() string {
	// Check GitHub step summary environment
	githubSummary := os.Getenv("GITHUB_STEP_SUMMARY")
	var summaryStatus string
	var summaryPath string

	if githubSummary == "" {
		summaryStatus = "- GITHUB_STEP_SUMMARY not set (skipped)."
	} else {
		summaryStatus = fmt.Sprintf("Output GitHub step summary to %s", githubSummary)
	}

	// Check for markdown summary file
	if _, err := os.Stat("test-summary.md"); err == nil {
		summaryPath = fmt.Sprintf("%s Output markdown summary to test-summary.md", passStyle.Render(checkPass))
	}

	// Calculate total tests and duration (approximate)
	totalTests := m.passCount + m.failCount + m.skipCount

	// Build the summary box
	border := strings.Repeat("─", 40)

	var output strings.Builder
	output.WriteString("\n")
	output.WriteString(summaryStatus)
	output.WriteString("\n")
	if summaryPath != "" {
		output.WriteString(summaryPath)
		output.WriteString("\n")
	}
	output.WriteString("\n")
	output.WriteString(border)
	output.WriteString("\n")
	output.WriteString("Test Summary:\n")
	output.WriteString(fmt.Sprintf("  %s Passed:  %d\n", passStyle.Render(checkPass), m.passCount))
	output.WriteString(fmt.Sprintf("  %s Failed:  %d\n", failStyle.Render(checkFail), m.failCount))
	output.WriteString(fmt.Sprintf("  %s Skipped: %d\n", skipStyle.Render(checkSkip), m.skipCount))
	output.WriteString(fmt.Sprintf("  Total:    %d tests\n", totalTests))
	output.WriteString(border)
	output.WriteString("\n")

	return output.String()
}
