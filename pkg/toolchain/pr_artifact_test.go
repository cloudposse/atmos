package toolchain

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Note: IsPRVersion tests are in version_spec_test.go.

func TestSanitizeZipPath(t *testing.T) {
	// Use t.TempDir() to get a platform-appropriate temp directory.
	baseDir := t.TempDir()
	destDir := filepath.Clean(baseDir) + string(os.PathSeparator)

	tests := []struct {
		name      string
		entryName string
		wantPath  string
		wantErr   bool
	}{
		// Valid paths.
		{
			name:      "simple file",
			entryName: "atmos",
			wantPath:  filepath.Join(baseDir, "atmos"),
			wantErr:   false,
		},
		{
			name:      "file in subdirectory",
			entryName: "build/atmos",
			wantPath:  filepath.Join(baseDir, "build", "atmos"),
			wantErr:   false,
		},
		{
			name:      "nested subdirectory",
			entryName: "a/b/c/file.txt",
			wantPath:  filepath.Join(baseDir, "a", "b", "c", "file.txt"),
			wantErr:   false,
		},

		// Zip Slip attack patterns (should error).
		{
			name:      "simple path traversal",
			entryName: "../etc/passwd",
			wantErr:   true,
		},
		{
			name:      "deep path traversal",
			entryName: "../../../etc/passwd",
			wantErr:   true,
		},
		{
			name:      "hidden path traversal",
			entryName: "build/../../../etc/passwd",
			wantErr:   true,
		},
		{
			name:      "windows-style traversal",
			entryName: "..\\etc\\passwd",
			wantErr:   true,
		},
		{
			name:      "absolute path unix",
			entryName: "/etc/passwd",
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotPath, err := sanitizeZipPath(tt.entryName, destDir)

			if tt.wantErr {
				require.Error(t, err, "expected error for malicious path")
				assert.True(t, errors.Is(err, ErrPRArtifactExtractFailed),
					"error should wrap ErrPRArtifactExtractFailed")
				assert.Contains(t, err.Error(), "Zip Slip")
				return
			}

			require.NoError(t, err, "unexpected error")
			assert.Equal(t, tt.wantPath, gotPath, "path mismatch")
		})
	}
}

func TestBuildTokenRequiredError(t *testing.T) {
	err := buildTokenRequiredError()
	assert.Error(t, err)
	// The error uses ErrAuthenticationFailed sentinel.
	assert.Contains(t, err.Error(), "authentication")
}

func TestHandlePRArtifactError(t *testing.T) {
	t.Run("generic error returns tool installation error", func(t *testing.T) {
		err := handlePRArtifactError(assert.AnError, 2038)
		assert.Error(t, err)
		// Generic errors are wrapped with ErrToolInstall.
		assert.Contains(t, err.Error(), "tool installation")
	})
}

// Note: Full integration tests for InstallFromPR require:
// - A valid GitHub token
// - Network access
// - A real PR with artifacts
// Those tests should be in a separate integration test file with appropriate
// skip conditions.
