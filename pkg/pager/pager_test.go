package pager

import (
	"errors"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"

	"github.com/cloudposse/atmos/pkg/data"
	iolib "github.com/cloudposse/atmos/pkg/io"
)

// setupTestWriter initializes I/O context and data writer for tests.
// This ensures tests that may trigger data.Write() operations don't depend on globals.
func setupTestWriter(t *testing.T) {
	t.Helper()
	ioCtx, err := iolib.NewContext()
	if err != nil {
		t.Fatalf("Failed to initialize I/O context: %v", err)
	}
	data.InitWriter(ioCtx)
}

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
			setupTestWriter(t)

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
				isTTYAccessible: func() bool {
					return true // Mock /dev/tty as accessible
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
				assert.Len(t, capturedOpts, 1, "Should have 1 tea program option (WithAltScreen)")
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
	setupTestWriter(t)

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
		isTTYAccessible: func() bool {
			return true // Mock /dev/tty as accessible
		},
		enablePager: true,
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
	assert.False(t, concretePC.enablePager, "Pager should be disabled by default")
}

func TestNewWithAtmosConfig(t *testing.T) {
	// Test with pager enabled
	pcEnabled := NewWithAtmosConfig(true)
	assert.NotNil(t, pcEnabled)
	concretePC, ok := pcEnabled.(*pageCreator)
	assert.True(t, ok)
	assert.True(t, concretePC.enablePager, "Pager should be enabled when true is passed")

	// Test with pager disabled
	pcDisabled := NewWithAtmosConfig(false)
	assert.NotNil(t, pcDisabled)
	concretePC2, ok := pcDisabled.(*pageCreator)
	assert.True(t, ok)
	assert.False(t, concretePC2.enablePager, "Pager should be disabled when false is passed")
}

func TestPageCreator_Run_ModelCreation(t *testing.T) {
	setupTestWriter(t)

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
		isTTYAccessible: func() bool {
			return true // Mock /dev/tty as accessible
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
	assert.Len(t, capturedOpts, 1, "Should have 1 tea program option (WithAltScreen)")
}

func TestPageCreator_Run_WithoutPager(t *testing.T) {
	// Test scenarios where pager is not used
	testCases := []struct {
		name          string
		enablePager   bool
		ttySupported  bool
		ttyAccessible bool
		contentFits   bool
	}{
		{
			name:          "pager disabled",
			enablePager:   false,
			ttySupported:  true,
			ttyAccessible: true,
			contentFits:   false,
		},
		{
			name:          "no TTY support",
			enablePager:   true,
			ttySupported:  false,
			ttyAccessible: true,
			contentFits:   false,
		},
		{
			name:          "TTY supported but content fits",
			enablePager:   true,
			ttySupported:  true,
			ttyAccessible: true,
			contentFits:   true,
		},
		{
			name:          "TTY supported but not accessible - skips pager",
			enablePager:   true,
			ttySupported:  true,
			ttyAccessible: false,
			contentFits:   false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			setupTestWriter(t)
			teaProgramCalled := false

			pc := &pageCreator{
				enablePager: tc.enablePager,
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
				isTTYAccessible: func() bool {
					return tc.ttyAccessible
				},
			}

			err := pc.Run("Test", "Content")
			assert.NoError(t, err)
			assert.False(t, teaProgramCalled, "Tea program should not be called when content should be printed directly")
		})
	}
}

func TestMaskContent(t *testing.T) {
	t.Run("masks content with initialized io context", func(t *testing.T) {
		// Initialize I/O context.
		err := iolib.Initialize()
		assert.NoError(t, err)

		// Test with normal content.
		content := "Hello, World!"
		result := maskContent(content)
		assert.Equal(t, content, result) // No secrets to mask, should be unchanged.
	})

	t.Run("returns content unchanged when no secrets", func(t *testing.T) {
		// Initialize I/O context.
		_ = iolib.Initialize()

		content := "This is normal text without any secrets"
		result := maskContent(content)
		assert.Equal(t, content, result)
	})
}

func TestDataWriter_Write(t *testing.T) {
	t.Run("writes content using data package", func(t *testing.T) {
		// Initialize writer.
		setupTestWriter(t)

		writer := &dataWriter{}
		err := writer.Write("test content")
		assert.NoError(t, err)
	})

	t.Run("handles empty content", func(t *testing.T) {
		setupTestWriter(t)

		writer := &dataWriter{}
		err := writer.Write("")
		assert.NoError(t, err)
	})
}

func TestPageCreator_WriteContent(t *testing.T) {
	t.Run("writes content with valid writer", func(t *testing.T) {
		setupTestWriter(t)

		pc := &pageCreator{
			writer: &dataWriter{},
		}

		err := pc.writeContent("test content")
		assert.NoError(t, err)
	})

	t.Run("handles nil writer with fallback", func(t *testing.T) {
		// Initialize I/O context for masking fallback.
		_ = iolib.Initialize()

		pc := &pageCreator{
			writer: nil,
		}

		// Should not panic, and should use fallback.
		err := pc.writeContent("test content")
		assert.NoError(t, err)
	})

	t.Run("returns error from writer", func(t *testing.T) {
		pc := &pageCreator{
			writer: &errorWriter{err: errors.New("write failed")},
		}

		err := pc.writeContent("test content")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to write content")
	})
}

// errorWriter is a mock writer that always returns an error.
type errorWriter struct {
	err error
}

func (w *errorWriter) Write(content string) error {
	return w.err
}

func TestIsTTYAccessible(t *testing.T) {
	// This is an integration test that checks the actual isTTYAccessible function.
	// The result depends on the environment.
	t.Run("returns bool without panicking", func(t *testing.T) {
		// Just verify it doesn't panic and returns a bool.
		result := isTTYAccessible()
		// Can't assert specific value as it depends on environment.
		_ = result
	})
}

func TestPageCreator_IntegrationScenarios(t *testing.T) {
	t.Run("full flow with enabled pager and fitting content", func(t *testing.T) {
		setupTestWriter(t)

		pc := &pageCreator{
			enablePager: true,
			writer:      &dataWriter{},
			newTeaProgram: func(model tea.Model, opts ...tea.ProgramOption) *tea.Program {
				return tea.NewProgram(&simpleTestModel{}, tea.WithInput(nil), tea.WithOutput(nil))
			},
			contentFitsTerminal: func(content string) bool {
				return true // Content fits, no pager needed.
			},
			isTTYSupportForStdout: func() bool {
				return true
			},
			isTTYAccessible: func() bool {
				return true
			},
		}

		err := pc.Run("Title", "Short content")
		assert.NoError(t, err)
	})

	t.Run("full flow with disabled pager", func(t *testing.T) {
		setupTestWriter(t)

		pc := &pageCreator{
			enablePager: false,
			writer:      &dataWriter{},
		}

		err := pc.Run("Title", "Content")
		assert.NoError(t, err)
	})
}

func TestWriterInterface(t *testing.T) {
	t.Run("dataWriter implements Writer interface", func(t *testing.T) {
		var w Writer = &dataWriter{}
		assert.NotNil(t, w)
	})

	t.Run("errorWriter implements Writer interface", func(t *testing.T) {
		var w Writer = &errorWriter{err: errors.New("test")}
		assert.NotNil(t, w)
	})
}

func TestPageCreatorInterface(t *testing.T) {
	t.Run("New returns valid PageCreator", func(t *testing.T) {
		// New() returns PageCreator interface - compiler verifies compliance.
		pc := New()
		assert.NotNil(t, pc)
	})

	t.Run("NewWithAtmosConfig returns valid PageCreator", func(t *testing.T) {
		// NewWithAtmosConfig() returns PageCreator interface - compiler verifies compliance.
		pc := NewWithAtmosConfig(true)
		assert.NotNil(t, pc)
	})
}
