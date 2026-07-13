package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cloudposse/atmos/pkg/generator/engine"
	"github.com/cloudposse/atmos/pkg/generator/templates"
	iolib "github.com/cloudposse/atmos/pkg/io"
	"github.com/cloudposse/atmos/pkg/terminal"
	atmosui "github.com/cloudposse/atmos/pkg/ui"
)

// createTestUI creates a UI instance with I/O for testing.
func createTestUI(t *testing.T) *InitUI {
	t.Helper()

	// Create I/O context with default settings (stdout/stderr)
	ioCtx, err := iolib.NewContext()
	if err != nil {
		t.Fatalf("Failed to create I/O context: %v", err)
	}

	// Initialize UI formatter with I/O context
	atmosui.InitFormatter(ioCtx)

	// Create terminal with I/O
	termWriter := iolib.NewTerminalWriter(ioCtx)
	term := terminal.New(terminal.WithIO(termWriter))

	return NewInitUI(ioCtx, term)
}

func TestNewInitUI(t *testing.T) {
	ui := createTestUI(t)

	if ui.checkmark != "✓" {
		t.Errorf("Expected checkmark to be ✓, got %s", ui.checkmark)
	}

	if ui.xMark != "✗" {
		t.Errorf("Expected xMark to be ✗, got %s", ui.xMark)
	}

	// maxChanges field has been removed - threshold is now handled by the templating processor
}

func TestProcessFile_NewFile(t *testing.T) {
	ui := createTestUI(t)
	tempDir := t.TempDir()

	file := templates.File{
		Path:        "test.txt",
		Content:     "Hello World!",
		IsTemplate:  false,
		Permissions: 0o644,
	}

	// Convert templates.File to engine.File and use the processor
	templatingFile := engine.File{
		Path:        file.Path,
		Content:     file.Content,
		IsTemplate:  file.IsTemplate,
		Permissions: file.Permissions,
	}

	err := ui.processor.ProcessFile(templatingFile, tempDir, false, false, nil, nil)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	filePath := filepath.Join(tempDir, "test.txt")
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		t.Error("Expected file to be created")
	}
}

func TestProcessFile_ExistingFile_NoFlags(t *testing.T) {
	ui := createTestUI(t)
	tempDir := t.TempDir()

	// Create existing file
	filePath := filepath.Join(tempDir, "test.txt")
	err := os.WriteFile(filePath, []byte("existing content"), 0o644)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	file := templates.File{
		Path:        "test.txt",
		Content:     "new content",
		IsTemplate:  false,
		Permissions: 0o644,
	}

	// Convert templates.File to engine.File and use the processor
	templatingFile := engine.File{
		Path:        file.Path,
		Content:     file.Content,
		IsTemplate:  file.IsTemplate,
		Permissions: file.Permissions,
	}

	err = ui.processor.ProcessFile(templatingFile, tempDir, false, false, nil, nil)
	if err == nil {
		t.Fatal("Expected error for existing file")
	}

	if !strings.Contains(err.Error(), "file already exists") {
		t.Errorf("Expected error about existing file, got: %v", err)
	}
}

// TestFileExistsAt verifies the dry-run create/update status helper: the
// single authoritative signal executeWithSetup uses to label a file
// "(would create)" vs "(would update)" in dry-run preview output.
func TestFileExistsAt(t *testing.T) {
	tempDir := t.TempDir()

	if fileExistsAt(tempDir, "missing.txt") {
		t.Error("expected fileExistsAt to report false for a file that doesn't exist")
	}

	if err := os.WriteFile(filepath.Join(tempDir, "present.txt"), []byte("content"), 0o644); err != nil {
		t.Fatalf("failed to seed file: %v", err)
	}
	if !fileExistsAt(tempDir, "present.txt") {
		t.Error("expected fileExistsAt to report true for an existing file")
	}

	if err := os.MkdirAll(filepath.Join(tempDir, "adir"), 0o755); err != nil {
		t.Fatalf("failed to seed directory: %v", err)
	}
	if fileExistsAt(tempDir, "adir") {
		t.Error("expected fileExistsAt to report false for a directory (only files count)")
	}
}

// TestExecuteWithCommandValues_CustomDelimitersReachFileContent verifies that
// custom delimiters resolved in executeWithCommandValues actually reach file
// rendering (previously nil was passed as scaffoldConfig, so ProcessFile
// always fell back to the default "{{"/"}}" delimiters and left custom-
// delimited templates unrendered).
func TestExecuteWithCommandValues_CustomDelimitersReachFileContent(t *testing.T) {
	ui := createTestUI(t)
	tempDir := t.TempDir()

	embedsConfig := &templates.Configuration{
		Name: "test-template",
		Files: []templates.File{
			{
				Path:        "greeting.txt",
				Content:     "Hello, [[ .Config.name ]]!",
				IsTemplate:  true,
				Permissions: 0o644,
			},
		},
	}

	err := ui.executeWithCommandValues(embedsConfig, tempDir, false, false,
		map[string]interface{}{"name": "widget"}, []string{"[[", "]]"})
	if err != nil {
		t.Fatalf("executeWithCommandValues failed: %v", err)
	}

	content, readErr := os.ReadFile(filepath.Join(tempDir, "greeting.txt"))
	if readErr != nil {
		t.Fatalf("failed to read generated file: %v", readErr)
	}

	if !strings.Contains(string(content), "Hello, widget!") {
		t.Errorf("expected custom-delimited template to render, got: %q", string(content))
	}
	if strings.Contains(string(content), "[[") {
		t.Errorf("expected no unresolved custom delimiters in output, got: %q", string(content))
	}
}
