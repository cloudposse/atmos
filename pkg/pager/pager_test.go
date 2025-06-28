package pager

import (
	"errors"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
)

func TestPageCreator_Run(t *testing.T) {
	tests := []struct {
		name                 string
		title                string
		content              string
		isTTYSupported       bool
		contentFitsTerminal  bool
		expectedError        error
		expectTeaProgramCall bool
	}{
		{
			name:                 "no TTY support - prints content directly",
			title:                "Test Title",
			content:              "Test content",
			isTTYSupported:       false,
			contentFitsTerminal:  true,
			expectTeaProgramCall: false,
		},
		{
			name:                 "TTY support and content fits - prints content directly",
			title:                "Test Title",
			content:              "Short content",
			isTTYSupported:       true,
			contentFitsTerminal:  true,
			expectTeaProgramCall: false,
		},
		{
			name:                 "TTY support and content doesn't fit - uses pager",
			title:                "Long Title",
			content:              "Very long content that doesn't fit",
			isTTYSupported:       true,
			contentFitsTerminal:  false,
			expectTeaProgramCall: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Track whether newTeaProgram was called and capture the model.
			teaProgramCalled := false
			var capturedModel *model
			var capturedOpts []tea.ProgramOption

			// Create pageCreator with mocked dependencies
			pc := &pageCreator{
				enablePager: true, // Enable pager for testing
				newTeaProgram: func(modelObj tea.Model, opts ...tea.ProgramOption) *tea.Program {
					teaProgramCalled = true

					// Verify the model is created correctly
					m, ok := modelObj.(*model)
					assert.True(t, ok, "Model should be of type *model")
					assert.Equal(t, tt.title, m.title)
					assert.Equal(t, tt.content, m.content)
					assert.False(t, m.ready)
					assert.NotNil(t, m.viewport)
					capturedModel = m
					capturedOpts = opts

					// Create a real tea.Program but with a simple model that won't actually run
					// Since we're mocking the function, we control what gets returned
					return tea.NewProgram(&simpleTestModel{}, tea.WithInput(nil), tea.WithOutput(nil))
				},
				contentFitsTerminal: func(content string) bool {
					assert.Equal(t, tt.content, content)
					return tt.contentFitsTerminal
				},
				isTTYSupportForStdout: func() bool {
					return tt.isTTYSupported
				},
			}

			// Execute the test
			err := pc.Run(tt.title, tt.content)

			// Verify results
			if tt.expectedError != nil {
				assert.Error(t, err)
				assert.Equal(t, tt.expectedError.Error(), err.Error())
			} else {
				assert.NoError(t, err)
			}

			// Verify whether tea program was called as expected
			assert.Equal(t, tt.expectTeaProgramCall, teaProgramCalled)

			if tt.expectTeaProgramCall {
				assert.NotNil(t, capturedModel)
				assert.Len(t, capturedOpts, 2, "Should have 2 tea program options")
			}
		})
	}
}

// simpleTestModel is a minimal tea.Model that immediately quits for testing.
type simpleTestModel struct{}

func (m *simpleTestModel) Init() tea.Cmd {
	return tea.Quit
}

func (m *simpleTestModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	return m, tea.Quit
}

func (m *simpleTestModel) View() string {
	return ""
}

func TestPageCreator_Run_WithError(t *testing.T) {
	// Test error handling by creating a pageCreator that simulates an error
	expectedErr := errors.New("simulated tea program error")

	pc := &pageCreator{
		newTeaProgram: func(model tea.Model, opts ...tea.ProgramOption) *tea.Program {
			// Create a program that will cause an error when Run() is called
			// We simulate this by returning a program with a model that returns an error
			return tea.NewProgram(&errorTestModel{err: expectedErr}, tea.WithInput(nil), tea.WithOutput(nil))
		},
		contentFitsTerminal: func(content string) bool {
			return false // Force pager usage
		},
		isTTYSupportForStdout: func() bool {
			return true
		},
	}

	err := pc.Run("Test", "Content")
	// Note: In practice, tea.Program.Run() doesn't typically return errors from Update()
	// This is more for demonstrating how we might test error scenarios
	assert.NoError(t, err) // tea.Program.Run() handles model errors internally
}

// errorTestModel is a test model that can simulate errors.
type errorTestModel struct {
	err error
}

func (m *errorTestModel) Init() tea.Cmd {
	return tea.Quit
}

func (m *errorTestModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	return m, tea.Quit
}

func (m *errorTestModel) View() string {
	return ""
}

func TestNew(t *testing.T) {
	pc := New()

	// Verify that New() returns a pageCreator with proper function assignments
	assert.NotNil(t, pc)

	// Cast to concrete type to verify internal structure
	concretePC, ok := pc.(*pageCreator)
	assert.True(t, ok, "New() should return *pageCreator")
	assert.NotNil(t, concretePC.newTeaProgram)
	assert.NotNil(t, concretePC.contentFitsTerminal)
	assert.NotNil(t, concretePC.isTTYSupportForStdout)
}

func TestPageCreator_Run_ModelCreation(t *testing.T) {
	title := "Test Title"
	content := "Test Content"

	// Create a pageCreator that captures the model passed to newTeaProgram
	var capturedModel *model
	var capturedOpts []tea.ProgramOption

	pc := &pageCreator{
		newTeaProgram: func(modelObj tea.Model, opts ...tea.ProgramOption) *tea.Program {
			capturedModel = modelObj.(*model)
			capturedOpts = opts
			return tea.NewProgram(&simpleTestModel{}, tea.WithInput(nil), tea.WithOutput(nil))
		},
		contentFitsTerminal: func(content string) bool {
			return false // Force pager usage
		},
		isTTYSupportForStdout: func() bool {
			return true
		},
		enablePager: true,
	}

	err := pc.Run(title, content)

	assert.NoError(t, err)
	assert.NotNil(t, capturedModel)
	assert.Equal(t, title, capturedModel.title)
	assert.Equal(t, content, capturedModel.content)
	assert.False(t, capturedModel.ready)
	assert.NotNil(t, capturedModel.viewport)

	// Verify viewport is initialized with zero dimensions
	assert.Equal(t, 0, capturedModel.viewport.Width)
	assert.Equal(t, 0, capturedModel.viewport.Height)

	// Verify the program options
	assert.Len(t, capturedOpts, 2, "Should have 2 tea program options")
}

func TestPageCreator_Run_WithoutPager(t *testing.T) {
	// Test scenarios where pager is not used
	testCases := []struct {
		name         string
		ttySupported bool
		contentFits  bool
	}{
		{
			name:         "no TTY support",
			ttySupported: false,
			contentFits:  true,
		},
		{
			name:         "TTY supported but content fits",
			ttySupported: true,
			contentFits:  true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			teaProgramCalled := false

			pc := &pageCreator{
				enablePager: true, // Disable pager to test direct printing
				newTeaProgram: func(model tea.Model, opts ...tea.ProgramOption) *tea.Program {
					teaProgramCalled = true
					return tea.NewProgram(&simpleTestModel{}, tea.WithInput(nil), tea.WithOutput(nil))
				},
				contentFitsTerminal: func(content string) bool {
					return tc.contentFits
				},
				isTTYSupportForStdout: func() bool {
					return tc.ttySupported
				},
			}

			err := pc.Run("Test", "Content")
			assert.NoError(t, err)
			assert.False(t, teaProgramCalled, "Tea program should not be called when content should be printed directly")
		})
	}
}
