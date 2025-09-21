package tui

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/cloudposse/atmos/tools/gotcha/pkg/constants"

	"github.com/charmbracelet/bubbles/progress"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/cloudposse/atmos/tools/gotcha/pkg/config"
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
	m.scrollOffset -= ScrollPageSize
	if m.scrollOffset < 0 {
		m.scrollOffset = 0
	}
}

// handlePageDown scrolls the view down by 10 lines.
func (m *TestModel) handlePageDown() {
	m.scrollOffset += ScrollPageSize
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
	m.scanner = bufio.NewScanner(msg.proc)

	// Open JSON file for writing
	jsonFile, err := os.Create(m.outputFile)
	if err == nil {
		m.jsonFile = jsonFile
	}

	// Start reading lines
	return m.readNextLine()
}

// handleStreamOutput processes streaming test output.
func (m *TestModel) handleStreamOutput(msg StreamOutputMsg) tea.Cmd {
	// Write to JSON file if open
	if m.jsonFile != nil {
		m.jsonWriter.Lock()
		_, _ = m.jsonFile.Write([]byte(msg.Line))
		_, _ = m.jsonFile.Write([]byte("\n"))
		m.jsonWriter.Unlock()
	}

	// Parse JSON event
	var event types.TestEvent
	if err := json.Unmarshal([]byte(msg.Line), &event); err == nil {
		m.processEvent(&event)

		// Check if any packages completed and display them once
		var cmds []tea.Cmd

		// Update progress bar if we have tests
		if m.totalTests > 0 && m.completedTests > 0 {
			percentFloat := float64(m.completedTests) / float64(m.totalTests)
			// Use SetPercent and ensure we return the animation command
			cmd := m.progress.SetPercent(percentFloat)
			if cmd != nil {
				cmds = append(cmds, cmd)
			}
		}

		// Display completed packages from the packageOrder list
		for _, pkg := range m.packageOrder {
			if result, exists := m.packageResults[pkg]; exists {
				// Check if package is complete (not running) OR no longer active
				isComplete := result.Status != TestStatusRunning || !m.activePackages[pkg]

				if isComplete && !m.displayedPackages[pkg] {
					// Mark as displayed and generate output
					m.displayedPackages[pkg] = true

					// If still marked as running but not active, mark it as done
					if result.Status == TestStatusRunning && !m.activePackages[pkg] {
						// Package finished but didn't send proper completion event
						// This can happen with packages that have no tests
						if !result.HasTests && len(result.Tests) == 0 {
							result.Status = TestStatusSkip
						} else if len(result.Tests) > 0 {
							// Has tests, check their status
							allPassed := true
							for _, test := range result.Tests {
								if test.Status == TestStatusFail {
									allPassed = false
									break
								}
							}
							if allPassed {
								result.Status = TestStatusPass
							} else {
								result.Status = TestStatusFail
							}
						}
					}

					output := m.displayPackageResult(result)

					// Debug logging for package display
					if debugFile := config.GetDebugFile(); debugFile != "" {
						if f, err := os.OpenFile(debugFile, os.O_CREATE|os.O_APPEND|os.O_WRONLY, constants.DefaultFilePerms); err == nil {
							fmt.Fprintf(f, "[TUI-DEBUG] Package display: %s, status=%s, output_len=%d, has_tests=%v, active=%v\n",
								pkg, result.Status, len(output), result.HasTests, m.activePackages[pkg])
							f.Close()
						}
					}

					if output != "" {
						// Use tea.Printf to print the output once
						cmds = append(cmds, tea.Printf("%s", output))
					}
				}
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
//
// - Exit code analysis
// - Test failure detection
// - Package completion verification
// - Final summary generation
// - Cleanup operations
// These conditions ensure proper test run finalization.
//
//nolint:nestif,gocognit // Test completion requires multiple status checks:
func (m *TestModel) handleTestComplete(msg TestCompleteMsg) tea.Cmd {
	m.done = true
	m.endTime = time.Now()

	// Set exit code based on test results
	// If we have any failures, override exit code to 1
	if m.failCount > 0 {
		m.exitCode = 1
	} else {
		m.exitCode = msg.ExitCode
	}

	// Close JSON file
	if m.jsonFile != nil {
		_ = m.jsonFile.Close()
	}

	// Display any remaining packages that weren't displayed yet
	var displayCmds []tea.Cmd
	for _, pkg := range m.packageOrder {
		if result, exists := m.packageResults[pkg]; exists && !m.displayedPackages[pkg] {
			m.displayedPackages[pkg] = true

			// Fix status if still running
			if result.Status == TestStatusRunning {
				if !result.HasTests && len(result.Tests) == 0 {
					result.Status = TestStatusSkip
				} else {
					result.Status = TestStatusPass // Assume pass if no failures recorded
				}
			}

			output := m.displayPackageResult(result)
			if output != "" {
				displayCmds = append(displayCmds, tea.Printf("%s", output))
			}
		}
	}

	// Emit alert if requested
	if m.alert {
		emitAlert(true)
	}

	// Force 100% progress on completion
	var cmds []tea.Cmd
	cmds = append(cmds, displayCmds...)
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
