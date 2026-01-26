package clean

import (
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestIsValidDataDir(t *testing.T) {
	tests := []struct {
		name          string
		tfDataDir     string
		expectedError error
	}{
		{
			name:          "Empty TF_DATA_DIR",
			tfDataDir:     "",
			expectedError: ErrEmptyEnvDir,
		},
		{
			name:          "Root TF_DATA_DIR",
			tfDataDir:     "/",
			expectedError: ErrRefusingToDeleteDir,
		},
		{
			name:          "Valid TF_DATA_DIR",
			tfDataDir:     "/valid/path",
			expectedError: nil,
		},
		{
			name:          "Valid relative path",
			tfDataDir:     "./terraform-data",
			expectedError: nil,
		},
		{
			name:          "Valid nested path",
			tfDataDir:     "/home/user/project/.terraform",
			expectedError: nil,
		},
		{
			name:          "Valid path with dot prefix",
			tfDataDir:     "./.terraform",
			expectedError: nil,
		},
		{
			name:          "Valid deep nested path",
			tfDataDir:     "/var/lib/terraform/data/cache",
			expectedError: nil,
		},
		{
			name:          "Valid simple directory name",
			tfDataDir:     ".terraform",
			expectedError: nil,
		},
		{
			name:          "Valid custom data directory",
			tfDataDir:     ".custom-tf-data",
			expectedError: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := IsValidDataDir(tt.tfDataDir)
			if tt.expectedError != nil {
				require.Error(t, err)
				require.ErrorIs(t, err, tt.expectedError)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// TestIsValidDataDir_PathVariations tests various path patterns including those with ".." sequences.
// Note: filepath.Abs normalizes paths before validation, so ".." in path traversal contexts
// (like "/path/../other") gets resolved. Only embedded ".." that remain after normalization
// (like "some..path" where ".." is part of the name) will be rejected.
func TestIsValidDataDir_PathVariations(t *testing.T) {
	tests := []struct {
		name          string
		tfDataDir     string
		expectedError error
	}{
		{
			name:          "Valid absolute path",
			tfDataDir:     "/home/user/terraform",
			expectedError: nil,
		},
		{
			name:          "Valid relative path",
			tfDataDir:     "subdir/terraform-data",
			expectedError: nil,
		},
		{
			// Embedded ".." in filename (not path traversal) - rejected because
			// ".." remains in the absolute path after normalization.
			name:          "Embedded double dots in name - rejected",
			tfDataDir:     "some..path",
			expectedError: ErrRefusingToDelete,
		},
		{
			// Path traversal ".." gets resolved by filepath.Abs, so the final path
			// is clean and valid (e.g., "/path/with/../dots" becomes "/path/dots").
			name:          "Path traversal resolved by filepath.Abs - accepted",
			tfDataDir:     "/path/with/../dots",
			expectedError: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := IsValidDataDir(tt.tfDataDir)
			if tt.expectedError != nil {
				require.Error(t, err)
				require.ErrorIs(t, err, tt.expectedError)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// TestIsValidDataDir_WindowsRootPath tests Windows root path validation.
// Note: These tests verify Windows-style path handling. On Unix/macOS, paths like "C:\"
// are treated as relative paths by filepath.Abs, so they won't trigger the Windows root
// check. The Windows root rejection (validation.go:28-31) only activates on Windows.
func TestIsValidDataDir_WindowsRootPath(t *testing.T) {
	tests := []struct {
		name          string
		tfDataDir     string
		shouldBeError bool
	}{
		{
			name:          "Valid Windows-like path",
			tfDataDir:     "C:/Users/terraform",
			shouldBeError: false,
		},
		{
			name:          "Valid path with drive letter",
			tfDataDir:     "D:/projects/terraform",
			shouldBeError: false,
		},
		{
			// On Windows, "C:\" is rejected as a root path.
			// On Unix/macOS, filepath.Abs treats this as a relative path, so it's accepted.
			name:          "Windows root path - rejected on Windows",
			tfDataDir:     "C:\\",
			shouldBeError: runtime.GOOS == "windows",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := IsValidDataDir(tt.tfDataDir)
			if tt.shouldBeError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}
