package stream

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/progress"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// TUIRunner runs tests with a progress bar using StreamProcessor.
type TUIRunner struct {
	testPackages   []string
	testArgs       string
	outputFile     string
	coverProfile   string
	showFilter     string
	verbosityLevel string
	estimatedTotal int
	alert          bool

	// Bubble Tea components
	spinner  spinner.Model
	progress progress.Model

	// State
	startTime      time.Time
	completedTests int
	totalTests     int
	activePackage  string
	finalOutput    string
	exitCode       int
	done           bool
	aborted        bool
}

// NewTUIRunner creates a new TUI runner.
func NewTUIRunner(testPackages []string, testArgs, outputFile, coverProfile, showFilter string, alert bool, verbosityLevel string, estimatedTotal int) *TUIRunner {
	// Create spinner with custom style
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("14"))

	// Create progress bar with custom style
	p := progress.New(
		progress.WithDefaultGradient(),
		progress.WithoutPercentage(),
	)

	return &TUIRunner{
		testPackages:   testPackages,
		testArgs:       testArgs,
		outputFile:     outputFile,
		coverProfile:   coverProfile,
		showFilter:     showFilter,
		verbosityLevel: verbosityLevel,
		estimatedTotal: estimatedTotal,
		alert:          alert,
		spinner:        s,
		progress:       p,
		startTime:      time.Now(),
		totalTests:     estimatedTotal, // Start with estimate
	}
}

// Init initializes the TUI.
func (r *TUIRunner) Init() tea.Cmd {
	return tea.Batch(
		r.spinner.Tick,
		tickCmd(), // Start polling for progress updates
		r.runTests(),
	)
}

// runTests starts the test process using StreamProcessor.
func (r *TUIRunner) runTests() tea.Cmd {
	return func() tea.Msg {
		// Build the go test command
		args := []string{"test", "-json"}

		// Add coverage if requested
		if r.coverProfile != "" {
			args = append(args, fmt.Sprintf("-coverprofile=%s", r.coverProfile))
		}

		// Add verbose flag
		args = append(args, "-v")

		// Add test arguments
		if r.testArgs != "" {
			testArgsList := strings.Fields(r.testArgs)
			args = append(args, testArgsList...)
		}

		// Add packages
		args = append(args, r.testPackages...)

		// Create the command
		cmd := exec.Command("go", args...)
		stdout, err := cmd.StdoutPipe()
		if err != nil {
			return testErrorMsg{err: err}
		}

		// Pass through stderr to console
		cmd.Stderr = os.Stderr

		// Start the command
		if err := cmd.Start(); err != nil {
			return testErrorMsg{err: err}
		}

		// Create output file for JSON
		jsonFile, err := os.Create(r.outputFile)
		if err != nil {
			return testErrorMsg{err: err}
		}
		defer jsonFile.Close()

		// Extract test filter from args
		var testFilter string
		for i := 0; i < len(args)-1; i++ {
			if args[i] == "-run" {
				testFilter = args[i+1]
				break
			}
		}

		// Create progress reporter
		progressReporter := NewProgressReporter(r.showFilter, testFilter, r.verbosityLevel)

		// Set callback to send progress updates
		progressReporter.SetProgressCallback(func(completed, total int, activePackage string, elapsed time.Duration) {
			// This will be called from the processor's goroutine
			// We can't send tea.Msg from here directly, so we'll poll instead
			r.completedTests = completed
			if total > 0 {
				r.totalTests = total
			}
			r.activePackage = activePackage
		})

		// Create stream processor with progress reporter
		processor := NewStreamProcessorWithReporter(jsonFile, progressReporter)

		// Process the stream
		processErr := processor.ProcessStream(stdout)

		// Wait for command to complete
		testErr := cmd.Wait()

		// Get final summary
		processor.PrintSummary()

		// Don't store final output - StreamProcessor already printed it
		// The ProgressReporter's Finalize returns empty string anyway

		// Determine exit code
		if processErr != nil {
			r.exitCode = 1
		} else if testErr != nil {
			if exitErr, ok := testErr.(*exec.ExitError); ok {
				r.exitCode = exitErr.ExitCode()
			} else {
				r.exitCode = 1
			}
		}

		return testCompleteMsg{exitCode: r.exitCode}
	}
}

// Update handles messages.
func (r *TUIRunner) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "esc", "ctrl+c":
			r.aborted = true
			r.done = true
			return r, tea.Quit
		}

	case spinner.TickMsg:
		if !r.done {
			var cmd tea.Cmd
			r.spinner, cmd = r.spinner.Update(msg)
			return r, cmd
		}

	case progress.FrameMsg:
		if !r.done {
			progressModel, cmd := r.progress.Update(msg)
			r.progress = progressModel.(progress.Model)
			return r, cmd
		}

	case testCompleteMsg:
		r.exitCode = msg.exitCode
		r.done = true
		return r, tea.Quit

	case testErrorMsg:
		r.exitCode = 1
		r.done = true
		fmt.Fprintf(os.Stderr, "Error: %v\n", msg.err)
		return r, tea.Quit

	case tickMsg:
		// Poll for progress updates
		if !r.done {
			return r, tickCmd()
		}
	}

	return r, nil
}

// View renders the TUI.
func (r *TUIRunner) View() string {
	if r.done {
		if r.aborted {
			return "\n✗ Test run aborted\n"
		}
		// Clear the progress line
		return "\n"
	}

	// Calculate progress
	var progressPercent float64
	if r.totalTests > 0 {
		progressPercent = float64(r.completedTests) / float64(r.totalTests)
		// Ensure we don't exceed 100%
		if progressPercent > 1.0 {
			progressPercent = 1.0
		}
	}

	// Update progress bar percentage
	r.progress.SetPercent(progressPercent)

	// Build status line components (everything on one line)
	spin := r.spinner.View() + " "

	// Status text
	var info string
	if r.activePackage != "" {
		pkgName := r.activePackage
		const maxWidth = 50
		if len(pkgName) > maxWidth {
			pkgName = pkgName[:maxWidth-3] + "..."
		}
		info = fmt.Sprintf("Testing %-*s", maxWidth, pkgName)
	} else {
		info = fmt.Sprintf("%-*s", 57, "Starting tests...")
	}

	// Progress bar
	prog := r.progress.View()

	// Percentage and count
	var percentage string
	var testCount string
	if r.totalTests > 0 {
		percentage = fmt.Sprintf("%3.0f%%", progressPercent*100)
		testCount = fmt.Sprintf("(%d/%d)", r.completedTests, r.totalTests)
	} else {
		percentage = "   "
		testCount = fmt.Sprintf("(%d)", r.completedTests)
	}

	// Elapsed time
	elapsed := time.Since(r.startTime)
	timeStr := fmt.Sprintf("[%.1fs]", elapsed.Seconds())

	// Assemble complete status line
	statusLine := spin + info + "  " + prog + " " + percentage + " " + testCount + "  " + timeStr

	return statusLine + "\n"
}

// GetExitCode returns the exit code.
func (r *TUIRunner) GetExitCode() int {
	return r.exitCode
}

// Message types
type testCompleteMsg struct {
	exitCode int
}

type testErrorMsg struct {
	err error
}

type tickMsg struct{}

func tickCmd() tea.Cmd {
	return tea.Tick(100*time.Millisecond, func(time.Time) tea.Msg {
		return tickMsg{}
	})
}
