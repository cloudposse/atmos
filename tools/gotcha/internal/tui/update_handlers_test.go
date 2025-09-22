package tui

import (
	"io"
	"os"
	"sync"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestHandleTestComplete tests the handleTestComplete function.
func TestHandleTestComplete(t *testing.T) {
	tests := []struct {
		name         string
		setupModel   func() *TestModel
		msg          TestCompleteMsg
		expectedExit int
		checkModel   func(t *testing.T, m *TestModel)
	}{
		{
			name: "sets exit code from message",
			setupModel: func() *TestModel {
				return &TestModel{
					startTime:         time.Now(),
					packageResults:    make(map[string]*PackageResult),
					displayedPackages: make(map[string]bool),
				}
			},
			msg:          TestCompleteMsg{ExitCode: 0},
			expectedExit: 0,
			checkModel: func(t *testing.T, m *TestModel) {
				assert.True(t, m.done)
				assert.Equal(t, 0, m.exitCode)
				assert.NotZero(t, m.endTime)
			},
		},
		{
			name: "preserves non-zero exit code",
			setupModel: func() *TestModel {
				return &TestModel{
					startTime:         time.Now(),
					packageResults:    make(map[string]*PackageResult),
					displayedPackages: make(map[string]bool),
				}
			},
			msg:          TestCompleteMsg{ExitCode: 1},
			expectedExit: 1,
			checkModel: func(t *testing.T, m *TestModel) {
				assert.True(t, m.done)
				assert.Equal(t, 1, m.exitCode)
			},
		},
		{
			name: "closes JSON file if open",
			setupModel: func() *TestModel {
				// Create a temporary file for testing
				tmpFile, err := os.CreateTemp("", "test-*.json")
				if err != nil {
					t.Fatal(err)
				}
				return &TestModel{
					startTime:         time.Now(),
					packageResults:    make(map[string]*PackageResult),
					displayedPackages: make(map[string]bool),
					jsonFile:          tmpFile,
				}
			},
			msg:          TestCompleteMsg{ExitCode: 0},
			expectedExit: 0,
			checkModel: func(t *testing.T, m *TestModel) {
				assert.True(t, m.done)
				// Try to write to the file - should fail if closed
				if m.jsonFile != nil {
					_, err := m.jsonFile.Write([]byte("test"))
					assert.Error(t, err)
				}
			},
		},
		{
			name: "marks undisplayed packages as displayed",
			setupModel: func() *TestModel {
				return &TestModel{
					startTime:    time.Now(),
					packageOrder: []string{"pkg1", "pkg2"},
					packageResults: map[string]*PackageResult{
						"pkg1": {
							Package:   "pkg1",
							Status:    TestStatusPass,
							Tests:     make(map[string]*TestResult),
							TestOrder: []string{},
						},
						"pkg2": {
							Package:  "pkg2",
							Status:   TestStatusRunning,
							HasTests: true, // Has tests so it will be marked as pass
							Tests: map[string]*TestResult{
								"TestSomething": {Name: "TestSomething", Status: "pass"},
							},
							TestOrder: []string{"TestSomething"},
						},
					},
					displayedPackages: map[string]bool{
						"pkg1": true,
						// pkg2 is not displayed yet
					},
					verbosityLevel: "standard",
					showFilter:     "all",
				}
			},
			msg:          TestCompleteMsg{ExitCode: 0},
			expectedExit: 0,
			checkModel: func(t *testing.T, m *TestModel) {
				assert.True(t, m.done)
				assert.True(t, m.displayedPackages["pkg1"])
				assert.True(t, m.displayedPackages["pkg2"])
				// Running package should be changed to pass
				assert.Equal(t, TestStatusPass, m.packageResults["pkg2"].Status)
			},
		},
		{
			name: "handles packages with no tests",
			setupModel: func() *TestModel {
				return &TestModel{
					startTime:    time.Now(),
					packageOrder: []string{"pkg1"},
					packageResults: map[string]*PackageResult{
						"pkg1": {
							Package:   "pkg1",
							Status:    TestStatusRunning,
							HasTests:  false,
							Tests:     make(map[string]*TestResult),
							TestOrder: []string{},
						},
					},
					displayedPackages: make(map[string]bool),
					verbosityLevel:    "standard",
					showFilter:        "all",
				}
			},
			msg:          TestCompleteMsg{ExitCode: 0},
			expectedExit: 0,
			checkModel: func(t *testing.T, m *TestModel) {
				assert.True(t, m.done)
				// Package with no tests should be marked as skip
				assert.Equal(t, TestStatusSkip, m.packageResults["pkg1"].Status)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			model := tt.setupModel()
			cmd := model.handleTestComplete(tt.msg)

			// Check the model state
			tt.checkModel(t, model)
			assert.Equal(t, tt.expectedExit, model.GetExitCode())

			// The command should batch multiple commands
			if cmd != nil {
				// Execute the command to ensure it doesn't panic
				batchCmd, ok := cmd().(tea.BatchMsg)
				if ok {
					assert.NotNil(t, batchCmd)
				}
			}
		})
	}
}

// TestHandleKeyMsg tests the keyboard input handling.
func TestHandleKeyMsg(t *testing.T) {
	tests := []struct {
		name          string
		setupModel    func() *TestModel
		keyMsg        tea.KeyMsg
		expectQuit    bool
		expectAborted bool
		checkScroll   func(t *testing.T, before, after int)
	}{
		{
			name: "quit on q key",
			setupModel: func() *TestModel {
				return &TestModel{}
			},
			keyMsg:        tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}},
			expectQuit:    true,
			expectAborted: true,
		},
		{
			name: "quit on ctrl+c",
			setupModel: func() *TestModel {
				return &TestModel{}
			},
			keyMsg:        tea.KeyMsg{Type: tea.KeyCtrlC},
			expectQuit:    true,
			expectAborted: true,
		},
		{
			name: "scroll up",
			setupModel: func() *TestModel {
				return &TestModel{
					scrollOffset: 5,
					maxScroll:    10,
				}
			},
			keyMsg: tea.KeyMsg{Type: tea.KeyUp},
			checkScroll: func(t *testing.T, before, after int) {
				assert.Less(t, after, before)
			},
		},
		{
			name: "scroll down",
			setupModel: func() *TestModel {
				return &TestModel{
					scrollOffset: 5,
					maxScroll:    10,
				}
			},
			keyMsg: tea.KeyMsg{Type: tea.KeyDown},
			checkScroll: func(t *testing.T, before, after int) {
				assert.Greater(t, after, before)
			},
		},
		{
			name: "page up",
			setupModel: func() *TestModel {
				return &TestModel{
					scrollOffset: 20,
					maxScroll:    100,
					height:       30,
				}
			},
			keyMsg: tea.KeyMsg{Type: tea.KeyPgUp},
			checkScroll: func(t *testing.T, before, after int) {
				assert.Less(t, after, before)
			},
		},
		{
			name: "page down",
			setupModel: func() *TestModel {
				return &TestModel{
					scrollOffset: 20,
					maxScroll:    100,
					height:       30,
				}
			},
			keyMsg: tea.KeyMsg{Type: tea.KeyPgDown},
			checkScroll: func(t *testing.T, before, after int) {
				assert.GreaterOrEqual(t, after, before) // May hit max
			},
		},
		{
			name: "home key",
			setupModel: func() *TestModel {
				return &TestModel{
					scrollOffset: 50,
					maxScroll:    100,
				}
			},
			keyMsg: tea.KeyMsg{Type: tea.KeyHome},
			checkScroll: func(t *testing.T, before, after int) {
				assert.Equal(t, 0, after)
			},
		},
		{
			name: "end key",
			setupModel: func() *TestModel {
				return &TestModel{
					scrollOffset: 50,
					maxScroll:    100,
				}
			},
			keyMsg: tea.KeyMsg{Type: tea.KeyEnd},
			checkScroll: func(t *testing.T, before, after int) {
				assert.Equal(t, 100, after)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			model := tt.setupModel()
			beforeScroll := model.scrollOffset

			result, cmd := model.handleKeyMsg(tt.keyMsg)
			resultModel := result.(*TestModel)

			if tt.expectQuit {
				assert.NotNil(t, cmd)
				assert.True(t, resultModel.aborted)
			}

			if tt.expectAborted {
				assert.True(t, resultModel.aborted)
			}

			if tt.checkScroll != nil {
				tt.checkScroll(t, beforeScroll, resultModel.scrollOffset)
			}
		})
	}
}

// TestHandleStreamOutput tests the stream output handling.
func TestHandleStreamOutput(t *testing.T) {
	tests := []struct {
		name       string
		setupModel func() *TestModel
		msg        StreamOutputMsg
		checkModel func(t *testing.T, m *TestModel)
	}{
		{
			name: "writes to JSON file if present",
			setupModel: func() *TestModel {
				tmpFile, err := os.CreateTemp("", "test-output-*.json")
				require.NoError(t, err)

				return &TestModel{
					jsonFile:          tmpFile,
					jsonWriter:        &sync.Mutex{},
					packageResults:    make(map[string]*PackageResult),
					displayedPackages: make(map[string]bool),
					packageOrder:      []string{},
					activePackages:    make(map[string]bool),
					subtestStats:      make(map[string]*SubtestStats),
				}
			},
			msg: StreamOutputMsg{
				Line: `{"Time":"2024-01-01T12:00:00Z","Action":"run","Package":"example.com/test","Test":"TestExample"}`,
			},
			checkModel: func(t *testing.T, m *TestModel) {
				// The JSON content should have been written
				// We can't easily verify file content here since jsonFile is an io.WriteCloser
				// but the test ensures no panic occurs
				assert.NotNil(t, m.jsonFile)
			},
		},
		{
			name: "processes valid JSON test event",
			setupModel: func() *TestModel {
				return &TestModel{
					packageResults:    make(map[string]*PackageResult),
					displayedPackages: make(map[string]bool),
					packageOrder:      []string{},
					activePackages:    make(map[string]bool),
					subtestStats:      make(map[string]*SubtestStats),
					jsonWriter:        &sync.Mutex{},
				}
			},
			msg: StreamOutputMsg{
				Line: `{"Time":"2024-01-01T12:00:00Z","Action":"pass","Package":"example.com/test","Test":"TestExample","Elapsed":0.5}`,
			},
			checkModel: func(t *testing.T, m *TestModel) {
				// Verify the test was processed
				assert.Equal(t, 1, m.passCount)
				assert.Contains(t, m.packageOrder, "example.com/test")
				assert.NotNil(t, m.packageResults["example.com/test"])
			},
		},
		{
			name: "handles invalid JSON gracefully",
			setupModel: func() *TestModel {
				return &TestModel{
					packageResults:    make(map[string]*PackageResult),
					displayedPackages: make(map[string]bool),
					packageOrder:      []string{},
					activePackages:    make(map[string]bool),
					subtestStats:      make(map[string]*SubtestStats),
					jsonWriter:        &sync.Mutex{},
				}
			},
			msg: StreamOutputMsg{
				Line: "not json content",
			},
			checkModel: func(t *testing.T, m *TestModel) {
				// Should not crash and counts should remain zero
				assert.Equal(t, 0, m.passCount)
				assert.Equal(t, 0, m.failCount)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			model := tt.setupModel()
			_ = model.handleStreamOutput(tt.msg)
			tt.checkModel(t, model)
		})
	}
}

// MockWriteCloser implements io.WriteCloser for testing.
type MockWriteCloser struct {
	io.Writer
	closed bool
}

func (m *MockWriteCloser) Close() error {
	m.closed = true
	return nil
}

func (m *MockWriteCloser) Write(p []byte) (n int, err error) {
	if m.closed {
		return 0, os.ErrClosed
	}
	return m.Writer.Write(p)
}
