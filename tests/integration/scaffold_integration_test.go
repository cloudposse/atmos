//go:build ignore
// +build ignore

package integration

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/cloudposse/atmos/experiments/init/embeds"
	"github.com/cloudposse/atmos/experiments/init/internal/ui"
)

func TestScaffoldIntegration_FileSkipping(t *testing.T) {
	// Test that the scaffold command properly handles file skipping
	// This test simulates what happens when the scaffold command processes templates

	initUI := ui.NewInitUI()
	tempDir := t.TempDir()

	// Create a configuration with files that should be skipped
	config := embeds.Configuration{
		Name:        "Test Scaffold",
		Description: "Test scaffold integration",
		TemplateID:  "test-scaffold",
		Files: []embeds.File{
			{
				Path:        "README.md",
				Content:     "Test README",
				IsTemplate:  false,
				Permissions: 0644,
			},
			{
				Path:        "{{.Config.namespace}}/config.yaml",
				Content:     "namespace: {{.Config.namespace}}",
				IsTemplate:  true,
				Permissions: 0644,
			},
			{
				Path:        "{{.Config.namespace}}/{{.Config.subdirectory}}/deep.yaml",
				Content:     "deep config",
				IsTemplate:  true,
				Permissions: 0644,
			},
			{
				Path:        "{{if .Config.enable_monitoring}}{{.Config.namespace}}/monitoring.yaml{{end}}",
				Content:     "monitoring config",
				IsTemplate:  true,
				Permissions: 0644,
			},
			{
				Path:        "static/file.txt",
				Content:     "static content",
				IsTemplate:  false,
				Permissions: 0644,
			},
		},
		README: "Test README content",
	}

	// Test case 1: Empty subdirectory and disabled monitoring (should skip files)
	t.Run("empty_subdirectory_and_disabled_monitoring", func(t *testing.T) {
		targetDir := filepath.Join(tempDir, "test1")
		if err := os.MkdirAll(targetDir, 0755); err != nil {
			t.Fatalf("Failed to create target directory: %v", err)
		}

		cmdValues := map[string]interface{}{
			"namespace":         "default",
			"subdirectory":      "",
			"enable_monitoring": false,
		}

		err := initUI.Execute(config, targetDir, false, false, true, cmdValues)
		if err != nil {
			t.Fatalf("Failed to execute template: %v", err)
		}

		// Check that files with empty path segments were skipped
		expectedCreated := []string{
			"README.md",
			"default/config.yaml",
			"static/file.txt",
		}

		expectedSkipped := []string{
			"default/deep.yaml",       // Should be skipped due to empty subdirectory
			"default/monitoring.yaml", // Should be skipped due to disabled monitoring
		}

		// Check created files
		for _, expectedFile := range expectedCreated {
			filePath := filepath.Join(targetDir, expectedFile)
			if _, err := os.Stat(filePath); os.IsNotExist(err) {
				t.Errorf("Expected file '%s' to be created, but it doesn't exist", expectedFile)
			}
		}

		// Check that skipped files don't exist
		for _, skippedFile := range expectedSkipped {
			filePath := filepath.Join(targetDir, skippedFile)
			if _, err := os.Stat(filePath); err == nil {
				t.Errorf("Expected file '%s' to be skipped, but it exists", skippedFile)
			}
		}
	})

	// Test case 2: Valid subdirectory and enabled monitoring (should create all files)
	t.Run("valid_subdirectory_and_enabled_monitoring", func(t *testing.T) {
		targetDir := filepath.Join(tempDir, "test2")
		if err := os.MkdirAll(targetDir, 0755); err != nil {
			t.Fatalf("Failed to create target directory: %v", err)
		}

		cmdValues := map[string]interface{}{
			"namespace":         "production",
			"subdirectory":      "config",
			"enable_monitoring": true,
		}

		err := initUI.Execute(config, targetDir, false, false, true, cmdValues)
		if err != nil {
			t.Fatalf("Failed to execute template: %v", err)
		}

		// Check that all files were created
		expectedCreated := []string{
			"README.md",
			"production/config.yaml",
			"production/config/deep.yaml",
			"production/monitoring.yaml",
			"static/file.txt",
		}

		for _, expectedFile := range expectedCreated {
			filePath := filepath.Join(targetDir, expectedFile)
			if _, err := os.Stat(filePath); os.IsNotExist(err) {
				t.Errorf("Expected file '%s' to be created, but it doesn't exist", expectedFile)
			}
		}
	})

	// Test case 3: Empty namespace (should skip files with empty namespace)
	t.Run("empty_namespace", func(t *testing.T) {
		targetDir := filepath.Join(tempDir, "test3")
		if err := os.MkdirAll(targetDir, 0755); err != nil {
			t.Fatalf("Failed to create target directory: %v", err)
		}

		cmdValues := map[string]interface{}{
			"namespace":         "",
			"subdirectory":      "config",
			"enable_monitoring": true,
		}

		err := initUI.Execute(config, targetDir, false, false, true, cmdValues)
		if err != nil {
			t.Fatalf("Failed to execute template: %v", err)
		}

		// Check that only static files were created
		expectedCreated := []string{
			"README.md",
			"static/file.txt",
		}

		expectedSkipped := []string{
			"config.yaml",      // Should be skipped due to empty namespace
			"config/deep.yaml", // Should be skipped due to empty namespace
			"monitoring.yaml",  // Should be skipped due to empty namespace
		}

		// Check created files
		for _, expectedFile := range expectedCreated {
			filePath := filepath.Join(targetDir, expectedFile)
			if _, err := os.Stat(filePath); os.IsNotExist(err) {
				t.Errorf("Expected file '%s' to be created, but it doesn't exist", expectedFile)
			}
		}

		// Check that skipped files don't exist
		for _, skippedFile := range expectedSkipped {
			filePath := filepath.Join(targetDir, skippedFile)
			if _, err := os.Stat(filePath); err == nil {
				t.Errorf("Expected file '%s' to be skipped, but it exists", skippedFile)
			}
		}
	})
}

func TestScaffoldIntegration_EdgeCases(t *testing.T) {
	// Test edge cases for file skipping behavior

	initUI := ui.NewInitUI()
	tempDir := t.TempDir()

	// Create a configuration with edge case files
	config := embeds.Configuration{
		Name:        "Edge Case Test",
		Description: "Test edge cases for file skipping",
		TemplateID:  "edge-case-test",
		Files: []embeds.File{
			{
				Path:        "{{.Config.empty_field}}/empty_file.yaml",
				Content:     "empty field file",
				IsTemplate:  true,
				Permissions: 0644,
			},
			{
				Path:        "{{.Config.nonexistent_field}}/nonexistent_file.yaml",
				Content:     "nonexistent field file",
				IsTemplate:  true,
				Permissions: 0644,
			},
			{
				Path:        "{{.Config.false_field}}/false_file.yaml",
				Content:     "false field file",
				IsTemplate:  true,
				Permissions: 0644,
			},
			{
				Path:        "{{.Config.null_field}}/null_file.yaml",
				Content:     "null field file",
				IsTemplate:  true,
				Permissions: 0644,
			},
			{
				Path:        "valid/file.yaml",
				Content:     "valid file",
				IsTemplate:  false,
				Permissions: 0644,
			},
		},
		README: "Edge case test README",
	}

	targetDir := filepath.Join(tempDir, "edge-cases")
	if err := os.MkdirAll(targetDir, 0755); err != nil {
		t.Fatalf("Failed to create target directory: %v", err)
	}

	cmdValues := map[string]interface{}{
		"empty_field": "",
		"false_field": false,
		"null_field":  "<no value>", // Use the actual value that gets rendered for nil
		// nonexistent_field is not provided
	}

	err := initUI.Execute(config, targetDir, false, false, true, cmdValues)
	if err != nil {
		t.Fatalf("Failed to execute template: %v", err)
	}

	// Check that only the valid file was created
	expectedCreated := []string{
		"valid/file.yaml",
	}

	expectedSkipped := []string{
		"empty_file.yaml", // Should be skipped due to empty field
		"null_file.yaml",  // Should be skipped due to null field
		// Other files should be skipped due to false/nonexistent fields
	}

	// Check created files
	for _, expectedFile := range expectedCreated {
		filePath := filepath.Join(targetDir, expectedFile)
		if _, err := os.Stat(filePath); os.IsNotExist(err) {
			t.Errorf("Expected file '%s' to be created, but it doesn't exist", expectedFile)
		}
	}

	// Check that skipped files don't exist
	for _, skippedFile := range expectedSkipped {
		filePath := filepath.Join(targetDir, skippedFile)
		if _, err := os.Stat(filePath); err == nil {
			t.Errorf("Expected file '%s' to be skipped, but it exists", skippedFile)
		}
	}
}
