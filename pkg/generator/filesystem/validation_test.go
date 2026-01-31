package filesystem

import (
	"os"
	"path/filepath"
	"testing"
)

func TestValidateTargetDirectory(t *testing.T) {
	tests := []struct {
		name        string
		setupDir    bool
		addFiles    bool
		force       bool
		update      bool
		expectError bool
	}{
		{
			name:        "empty directory with force",
			setupDir:    true,
			addFiles:    false,
			force:       true,
			update:      false,
			expectError: false,
		},
		{
			name:        "empty directory with update",
			setupDir:    true,
			addFiles:    false,
			force:       false,
			update:      true,
			expectError: false,
		},
		{
			name:        "empty directory without flags",
			setupDir:    true,
			addFiles:    false,
			force:       false,
			update:      false,
			expectError: false,
		},
		{
			name:        "directory with files and force",
			setupDir:    true,
			addFiles:    true,
			force:       true,
			update:      false,
			expectError: false,
		},
		{
			name:        "directory with files and update",
			setupDir:    true,
			addFiles:    true,
			force:       false,
			update:      true,
			expectError: false,
		},
		{
			name:        "directory with files without flags",
			setupDir:    true,
			addFiles:    true,
			force:       false,
			update:      false,
			expectError: true,
		},
		{
			name:        "non-existent directory",
			setupDir:    false,
			addFiles:    false,
			force:       false,
			update:      false,
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temporary directory for test
			tempDir := t.TempDir()
			targetPath := filepath.Join(tempDir, "target")

			//nolint:nestif // test setup logic requires nested conditionals
			if tt.setupDir {
				if err := os.MkdirAll(targetPath, 0o755); err != nil {
					t.Fatalf("failed to create test directory: %v", err)
				}

				if tt.addFiles {
					// Create some test files
					testFiles := []string{"file1.txt", "file2.yaml", "subdir/file3.go"}
					for _, file := range testFiles {
						filePath := filepath.Join(targetPath, file)
						dir := filepath.Dir(filePath)
						if err := os.MkdirAll(dir, 0o755); err != nil {
							t.Fatalf("failed to create subdirectory: %v", err)
						}
						if err := os.WriteFile(filePath, []byte("test content"), 0o644); err != nil {
							t.Fatalf("failed to create test file: %v", err)
						}
					}
				}
			}

			// Run validation
			err := ValidateTargetDirectory(targetPath, tt.force, tt.update)

			// Check results
			if tt.expectError && err == nil {
				t.Errorf("expected error but got none")
			}
			if !tt.expectError && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}
