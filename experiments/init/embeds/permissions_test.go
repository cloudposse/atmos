package embeds

import (
	"os"
	"path/filepath"
	"testing"
)

// TestFilePermissionsHandling tests that file permissions are handled correctly
func TestFilePermissionsHandling(t *testing.T) {
	tempDir := t.TempDir()

	testCases := []struct {
		name         string
		file         File
		expectedPerm os.FileMode
	}{
		{
			name: "regular file with 0644 permissions",
			file: File{
				Path:        "config.yaml",
				Content:     "apiVersion: v1",
				IsTemplate:  false,
				Permissions: 0644,
			},
			expectedPerm: 0644,
		},
		{
			name: "executable script with 0755 permissions",
			file: File{
				Path:        "setup.sh",
				Content:     "#!/bin/bash\necho 'Hello World'",
				IsTemplate:  false,
				Permissions: 0755,
			},
			expectedPerm: 0755,
		},
		{
			name: "template file with 0644 permissions",
			file: File{
				Path:        "{{.Config.namespace}}/config.yaml",
				Content:     "namespace: {{.Config.namespace}}",
				IsTemplate:  true,
				Permissions: 0644,
			},
			expectedPerm: 0644,
		},
		{
			name: "file with no permissions (should default)",
			file: File{
				Path:        "default.txt",
				Content:     "default content",
				IsTemplate:  false,
				Permissions: 0, // No permissions set
			},
			expectedPerm: 0, // No permissions set
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Create a temporary directory for this test case
			testDir := filepath.Join(tempDir, tc.name)
			if err := os.MkdirAll(testDir, 0755); err != nil {
				t.Fatalf("Failed to create test directory: %v", err)
			}

			// Create the file with the specified permissions
			filePath := filepath.Join(testDir, tc.file.Path)

			// Create directory if needed
			dir := filepath.Dir(filePath)
			if err := os.MkdirAll(dir, 0755); err != nil {
				t.Fatalf("Failed to create directory: %v", err)
			}

			// Write the file with the specified permissions
			if err := os.WriteFile(filePath, []byte(tc.file.Content), tc.file.Permissions); err != nil {
				t.Fatalf("Failed to write file: %v", err)
			}

			// Check that the file was created with correct permissions
			info, err := os.Stat(filePath)
			if err != nil {
				t.Fatalf("Failed to stat created file: %v", err)
			}

			// Get the file mode (permissions)
			fileMode := info.Mode() & 0777 // Mask to get just the permission bits
			if fileMode != tc.expectedPerm {
				t.Errorf("Expected permissions %o, got %o", tc.expectedPerm, fileMode)
			}
		})
	}
}
