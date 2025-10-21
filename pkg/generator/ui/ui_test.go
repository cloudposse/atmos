package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cloudposse/atmos/pkg/generator/templates"
	"github.com/cloudposse/atmos/pkg/generator/engine"
)

func TestNewInitUI(t *testing.T) {
	ui := NewInitUI()

	if ui.checkmark != "✓" {
		t.Errorf("Expected checkmark to be ✓, got %s", ui.checkmark)
	}

	if ui.xMark != "✗" {
		t.Errorf("Expected xMark to be ✗, got %s", ui.xMark)
	}

	// maxChanges field has been removed - threshold is now handled by the templating processor
}

func TestProcessFile_NewFile(t *testing.T) {
	ui := NewInitUI()
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
	ui := NewInitUI()
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
