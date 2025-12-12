package toolchain

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/cloudposse/atmos/pkg/schema"
)

func TestDuExec_NoToolsInstalled(t *testing.T) {
	// Save original config.
	originalConfig := GetAtmosConfig()
	defer SetAtmosConfig(originalConfig)

	// Create a temporary directory for tools.
	tempDir := t.TempDir()

	// Set up config to use temp directory.
	SetAtmosConfig(&schema.AtmosConfiguration{
		Toolchain: schema.Toolchain{
			InstallPath: tempDir,
		},
	})

	// This should not error even if the directory is empty.
	err := DuExec()
	if err != nil {
		t.Errorf("DuExec() should not error for empty directory: %v", err)
	}
}

func TestDuExec_WithInstalledTools(t *testing.T) {
	// Save original config.
	originalConfig := GetAtmosConfig()
	defer SetAtmosConfig(originalConfig)

	// Create a temporary directory for tools.
	tempDir := t.TempDir()

	// Create some fake tool files.
	toolDir := filepath.Join(tempDir, "terraform", "1.5.0")
	if err := os.MkdirAll(toolDir, 0o755); err != nil {
		t.Fatalf("Failed to create tool directory: %v", err)
	}

	// Create a file with known size (1MB).
	filePath := filepath.Join(toolDir, "terraform")
	data := make([]byte, 1024*1024) // 1MB.
	if err := os.WriteFile(filePath, data, 0o644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Set up config to use temp directory.
	SetAtmosConfig(&schema.AtmosConfiguration{
		Toolchain: schema.Toolchain{
			InstallPath: tempDir,
		},
	})

	// This should not error and should report size.
	err := DuExec()
	if err != nil {
		t.Errorf("DuExec() should not error with installed tools: %v", err)
	}
}

func TestDuExec_NonExistentDirectory(t *testing.T) {
	// Save original config.
	originalConfig := GetAtmosConfig()
	defer SetAtmosConfig(originalConfig)

	// Set up config to use a non-existent directory.
	SetAtmosConfig(&schema.AtmosConfiguration{
		Toolchain: schema.Toolchain{
			InstallPath: "/nonexistent/path/that/does/not/exist",
		},
	})

	// This should not error (should report 0 usage).
	err := DuExec()
	if err != nil {
		t.Errorf("DuExec() should not error for non-existent directory: %v", err)
	}
}

func TestCalculateDirectorySize(t *testing.T) {
	tests := []struct {
		name          string
		setupFunc     func(t *testing.T) string
		expectedSize  int64
		expectError   bool
		errorContains string
	}{
		{
			name: "empty directory",
			setupFunc: func(t *testing.T) string {
				return t.TempDir()
			},
			expectedSize: 0,
			expectError:  false,
		},
		{
			name: "directory with single file",
			setupFunc: func(t *testing.T) string {
				dir := t.TempDir()
				data := make([]byte, 1024) // 1KB.
				if err := os.WriteFile(filepath.Join(dir, "file.txt"), data, 0o644); err != nil {
					t.Fatalf("Failed to create test file: %v", err)
				}
				return dir
			},
			expectedSize: 1024,
			expectError:  false,
		},
		{
			name: "directory with nested files",
			setupFunc: func(t *testing.T) string {
				dir := t.TempDir()
				subdir := filepath.Join(dir, "subdir")
				if err := os.MkdirAll(subdir, 0o755); err != nil {
					t.Fatalf("Failed to create subdirectory: %v", err)
				}
				// Create 2KB file in root.
				data1 := make([]byte, 2048)
				if err := os.WriteFile(filepath.Join(dir, "file1.txt"), data1, 0o644); err != nil {
					t.Fatalf("Failed to create test file: %v", err)
				}
				// Create 3KB file in subdirectory.
				data2 := make([]byte, 3072)
				if err := os.WriteFile(filepath.Join(subdir, "file2.txt"), data2, 0o644); err != nil {
					t.Fatalf("Failed to create test file: %v", err)
				}
				return dir
			},
			expectedSize: 5120, // 2KB + 3KB.
			expectError:  false,
		},
		{
			name: "non-existent directory",
			setupFunc: func(t *testing.T) string {
				return "/nonexistent/path/that/does/not/exist"
			},
			expectedSize: 0,
			expectError:  true, // filepath.Walk returns error which we pass through.
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := tt.setupFunc(t)
			size, err := calculateDirectorySize(dir)

			if tt.expectError {
				if err == nil {
					t.Errorf("calculateDirectorySize() expected error but got none")
				}
				return
			}

			if err != nil {
				t.Errorf("calculateDirectorySize() unexpected error: %v", err)
			}

			if size != tt.expectedSize {
				t.Errorf("calculateDirectorySize() = %d, want %d", size, tt.expectedSize)
			}
		})
	}
}

func TestFormatBytes(t *testing.T) {
	tests := []struct {
		name  string
		bytes int64
		want  string
	}{
		{
			name:  "bytes",
			bytes: 500,
			want:  "500b",
		},
		{
			name:  "kilobytes",
			bytes: 1024,
			want:  "1kb",
		},
		{
			name:  "multiple kilobytes",
			bytes: 5120,
			want:  "5kb",
		},
		{
			name:  "megabytes",
			bytes: 1024 * 1024,
			want:  "1mb",
		},
		{
			name:  "multiple megabytes",
			bytes: 10 * 1024 * 1024,
			want:  "10mb",
		},
		{
			name:  "gigabytes",
			bytes: 1024 * 1024 * 1024,
			want:  "1.0gb",
		},
		{
			name:  "gigabytes with decimal",
			bytes: 1536 * 1024 * 1024, // 1.5GB.
			want:  "1.5gb",
		},
		{
			name:  "large gigabytes",
			bytes: 10 * 1024 * 1024 * 1024,
			want:  "10.0gb",
		},
		{
			name:  "zero bytes",
			bytes: 0,
			want:  "0b",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatBytes(tt.bytes)
			if got != tt.want {
				t.Errorf("formatBytes(%d) = %s, want %s", tt.bytes, got, tt.want)
			}
		})
	}
}
