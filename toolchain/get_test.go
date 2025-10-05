package toolchain

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cloudposse/atmos/pkg/schema"
)

// Setup temporary .tool-versions file for testing.
func createTempToolVersionsFile(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	filePath := filepath.Join(dir, DefaultToolVersionsFilePath)
	err := os.WriteFile(filePath, []byte(content), defaultFileWritePermissions)
	if err != nil {
		t.Fatalf("failed to create temp .tool-versions file: %v", err)
	}
	return filePath
}

// Setup temporary binary path for testing findBinaryPath.
func createTempBinary(t *testing.T, owner, repo, version string) string {
	t.Helper()
	dir := t.TempDir()
	binaryPath := filepath.Join(dir, owner, repo, version, "bin", "tool")
	err := os.MkdirAll(filepath.Dir(binaryPath), defaultMkdirPermissions)
	if err != nil {
		t.Fatalf("failed to create temp binary path: %v", err)
	}
	err = os.WriteFile(binaryPath, []byte("fake binary"), defaultMkdirPermissions)
	if err != nil {
		t.Fatalf("failed to create temp binary: %v", err)
	}
	return dir
}

func TestListToolVersions(t *testing.T) {
	// Mock termenv.ColorProfile for consistent styling

	tests := []struct {
		name          string
		filePath      string
		toolVersions  string
		toolName      string
		showAll       bool
		limit         int
		binaryDir     string
		expectedError string
	}{
		{
			name:          "empty filePath uses default",
			filePath:      "",
			toolVersions:  "owner/repo 1.0.0\n",
			toolName:      "owner/repo",
			binaryDir:     createTempBinary(t, "owner", "repo", "1.0.0"),
			expectedError: "",
		},
		{
			name:          "invalid tool name",
			filePath:      createTempToolVersionsFile(t, "owner/repo 1.0.0\n"),
			toolName:      "invalid",
			expectedError: "invalid tool name: ",
		},
		{
			name:          "tool not found",
			filePath:      createTempToolVersionsFile(t, "other/repo 1.0.0\n"),
			toolName:      "owner/repo",
			expectedError: "tool 'owner/repo' not found in",
		},
		{
			name:          "no versions configured",
			filePath:      createTempToolVersionsFile(t, "owner/repo\n"),
			toolName:      "owner/repo",
			expectedError: "missing version",
		},
		{
			name:          "load versions from file",
			filePath:      createTempToolVersionsFile(t, "owner/repo 1.0.0 2.0.0 1.0.0\n"),
			toolName:      "owner/repo",
			binaryDir:     createTempBinary(t, "owner", "repo", "1.0.0"),
			expectedError: "",
		},
		{
			name:          "use original toolName",
			filePath:      createTempToolVersionsFile(t, "terraform 1.0.0 2.0.0\n"),
			toolName:      "terraform",
			binaryDir:     createTempBinary(t, "hashicorp", "terraform", "1.0.0"),
			expectedError: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set up environment for findBinaryPath if needed
			if tt.binaryDir != "" {
				// Assume findBinaryPath checks a path like ~/.tools/owner/repo/version
				os.Setenv("HOME", tt.binaryDir)
				defer os.Unsetenv("HOME")
			}

			if tt.filePath == "" {
				tt.filePath = createTempToolVersionsFile(t, tt.toolVersions)
			}
			SetAtmosConfig(&schema.AtmosConfiguration{
				Toolchain: schema.Toolchain{
					FilePath: tt.filePath,
				},
			})
			// Run the function
			err := ListToolVersions(tt.showAll, tt.limit, tt.toolName)

			// Check error
			if tt.expectedError == "" {
				if err != nil {
					t.Errorf("expected no error, got %v", err)
				}
			} else {
				if err == nil || !strings.Contains(err.Error(), tt.expectedError) {
					t.Errorf("expected error containing %q, got %v", tt.expectedError, err)
				}
			}
		})
	}
}
