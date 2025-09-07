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

	"github.com/charmbracelet/bubbles/progress"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/viper"

	"github.com/cloudposse/atmos/tools/gotcha/pkg/types"
)

// NewTestModel creates a new test model for the TUI.
func NewTestModel(testPackages []string, testArgs, outputFile, coverProfile, showFilter string, alert bool, verbosityLevel string, estimatedTestCount int) TestModel {
	// Create spinner with custom style
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = spinnerStyle

	// Create progress bar with custom style
	p := progress.New(
		progress.WithDefaultGradient(),
		progress.WithoutPercentage(),
	)

	// Extract test filter from args if present
	var testFilter string
	args := strings.Fields(testArgs)
	for i := 0; i < len(args)-1; i++ {
		if args[i] == "-run" {
			testFilter = args[i+1]
			break
		}
	}

	// Set initial estimate information
	var usingEstimate bool
	var totalTests int
	if estimatedTestCount > 0 {
		usingEstimate = true
		totalTests = estimatedTestCount
	}

	return TestModel{
		testPackages:   testPackages,
		testArgs:       testArgs,
		buffers:        make(map[string][]string),
		subtestStats:   make(map[string]*SubtestStats),
		packageResults: make(map[string]*PackageResult),
		packageOrder:   []string{},
		activePackages: make(map[string]bool),
		spinner:        s,
		progress:       p,
		outputFile:     outputFile,
		coverProfile:   coverProfile,
		showFilter:     showFilter,
		verbosityLevel: verbosityLevel,
		testFilter:     testFilter,
		alert:          alert,
		startTime:      time.Now(),
		jsonWriter:     &sync.Mutex{},

		// Legacy compatibility
		packagesWithNoTests:   make(map[string]bool),
		packageHasTests:       make(map[string]bool),
		packageNoTestsPrinted: make(map[string]bool),

		// Estimate handling
		estimatedTotal:     estimatedTestCount,
		estimatedTestCount: estimatedTestCount,
		usingEstimate:      usingEstimate,
		totalTests:         totalTests,
	}
}

// Init initializes the TUI model.
func (m *TestModel) Init() tea.Cmd {
	return tea.Batch(
		m.spinner.Tick,
		m.startTestsCmd(),
		tea.EnterAltScreen,
	)
}

// startTestsCmd starts the test process.
func (m *TestModel) startTestsCmd() tea.Cmd {
	return func() tea.Msg {
		// Build the go test command
		args := []string{"test", "-json"}

		// Add coverage if requested
		if m.coverProfile != "" {
			args = append(args, fmt.Sprintf("-coverprofile=%s", m.coverProfile))
		}

		// Add verbose flag
		args = append(args, "-v")

		// Add test arguments
		if m.testArgs != "" {
			testArgsList := strings.Fields(m.testArgs)
			args = append(args, testArgsList...)
		}

		// Add packages
		args = append(args, m.testPackages...)

		// Create the command
		cmd := exec.Command("go", args...)
		stdout, err := cmd.StdoutPipe()
		if err != nil {
			return testFailMsg{test: "setup", pkg: "error: " + err.Error()}
		}

		// Pass through stderr to console
		cmd.Stderr = os.Stderr

		// Start the command
		if err := cmd.Start(); err != nil {
			return testFailMsg{test: "start", pkg: "error: " + err.Error()}
		}

		// Store the process for later use
		m.testCmd = cmd.Process

		return subprocessReadyMsg{proc: stdout}
	}
}

// readNextLine reads the next line from the test output.
func (m *TestModel) readNextLine() tea.Cmd {
	return func() tea.Msg {
		if m.scanner == nil {
			return nil
		}

		reader := bufio.NewReader(m.scanner)
		line, err := reader.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				// Check if the process exited with an error
				if m.testCmd != nil {
					// Wait for the process to finish and get exit code
					if state, err := m.testCmd.Wait(); err == nil {
						m.exitCode = state.ExitCode()
					}
				}
				return testCompleteMsg{exitCode: m.exitCode}
			}
			return testFailMsg{test: "read", pkg: "error: " + err.Error()}
		}

		return streamOutputMsg{line: strings.TrimRight(line, "\n")}
	}
}

// Update handles messages and updates the model.
// Update processes incoming messages and updates the model state.
// Refactored to use message handlers to reduce complexity.
func (m *TestModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		return m.handleKeyMsg(msg)

	case tea.WindowSizeMsg:
		m.handleWindowSizeMsg(msg)
		return m, nil

	case subprocessReadyMsg:
		return m, m.handleSubprocessReady(msg)

	case streamOutputMsg:
		return m, m.handleStreamOutput(msg)

	case testFailMsg:
		return m, m.handleTestFail()

	case testCompleteMsg:
		return m, m.handleTestComplete(msg)

	case spinner.TickMsg:
		return m, m.handleSpinnerTick(msg)

	case progress.FrameMsg:
		return m, m.handleProgressFrame(msg)

	default:
		return m, nil
	}
}

// Update processes incoming messages and updates the model - OLD VERSION.
// TODO: Remove this after verifying the refactored version works.
func (m *TestModel) UpdateOld(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c", "esc":
			m.aborted = true
			if m.testCmd != nil {
				_ = m.testCmd.Kill()
			}
			if m.jsonFile != nil {
				_ = m.jsonFile.Close()
			}
			return m, tea.Quit

		case "up", "k":
			if m.scrollOffset > 0 {
				m.scrollOffset--
			}

		case "down", "j":
			if m.scrollOffset < m.maxScroll {
				m.scrollOffset++
			}

		case "pgup":
			m.scrollOffset -= 10
			if m.scrollOffset < 0 {
				m.scrollOffset = 0
			}

		case "pgdn":
			m.scrollOffset += 10
			if m.scrollOffset > m.maxScroll {
				m.scrollOffset = m.maxScroll
			}

		case "home":
			m.scrollOffset = 0

		case "end":
			m.scrollOffset = m.maxScroll
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.progress.Width = msg.Width - 4
		if m.width < 1 {
			m.width = 1
		}
		if m.height < 1 {
			m.height = 1
		}

	case subprocessReadyMsg:
		m.scanner = msg.proc

		// Open JSON file for writing
		jsonFile, err := os.Create(m.outputFile)
		if err == nil {
			m.jsonFile = jsonFile
		}

		// Start reading lines
		cmds = append(cmds, m.readNextLine())

	case streamOutputMsg:
		// Write to JSON file if open
		if m.jsonFile != nil {
			m.jsonWriter.Lock()
			_, _ = m.jsonFile.Write([]byte(msg.line))
			_, _ = m.jsonFile.Write([]byte("\n"))
			m.jsonWriter.Unlock()
		}

		// Parse JSON event
		var event types.TestEvent
		if err := json.Unmarshal([]byte(msg.line), &event); err == nil {
			m.processEvent(&event)

			// Update progress based on test completion
			if m.totalTests > 0 {
				progress := float64(m.completedTests) / float64(m.totalTests)
				if progress > 1.0 {
					progress = 1.0
				}
				cmd := m.progress.SetPercent(progress)
				if cmd != nil {
					cmds = append(cmds, cmd)
				}
			}
		}

		// Continue reading
		cmds = append(cmds, m.readNextLine())

	case testFailMsg:
		// A test has failed
		m.failed++
		if !m.done {
			cmds = append(cmds, m.readNextLine())
		}

	case testCompleteMsg:
		m.done = true
		m.endTime = time.Now()
		m.exitCode = msg.exitCode

		// Close JSON file
		if m.jsonFile != nil {
			_ = m.jsonFile.Close()
		}

		// Emit alert if requested
		if m.alert {
			emitAlert(true)
		}

		// Force 100% progress on completion
		if m.totalTests > 0 {
			cmd := m.progress.SetPercent(1.0)
			if cmd != nil {
				cmds = append(cmds, cmd)
			}
		}

		// Auto-quit after a brief delay to show final state
		cmds = append(cmds, tea.Tick(2*time.Second, func(t time.Time) tea.Msg {
			return tea.Quit()
		}))

	case spinner.TickMsg:
		if !m.done {
			var cmd tea.Cmd
			m.spinner, cmd = m.spinner.Update(msg)
			cmds = append(cmds, cmd)
		}

	case progress.FrameMsg:
		progressModel, cmd := m.progress.Update(msg)
		m.progress = progressModel.(progress.Model)
		cmds = append(cmds, cmd)
	}

	return m, tea.Batch(cmds...)
}

// View renders the TUI.
func (m *TestModel) View() string {
	if m.width == 0 || m.height == 0 {
		return ""
	}

	var output strings.Builder

	// Header
	output.WriteString(StatsHeaderStyle.Render("🧪 Go Test Runner"))
	output.WriteString("\n\n")

	// Show spinner and current status
	if !m.done {
		output.WriteString(m.spinner.View())
		output.WriteString(" Running tests...\n")

		// Show progress bar
		if m.totalTests > 0 {
			progressText := fmt.Sprintf(" %d/%d tests completed", m.completedTests, m.totalTests)
			if m.usingEstimate {
				progressText += " (estimated)"
			}
			output.WriteString(m.progress.View())
			output.WriteString(progressText)
			output.WriteString("\n")
		}

		// Show current package/test
		if m.currentPackage != "" {
			output.WriteString(fmt.Sprintf("\nPackage: %s\n", DurationStyle.Render(m.currentPackage)))
		}
		if m.currentTest != "" {
			output.WriteString(fmt.Sprintf("Test: %s\n", DurationStyle.Render(m.currentTest)))
		}
	} else {
		// Show completion status
		if m.exitCode == 0 {
			output.WriteString(PassStyle.Render("✅ Tests completed successfully!\n"))
		} else {
			output.WriteString(FailStyle.Render(fmt.Sprintf("❌ Tests failed with exit code %d\n", m.exitCode)))
		}
	}

	// Show test statistics
	output.WriteString(fmt.Sprintf("\n%s %s | %s %s | %s %s\n",
		PassStyle.Render("✓"), fmt.Sprintf("%d passed", m.passed),
		FailStyle.Render("✗"), fmt.Sprintf("%d failed", m.failed),
		SkipStyle.Render("○"), fmt.Sprintf("%d skipped", m.skipped),
	))

	// Show package results
	if len(m.packageOrder) > 0 {
		output.WriteString("\n" + StatsHeaderStyle.Render("Package Results:") + "\n")

		// Build the full output for all packages
		var fullOutput strings.Builder
		for _, pkgName := range m.packageOrder {
			if pkg := m.packageResults[pkgName]; pkg != nil {
				fullOutput.WriteString(m.displayPackageResult(pkg))
			}
		}

		// Apply scrolling if needed
		lines := strings.Split(fullOutput.String(), "\n")
		availableHeight := m.height - strings.Count(output.String(), "\n") - 5 // Reserve space for footer

		if len(lines) > availableHeight {
			m.maxScroll = len(lines) - availableHeight
			if m.scrollOffset > m.maxScroll {
				m.scrollOffset = m.maxScroll
			}

			// Show scrolled content
			endLine := m.scrollOffset + availableHeight
			if endLine > len(lines) {
				endLine = len(lines)
			}

			for i := m.scrollOffset; i < endLine; i++ {
				output.WriteString(lines[i])
				if i < endLine-1 {
					output.WriteString("\n")
				}
			}

			// Show scroll indicator
			output.WriteString(fmt.Sprintf("\n\n%s (Line %d-%d of %d)",
				DurationStyle.Render("↑↓ to scroll"),
				m.scrollOffset+1,
				endLine,
				len(lines),
			))
		} else {
			// Show all content
			output.WriteString(fullOutput.String())
		}
	}

	// Show final summary if done
	if m.done {
		output.WriteString(m.generateFinalSummary())
	}

	// Footer with controls
	if !m.done {
		output.WriteString(fmt.Sprintf("\n\n%s", DurationStyle.Render("Press q to quit, ↑↓ to scroll")))
	} else {
		elapsed := m.endTime.Sub(m.startTime)
		output.WriteString(fmt.Sprintf("\n\nCompleted in %s", DurationStyle.Render(elapsed.Round(time.Millisecond).String())))
	}

	// Memory usage indicator (debug)
	if viper.GetBool("debug.show_memory") {
		bufferSize := m.getBufferSizeKB()
		output.WriteString(fmt.Sprintf("\n%s", DurationStyle.Render(fmt.Sprintf("Buffer: %.1f KB", bufferSize))))
	}

	return output.String()
}

// GetElapsedTime returns the elapsed time since the test started.
func (m *TestModel) GetElapsedTime() time.Duration {
	if m.done {
		return m.endTime.Sub(m.startTime)
	}
	return time.Since(m.startTime)
}

// GetExitCode returns the exit code from the test process.
func (m *TestModel) GetExitCode() int {
	// If aborted, return special exit code
	if m.aborted {
		return 130 // Standard exit code for SIGINT
	}
	// Return the actual exit code from the test process
	return m.exitCode
}

// IsAborted returns true if the test was aborted by the user.
func (m *TestModel) IsAborted() bool {
	return m.aborted
}
