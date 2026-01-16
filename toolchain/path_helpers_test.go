package toolchain

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetCurrentPath_ReadsEnv(t *testing.T) {
	testPath := "/custom/test/path:/another/path"
	t.Setenv("PATH", testPath)

	result := getCurrentPath()
	assert.Equal(t, testPath, result)
}

func TestGetCurrentPath_PreservesComplexPath(t *testing.T) {
	testPath := "/path/with spaces:/path/special$chars:/normal"
	t.Setenv("PATH", testPath)

	result := getCurrentPath()
	assert.Equal(t, testPath, result)
}

func TestConstructFinalPath(t *testing.T) {
	sep := string(os.PathListSeparator)
	tests := []struct {
		name        string
		pathEntries []string
		currentPath string
		expected    string
	}{
		{
			name:        "single entry",
			pathEntries: []string{"/tools/bin"},
			currentPath: "/usr/bin",
			expected:    "/tools/bin" + sep + "/usr/bin",
		},
		{
			name:        "multiple entries",
			pathEntries: []string{"/a", "/b"},
			currentPath: "/usr/bin",
			expected:    "/a" + sep + "/b" + sep + "/usr/bin",
		},
		{
			name:        "empty entries",
			pathEntries: []string{},
			currentPath: "/usr/bin",
			expected:    sep + "/usr/bin",
		},
		{
			name:        "empty current path",
			pathEntries: []string{"/tools/bin"},
			currentPath: "",
			expected:    "/tools/bin" + sep,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := constructFinalPath(tt.pathEntries, tt.currentPath)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestResolveDirPath(t *testing.T) {
	tests := []struct {
		name         string
		binaryPath   string
		relativeFlag bool
		expectAbs    bool
	}{
		{
			name:         "relative flag true",
			binaryPath:   "/home/user/.tools/terraform/1.5.0/bin/terraform",
			relativeFlag: true,
			expectAbs:    false,
		},
		{
			name:         "relative flag false",
			binaryPath:   ".tools/terraform/1.5.0/bin/terraform",
			relativeFlag: false,
			expectAbs:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := resolveDirPath(tt.binaryPath, tt.relativeFlag)
			assert.NoError(t, err)
			if tt.expectAbs {
				assert.True(t, filepath.IsAbs(result), "Expected absolute path, got: %s", result)
			}
		})
	}
}

func TestToolPathStruct(t *testing.T) {
	tp := ToolPath{
		Tool:    "terraform",
		Version: "1.5.0",
		Path:    "/home/user/.tools/terraform/1.5.0/bin",
	}

	assert.Equal(t, "terraform", tp.Tool)
	assert.Equal(t, "1.5.0", tp.Version)
	assert.Equal(t, "/home/user/.tools/terraform/1.5.0/bin", tp.Path)
}
