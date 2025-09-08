package tui

import (
	"encoding/json"
	"os"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/bubbles/progress"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/cloudposse/atmos/tools/gotcha/pkg/types"
)

// handleKeyMsg processes keyboard input messages.
func (m *TestModel) handleKeyMsg(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q", "ctrl+c", "esc":
		return m.handleQuit()
	case "up", "k":
		m.handleScrollUp()
	case "down", "j":
		m.handleScrollDown()
	case "pgup":
		m.handlePageUp()
	case "pgdn":
		m.handlePageDown()
	case "home":
		m.scrollOffset = 0
	case "end":
		m.scrollOffset = m.maxScroll
	}
	return m, nil
}

// handleQuit handles quit operations.
func (m *TestModel) handleQuit() (tea.Model, tea.Cmd) {
	m.aborted = true
	if m.testCmd != nil {
		_ = m.testCmd.Kill()
	}
	if m.jsonFile != nil {
		_ = m.jsonFile.Close()
	}
	return m, tea.Quit
}

// handleScrollUp scrolls the view up by one line.
func (m *TestModel) handleScrollUp() {
	if m.scrollOffset > 0 {
		m.scrollOffset--
	}
}

// handleScrollDown scrolls the view down by one line.
func (m *TestModel) handleScrollDown() {
	if m.scrollOffset < m.maxScroll {
		m.scrollOffset++
	}
}

// handlePageUp scrolls the view up by 10 lines.
func (m *TestModel) handlePageUp() {
	m.scrollOffset -= 10
	if m.scrollOffset < 0 {
		m.scrollOffset = 0
	}
}

// handlePageDown scrolls the view down by 10 lines.
func (m *TestModel) handlePageDown() {
	m.scrollOffset += 10
	if m.scrollOffset > m.maxScroll {
		m.scrollOffset = m.maxScroll
	}
}

// handleWindowSizeMsg processes window resize messages.
func (m *TestModel) handleWindowSizeMsg(msg tea.WindowSizeMsg) {
	m.width = msg.Width
	m.height = msg.Height
	m.progress.Width = msg.Width - 4
	if m.width < 1 {
		m.width = 1
	}
	if m.height < 1 {
		m.height = 1
	}
}

// handleSubprocessReady handles subprocess initialization.
func (m *TestModel) handleSubprocessReady(msg subprocessReadyMsg) tea.Cmd {
	m.scanner = msg.proc

	// Open JSON file for writing
	jsonFile, err := os.Create(m.outputFile)
	if err == nil {
		m.jsonFile = jsonFile
	}

	// Start reading lines
	return m.readNextLine()
}

// handleStreamOutput processes streaming test output.
func (m *TestModel) handleStreamOutput(msg streamOutputMsg) tea.Cmd {
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

		// Check if a package has completed and needs to be displayed
		var cmds []tea.Cmd
		
		// Display completed packages
		if event.Action == "pass" || event.Action == "fail" || event.Action == "skip" {
			if event.Package != "" && event.Test == "" {
				// This is a package completion event
				if pkg := m.packageResults[event.Package]; pkg != nil {
					if !m.displayedPackages[event.Package] {
						m.displayedPackages[event.Package] = true
						output := m.displayPackageResult(pkg)
						if output != "" {
							// Use tea.Printf to print the package result
							cmds = append(cmds, tea.Printf("%s", output))
						}
					}
				}
			}
		}

		// Update progress based on test completion
		if m.totalTests > 0 {
			progress := float64(m.completedTests) / float64(m.totalTests)
			if progress > 1.0 {
				progress = 1.0
			}
			if cmd := m.progress.SetPercent(progress); cmd != nil {
				cmds = append(cmds, cmd)
			}
		}
		
		// Continue reading
		cmds = append(cmds, m.readNextLine())
		return tea.Batch(cmds...)
	}

	// Continue reading
	return m.readNextLine()
}

// handleTestFail processes test failure messages.
func (m *TestModel) handleTestFail() tea.Cmd {
	m.failCount++
	if !m.done {
		return m.readNextLine()
	}
	return nil
}

// handleTestComplete processes test completion messages.
func (m *TestModel) handleTestComplete(msg testCompleteMsg) tea.Cmd {
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
	var cmds []tea.Cmd
	if m.totalTests > 0 {
		if cmd := m.progress.SetPercent(1.0); cmd != nil {
			cmds = append(cmds, cmd)
		}
	}

	// Auto-quit after a brief delay to show final state
	cmds = append(cmds, tea.Tick(2*time.Second, func(t time.Time) tea.Msg {
		return tea.Quit()
	}))

	return tea.Batch(cmds...)
}

// handleSpinnerTick processes spinner animation updates.
func (m *TestModel) handleSpinnerTick(msg spinner.TickMsg) tea.Cmd {
	if !m.done {
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return cmd
	}
	return nil
}

// handleProgressFrame processes progress bar animation updates.
func (m *TestModel) handleProgressFrame(msg progress.FrameMsg) tea.Cmd {
	progressModel, cmd := m.progress.Update(msg)
	m.progress = progressModel.(progress.Model)
	return cmd
}