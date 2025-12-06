package toolchain

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cloudposse/atmos/pkg/schema"
)

func TestEmitEnv_BashFormat(t *testing.T) {
	// Save original config.
	originalConfig := GetAtmosConfig()
	defer SetAtmosConfig(originalConfig)

	// Create temporary directory for test.
	tempDir := t.TempDir()

	// Create a tool-versions file.
	toolVersionsPath := filepath.Join(tempDir, ".tool-versions")
	content := "terraform 1.5.0\n"
	if err := os.WriteFile(toolVersionsPath, []byte(content), 0o644); err != nil {
		t.Fatalf("Failed to create tool-versions file: %v", err)
	}

	// Create fake terraform binary.
	binDir := filepath.Join(tempDir, "terraform", "1.5.0", "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatalf("Failed to create bin directory: %v", err)
	}
	binaryPath := filepath.Join(binDir, "terraform")
	if err := os.WriteFile(binaryPath, []byte("#!/bin/bash\necho fake"), 0o755); err != nil {
		t.Fatalf("Failed to create fake binary: %v", err)
	}

	// Set up config.
	SetAtmosConfig(&schema.AtmosConfiguration{
		Toolchain: schema.Toolchain{
			VersionsFile: toolVersionsPath,
			InstallPath:  tempDir,
		},
	})

	// Test bash format.
	err := EmitEnv("bash", false)
	// This will fail because terraform isn't actually installed via our installer,
	// but we're testing the logic flow.
	if err != nil && !strings.Contains(err.Error(), "no installed tools found") {
		t.Errorf("EmitEnv() unexpected error: %v", err)
	}
}

func TestEmitEnv_JsonFormat(t *testing.T) {
	// Save original config.
	originalConfig := GetAtmosConfig()
	defer SetAtmosConfig(originalConfig)

	// Create temporary directory for test.
	tempDir := t.TempDir()

	// Create a tool-versions file.
	toolVersionsPath := filepath.Join(tempDir, ".tool-versions")
	content := "terraform 1.5.0\n"
	if err := os.WriteFile(toolVersionsPath, []byte(content), 0o644); err != nil {
		t.Fatalf("Failed to create tool-versions file: %v", err)
	}

	// Set up config.
	SetAtmosConfig(&schema.AtmosConfiguration{
		Toolchain: schema.Toolchain{
			VersionsFile: toolVersionsPath,
			InstallPath:  tempDir,
		},
	})

	// Test JSON format.
	err := EmitEnv("json", false)
	// This will fail because no tools are actually installed.
	if err != nil && !strings.Contains(err.Error(), "no installed tools found") {
		t.Errorf("EmitEnv() unexpected error: %v", err)
	}
}

func TestEmitEnv_FishFormat(t *testing.T) {
	// Save original config.
	originalConfig := GetAtmosConfig()
	defer SetAtmosConfig(originalConfig)

	// Create temporary directory for test.
	tempDir := t.TempDir()

	// Create a tool-versions file.
	toolVersionsPath := filepath.Join(tempDir, ".tool-versions")
	content := "terraform 1.5.0\n"
	if err := os.WriteFile(toolVersionsPath, []byte(content), 0o644); err != nil {
		t.Fatalf("Failed to create tool-versions file: %v", err)
	}

	// Set up config.
	SetAtmosConfig(&schema.AtmosConfiguration{
		Toolchain: schema.Toolchain{
			VersionsFile: toolVersionsPath,
			InstallPath:  tempDir,
		},
	})

	// Test fish format.
	err := EmitEnv("fish", false)
	// This will fail because no tools are actually installed.
	if err != nil && !strings.Contains(err.Error(), "no installed tools found") {
		t.Errorf("EmitEnv() unexpected error: %v", err)
	}
}

func TestEmitEnv_PowershellFormat(t *testing.T) {
	// Save original config.
	originalConfig := GetAtmosConfig()
	defer SetAtmosConfig(originalConfig)

	// Create temporary directory for test.
	tempDir := t.TempDir()

	// Create a tool-versions file.
	toolVersionsPath := filepath.Join(tempDir, ".tool-versions")
	content := "terraform 1.5.0\n"
	if err := os.WriteFile(toolVersionsPath, []byte(content), 0o644); err != nil {
		t.Fatalf("Failed to create tool-versions file: %v", err)
	}

	// Set up config.
	SetAtmosConfig(&schema.AtmosConfiguration{
		Toolchain: schema.Toolchain{
			VersionsFile: toolVersionsPath,
			InstallPath:  tempDir,
		},
	})

	// Test PowerShell format.
	err := EmitEnv("powershell", false)
	// This will fail because no tools are actually installed.
	if err != nil && !strings.Contains(err.Error(), "no installed tools found") {
		t.Errorf("EmitEnv() unexpected error: %v", err)
	}
}

func TestEmitEnv_DotenvFormat(t *testing.T) {
	// Save original config.
	originalConfig := GetAtmosConfig()
	defer SetAtmosConfig(originalConfig)

	// Create temporary directory for test.
	tempDir := t.TempDir()

	// Create a tool-versions file.
	toolVersionsPath := filepath.Join(tempDir, ".tool-versions")
	content := "terraform 1.5.0\n"
	if err := os.WriteFile(toolVersionsPath, []byte(content), 0o644); err != nil {
		t.Fatalf("Failed to create tool-versions file: %v", err)
	}

	// Set up config.
	SetAtmosConfig(&schema.AtmosConfiguration{
		Toolchain: schema.Toolchain{
			VersionsFile: toolVersionsPath,
			InstallPath:  tempDir,
		},
	})

	// Test dotenv format.
	err := EmitEnv("dotenv", false)
	// This will fail because no tools are actually installed.
	if err != nil && !strings.Contains(err.Error(), "no installed tools found") {
		t.Errorf("EmitEnv() unexpected error: %v", err)
	}
}

func TestEmitEnv_NoToolVersionsFile(t *testing.T) {
	// Save original config.
	originalConfig := GetAtmosConfig()
	defer SetAtmosConfig(originalConfig)

	// Create temporary directory for test.
	tempDir := t.TempDir()

	// Set up config with non-existent tool-versions file.
	toolVersionsPath := filepath.Join(tempDir, ".tool-versions")
	SetAtmosConfig(&schema.AtmosConfiguration{
		Toolchain: schema.Toolchain{
			VersionsFile: toolVersionsPath,
			InstallPath:  tempDir,
		},
	})

	// This should error because tool-versions file doesn't exist.
	err := EmitEnv("bash", false)
	if err == nil {
		t.Error("EmitEnv() should error when tool-versions file doesn't exist")
	}
	if !strings.Contains(err.Error(), "no tools configured") {
		t.Errorf("EmitEnv() error should mention missing tool-versions file, got: %v", err)
	}
}

func TestEmitEnv_RelativePaths(t *testing.T) {
	// Save original config.
	originalConfig := GetAtmosConfig()
	defer SetAtmosConfig(originalConfig)

	// Create temporary directory for test.
	tempDir := t.TempDir()

	// Create a tool-versions file.
	toolVersionsPath := filepath.Join(tempDir, ".tool-versions")
	content := "terraform 1.5.0\n"
	if err := os.WriteFile(toolVersionsPath, []byte(content), 0o644); err != nil {
		t.Fatalf("Failed to create tool-versions file: %v", err)
	}

	// Set up config.
	SetAtmosConfig(&schema.AtmosConfiguration{
		Toolchain: schema.Toolchain{
			VersionsFile: toolVersionsPath,
			InstallPath:  tempDir,
		},
	})

	// Test relative paths.
	err := EmitEnv("bash", true)
	// This will fail because no tools are actually installed.
	if err != nil && !strings.Contains(err.Error(), "no installed tools found") {
		t.Errorf("EmitEnv() unexpected error: %v", err)
	}
}
