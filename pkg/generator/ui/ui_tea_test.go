package ui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/cloudposse/atmos/pkg/generator/templates"
)

// TestTruncateString tests the truncateString helper function.
func TestTruncateString(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		maxLen   int
		expected string
	}{
		{
			name:     "string shorter than max",
			input:    "hello",
			maxLen:   10,
			expected: "hello",
		},
		{
			name:     "string equal to max",
			input:    "hello",
			maxLen:   5,
			expected: "hello",
		},
		{
			name:     "string longer than max",
			input:    "hello world",
			maxLen:   5,
			expected: "hello...",
		},
		{
			name:     "empty string",
			input:    "",
			maxLen:   5,
			expected: "",
		},
		{
			name:     "max length zero",
			input:    "hello",
			maxLen:   0,
			expected: "...",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := truncateString(tt.input, tt.maxLen)
			if result != tt.expected {
				t.Errorf("Expected %q, got %q", tt.expected, result)
			}
		})
	}
}

// TestSpinnerModel_Init tests the spinner model initialization.
func TestSpinnerModel_Init(t *testing.T) {
	s := spinner.New()
	m := spinnerModel{
		spinner: s,
		message: "Loading...",
	}

	cmd := m.Init()
	if cmd == nil {
		t.Error("Expected Init to return a command")
	}
}

// TestSpinnerModel_Update tests the spinner model update function.
func TestSpinnerModel_Update(t *testing.T) {
	s := spinner.New()
	m := spinnerModel{
		spinner: s,
		message: "Loading...",
	}

	tests := []struct {
		name         string
		msg          tea.Msg
		expectQuit   bool
		checkMessage bool
		checkSpinner bool
	}{
		{
			name: "ctrl+c quits",
			msg: tea.KeyMsg{
				Type:  tea.KeyCtrlC,
				Runes: []rune{'c'},
			},
			expectQuit:   true,
			checkMessage: false,
		},
		{
			name: "q quits",
			msg: tea.KeyMsg{
				Type:  tea.KeyRunes,
				Runes: []rune{'q'},
			},
			expectQuit:   true,
			checkMessage: false,
		},
		{
			name: "other keys do nothing",
			msg: tea.KeyMsg{
				Type:  tea.KeyRunes,
				Runes: []rune{'a'},
			},
			expectQuit:   false,
			checkMessage: false,
		},
		{
			name:         "spinner tick updates spinner",
			msg:          s.Tick(),
			expectQuit:   false,
			checkSpinner: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			updatedModel, cmd := m.Update(tt.msg)

			// Check quit behavior
			if tt.expectQuit {
				if cmd == nil {
					t.Error("Expected quit command")
				}
				// We can't directly assert cmd == tea.Quit, but we can verify state
			} else if !tt.checkSpinner {
				if cmd != nil {
					// For non-spinner updates, we expect nil cmd
					t.Error("Expected nil command for non-quit, non-spinner updates")
				}
			}

			// Verify model type
			if _, ok := updatedModel.(spinnerModel); !ok {
				t.Error("Expected updated model to be spinnerModel")
			}
		})
	}
}

// TestSpinnerModel_View tests the spinner model view function.
func TestSpinnerModel_View(t *testing.T) {
	s := spinner.New()
	m := spinnerModel{
		spinner: s,
		message: "Processing files...",
	}

	view := m.View()

	// Strip ANSI codes for reliable testing
	cleanView := ansi.Strip(view)

	// Check that view contains the message
	if !strings.Contains(cleanView, "Processing files...") {
		t.Errorf("Expected view to contain message, got: %s", cleanView)
	}

	// Check that view starts with carriage return (for spinner animation)
	if !strings.HasPrefix(view, "\r") {
		t.Error("Expected view to start with carriage return")
	}
}

// TestFileSkippedError tests the FileSkippedError error type.
func TestFileSkippedError(t *testing.T) {
	err := &FileSkippedError{
		Path:         "{{.Config.env}}/config.yaml",
		RenderedPath: "production/config.yaml",
	}

	expected := "file skipped: {{.Config.env}}/config.yaml (rendered to: production/config.yaml)"
	if err.Error() != expected {
		t.Errorf("Expected %q, got %q", expected, err.Error())
	}
}

// TestInitUI_ColorSource tests the color source helper.
func TestInitUI_ColorSource(t *testing.T) {
	ui := createTestUI(t)

	tests := []struct {
		name         string
		source       string
		expectedText string
	}{
		{
			name:         "scaffold source",
			source:       "scaffold",
			expectedText: "scaffold",
		},
		{
			name:         "flag source",
			source:       "flag",
			expectedText: "flag",
		},
		{
			name:         "default source",
			source:       "unknown",
			expectedText: "default",
		},
		{
			name:         "empty source",
			source:       "",
			expectedText: "default",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ui.colorSource(tt.source)

			// Strip ANSI codes to check text content
			clean := ansi.Strip(result)

			if clean != tt.expectedText {
				t.Errorf("Expected %q, got %q", tt.expectedText, clean)
			}

			// Note: ANSI codes may not be present in test environment
			// The important test is that the text content is correct
		})
	}
}

// TestInitUI_WriteOutput tests the output buffering.
func TestInitUI_WriteOutput(t *testing.T) {
	ui := createTestUI(t)

	// Write to buffer
	ui.writeOutput("Hello %s", "World")
	ui.writeOutput("\nLine 2")

	// Check buffer content before flush
	output := ui.output.String()
	expected := "Hello World\nLine 2"

	if output != expected {
		t.Errorf("Expected %q, got %q", expected, output)
	}

	// Flush should clear buffer
	ui.flushOutput()

	afterFlush := ui.output.String()
	if afterFlush != "" {
		t.Errorf("Expected empty buffer after flush, got %q", afterFlush)
	}
}

// TestInitUI_SetThreshold tests the threshold setter.
func TestInitUI_SetThreshold(t *testing.T) {
	ui := createTestUI(t)

	// Test setting threshold
	ui.SetThreshold(75)

	// We can't directly verify the processor's internal state,
	// but we can verify the method doesn't panic
	ui.SetThreshold(50)
	ui.SetThreshold(100)
	ui.SetThreshold(0)
}

// TestInitUI_GetTerminalWidth tests terminal width detection.
func TestInitUI_GetTerminalWidth(t *testing.T) {
	ui := createTestUI(t)

	width := ui.GetTerminalWidth()

	// Terminal width should be either detected or fallback to 80
	if width <= 0 {
		t.Errorf("Expected positive width, got %d", width)
	}

	// In test environment, we expect fallback width of 80
	// since stdout is not a real terminal
	if width != 80 {
		t.Logf("Terminal width: %d (expected 80 in test environment)", width)
	}
}

// TestInitUI_GenerateSuggestedDirectoryWithValues tests directory suggestion with values.
func TestInitUI_GenerateSuggestedDirectoryWithValues(t *testing.T) {
	ui := createTestUI(t)

	tests := []struct {
		name         string
		config       templates.Configuration
		mergedValues map[string]interface{}
		expected     string
	}{
		{
			name: "uses name from merged values",
			config: templates.Configuration{
				Name: "my-template",
			},
			mergedValues: map[string]interface{}{
				"name": "my-project",
			},
			expected: "./my-project",
		},
		{
			name: "uses project_name from merged values",
			config: templates.Configuration{
				Name: "my-template",
			},
			mergedValues: map[string]interface{}{
				"project_name": "awesome-project",
			},
			expected: "./awesome-project",
		},
		{
			name: "prefers name over project_name",
			config: templates.Configuration{
				Name: "my-template",
			},
			mergedValues: map[string]interface{}{
				"name":         "my-project",
				"project_name": "other-project",
			},
			expected: "./my-project",
		},
		{
			name: "falls back to config name when no values",
			config: templates.Configuration{
				Name: "path/to/my-template",
			},
			mergedValues: nil,
			expected:     "./my-template",
		},
		{
			name: "falls back to config name when empty values",
			config: templates.Configuration{
				Name: "my-template",
			},
			mergedValues: map[string]interface{}{},
			expected:     "./my-template",
		},
		{
			name: "ignores empty name value",
			config: templates.Configuration{
				Name: "my-template",
			},
			mergedValues: map[string]interface{}{
				"name": "",
			},
			expected: "./my-template",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ui.generateSuggestedDirectoryWithValues(tt.config, tt.mergedValues)
			if result != tt.expected {
				t.Errorf("Expected %q, got %q", tt.expected, result)
			}
		})
	}
}

// TestInitUI_GenerateSuggestedDirectoryWithTemplateInfo tests directory suggestion with template info.
func TestInitUI_GenerateSuggestedDirectoryWithTemplateInfo(t *testing.T) {
	ui := createTestUI(t)

	tests := []struct {
		name         string
		templateInfo interface{}
		mergedValues map[string]interface{}
		expected     string
	}{
		{
			name: "uses name from merged values",
			templateInfo: templates.Configuration{
				Name: "my-template",
			},
			mergedValues: map[string]interface{}{
				"name": "my-project",
			},
			expected: "./my-project",
		},
		{
			name: "uses project_name from merged values",
			templateInfo: templates.Configuration{
				Name: "my-template",
			},
			mergedValues: map[string]interface{}{
				"project_name": "awesome-project",
			},
			expected: "./awesome-project",
		},
		{
			name: "uses Configuration.Name as fallback",
			templateInfo: templates.Configuration{
				Name: "path/to/my-template",
			},
			mergedValues: nil,
			expected:     "./my-template",
		},
		{
			name: "uses map name as fallback",
			templateInfo: map[string]interface{}{
				"name": "template-from-map",
			},
			mergedValues: nil,
			expected:     "./template-from-map",
		},
		{
			name:         "fallback when no info available",
			templateInfo: map[string]interface{}{},
			mergedValues: nil,
			expected:     "./new-project",
		},
		{
			name:         "fallback when info is nil",
			templateInfo: nil,
			mergedValues: nil,
			expected:     "./new-project",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ui.generateSuggestedDirectoryWithTemplateInfo(tt.templateInfo, tt.mergedValues)
			if result != tt.expected {
				t.Errorf("Expected %q, got %q", tt.expected, result)
			}
		})
	}
}

// TestInitUI_RenderMarkdown tests markdown rendering.
func TestInitUI_RenderMarkdown(t *testing.T) {
	ui := createTestUI(t)

	tests := []struct {
		name     string
		markdown string
		contains []string
	}{
		{
			name:     "renders simple text",
			markdown: "Hello World",
			contains: []string{"Hello World"},
		},
		{
			name:     "renders heading",
			markdown: "# Main Title\n\nSome content here.",
			contains: []string{"Main Title", "content"},
		},
		{
			name:     "renders list",
			markdown: "- Item 1\n- Item 2\n- Item 3",
			contains: []string{"Item 1", "Item 2", "Item 3"},
		},
		{
			name:     "renders code block",
			markdown: "```go\nfunc main() {\n  fmt.Println(\"Hello\")\n}\n```",
			contains: []string{"func main", "Println"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// renderMarkdown writes to atmosui output, so we can't easily capture it
			// We test that it doesn't panic and doesn't return error
			err := ui.renderMarkdown(tt.markdown)
			if err != nil {
				t.Errorf("Expected no error, got: %v", err)
			}
		})
	}
}

// TestInitUI_RenderREADME tests README rendering.
func TestInitUI_RenderREADME(t *testing.T) {
	ui := createTestUI(t)

	tests := []struct {
		name          string
		readmeContent string
		targetPath    string
		expectError   bool
	}{
		{
			name:          "renders simple README",
			readmeContent: "# My Project\n\nThis is a test project.",
			targetPath:    t.TempDir(),
			expectError:   false,
		},
		{
			name:          "renders README with template",
			readmeContent: "# {{.name}}\n\nVersion: {{.version}}",
			targetPath:    t.TempDir(),
			expectError:   false,
		},
		{
			name:          "handles empty README",
			readmeContent: "",
			targetPath:    t.TempDir(),
			expectError:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ui.renderREADME(tt.readmeContent, tt.targetPath)
			if tt.expectError && err == nil {
				t.Error("Expected error but got nil")
			}
			if !tt.expectError && err != nil {
				t.Errorf("Expected no error, got: %v", err)
			}
		})
	}
}

// TestInitUI_DisplayConfigurationTable tests configuration table rendering.
func TestInitUI_DisplayConfigurationTable(t *testing.T) {
	ui := createTestUI(t)

	tests := []struct {
		name   string
		header []string
		rows   [][]string
	}{
		{
			name:   "empty table does not render",
			header: []string{"Setting", "Value", "Source"},
			rows:   [][]string{},
		},
		{
			name:   "single row",
			header: []string{"Setting", "Value", "Source"},
			rows: [][]string{
				{"name", "my-project", "flag"},
			},
		},
		{
			name:   "multiple rows with different sources",
			header: []string{"Setting", "Value", "Source"},
			rows: [][]string{
				{"name", "my-project", "flag"},
				{"version", "1.0.0", "scaffold"},
				{"description", "Test project", "default"},
			},
		},
		{
			name:   "long values",
			header: []string{"Setting", "Value", "Source"},
			rows: [][]string{
				{"name", "my-very-long-project-name-that-exceeds-normal-width", "scaffold"},
				{"description", "A very long description that should be handled properly by the table rendering logic", "default"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Clear output buffer before test
			ui.output.Reset()

			// Display table
			ui.displayConfigurationTable(tt.header, tt.rows)

			// Get output
			output := ui.output.String()

			// For empty rows, no output should be generated
			if len(tt.rows) == 0 {
				if output != "" {
					t.Errorf("Expected no output for empty table, got: %s", output)
				}
				return
			}

			// For non-empty tables, verify content is present
			if output == "" {
				t.Error("Expected output but got empty string")
			}

			// Strip ANSI codes for content verification
			clean := ansi.Strip(output)

			// Check that header text appears
			if !strings.Contains(clean, "CONFIGURATION SUMMARY") {
				t.Error("Expected output to contain 'CONFIGURATION SUMMARY'")
			}

			// Check that row values appear in output
			for _, row := range tt.rows {
				for _, cell := range row {
					if !strings.Contains(clean, cell) {
						t.Errorf("Expected output to contain %q", cell)
					}
				}
			}
		})
	}
}

// TestInitUI_DisplayTemplateTable tests template table rendering.
func TestInitUI_DisplayTemplateTable(t *testing.T) {
	ui := createTestUI(t)

	tests := []struct {
		name   string
		header []string
		rows   [][]string
	}{
		{
			name:   "single template",
			header: []string{"Template", "Source", "Version", "Description"},
			rows: [][]string{
				{"my-template", "local", "1.0.0", "A test template"},
			},
		},
		{
			name:   "multiple templates",
			header: []string{"Template", "Source", "Version", "Description"},
			rows: [][]string{
				{"template-1", "github.com/org/repo", "v1.0.0", "First template"},
				{"template-2", "local", "v2.0.0", "Second template"},
				{"template-3", "s3://bucket/path", "latest", "Third template"},
			},
		},
		{
			name:   "empty rows",
			header: []string{"Template", "Source", "Version", "Description"},
			rows:   [][]string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// DisplayTemplateTable writes directly to atmosui, not the buffer
			// We can test that it doesn't panic
			ui.DisplayTemplateTable(tt.header, tt.rows)
		})
	}
}

// TestInitUI_DisplayScaffoldTemplateTable tests scaffold template table rendering.
func TestInitUI_DisplayScaffoldTemplateTable(t *testing.T) {
	ui := createTestUI(t)

	tests := []struct {
		name         string
		templatesMap map[string]interface{}
	}{
		{
			name: "single template",
			templatesMap: map[string]interface{}{
				"my-template": map[string]interface{}{
					"source":      "local",
					"description": "A test template",
					"ref":         "v1.0.0",
				},
			},
		},
		{
			name: "multiple templates",
			templatesMap: map[string]interface{}{
				"template-1": map[string]interface{}{
					"source":      "github.com/org/repo",
					"description": "First template",
					"ref":         "v1.0.0",
				},
				"template-2": map[string]interface{}{
					"source":      "local",
					"description": "Second template",
				},
				"template-3": map[string]interface{}{
					"description": "Third template with no source",
				},
			},
		},
		{
			name:         "empty templates",
			templatesMap: map[string]interface{}{},
		},
		{
			name: "template with invalid format",
			templatesMap: map[string]interface{}{
				"invalid": "not-a-map",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// DisplayScaffoldTemplateTable writes directly to atmosui
			// We can test that it doesn't panic
			ui.DisplayScaffoldTemplateTable(tt.templatesMap)
		})
	}
}
