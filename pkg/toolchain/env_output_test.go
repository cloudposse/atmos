package toolchain

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAppendToFile_GithubFormat(t *testing.T) {
	setupTestIO(t)
	tempFile := filepath.Join(t.TempDir(), "github-path")

	err := appendToFile(tempFile, "github", []string{"/tools/bin1", "/tools/bin2"}, "")
	require.NoError(t, err)

	content, err := os.ReadFile(tempFile)
	require.NoError(t, err)
	assert.Equal(t, "/tools/bin1\n/tools/bin2\n", string(content))
}

func TestAppendToFile_BashFormat(t *testing.T) {
	setupTestIO(t)
	tempFile := filepath.Join(t.TempDir(), "bash-env")

	err := appendToFile(tempFile, "bash", []string{}, "/tools/bin:/usr/bin")
	require.NoError(t, err)

	content, err := os.ReadFile(tempFile)
	require.NoError(t, err)
	assert.Equal(t, "export PATH='/tools/bin:/usr/bin'\n", string(content))
}

func TestAppendToFile_DotenvFormat(t *testing.T) {
	setupTestIO(t)
	tempFile := filepath.Join(t.TempDir(), "dotenv")

	err := appendToFile(tempFile, "dotenv", []string{}, "/tools/bin:/usr/bin")
	require.NoError(t, err)

	content, err := os.ReadFile(tempFile)
	require.NoError(t, err)
	assert.Equal(t, "PATH='/tools/bin:/usr/bin'\n", string(content))
}

func TestAppendToFile_FishFormat(t *testing.T) {
	setupTestIO(t)
	tempFile := filepath.Join(t.TempDir(), "fish")

	// Use platform-appropriate path separator since formatFishContent splits by os.PathListSeparator.
	testPath := "/tools/bin" + string(os.PathListSeparator) + "/usr/bin"
	err := appendToFile(tempFile, "fish", []string{}, testPath)
	require.NoError(t, err)

	content, err := os.ReadFile(tempFile)
	require.NoError(t, err)
	assert.Contains(t, string(content), "set -gx PATH")
	assert.Contains(t, string(content), "'/tools/bin'")
	assert.Contains(t, string(content), "'/usr/bin'")
}

func TestAppendToFile_PowershellFormat(t *testing.T) {
	setupTestIO(t)
	tempFile := filepath.Join(t.TempDir(), "ps")

	err := appendToFile(tempFile, "powershell", []string{}, "/tools/bin:/usr/bin")
	require.NoError(t, err)

	content, err := os.ReadFile(tempFile)
	require.NoError(t, err)
	assert.Contains(t, string(content), "$env:PATH")
	assert.Contains(t, string(content), "/tools/bin:/usr/bin")
}

func TestAppendToFile_JSONFormat(t *testing.T) {
	setupTestIO(t)
	tempFile := filepath.Join(t.TempDir(), "json")

	err := appendToFile(tempFile, "json", []string{}, "/tools/bin:/usr/bin")
	require.NoError(t, err)

	content, err := os.ReadFile(tempFile)
	require.NoError(t, err)
	assert.Contains(t, string(content), "final_path")
	assert.Contains(t, string(content), "/tools/bin:/usr/bin")
}

func TestAppendToFile_AppendsMultipleTimes(t *testing.T) {
	setupTestIO(t)
	tempFile := filepath.Join(t.TempDir(), "append-test")

	err := appendToFile(tempFile, "bash", []string{}, "/usr/bin")
	require.NoError(t, err)

	err = appendToFile(tempFile, "bash", []string{}, "/usr/bin")
	require.NoError(t, err)

	content, err := os.ReadFile(tempFile)
	require.NoError(t, err)
	lines := strings.Split(strings.TrimSpace(string(content)), "\n")
	assert.Len(t, lines, 2)
}

func TestAppendToFile_EscapesSingleQuotes(t *testing.T) {
	setupTestIO(t)
	tempFile := filepath.Join(t.TempDir(), "escape-test")

	err := appendToFile(tempFile, "bash", []string{}, "/path/with'quote:/usr/bin")
	require.NoError(t, err)

	content, err := os.ReadFile(tempFile)
	require.NoError(t, err)
	assert.Contains(t, string(content), `'\''`)
}

func TestAppendToFile_EscapesDollarSign(t *testing.T) {
	setupTestIO(t)
	tempFile := filepath.Join(t.TempDir(), "ps-escape")

	err := appendToFile(tempFile, "powershell", []string{}, "/path/$var:/usr/bin")
	require.NoError(t, err)

	content, err := os.ReadFile(tempFile)
	require.NoError(t, err)
	assert.Contains(t, string(content), "`$")
}

func TestAppendToFile_ErrorOnInvalidPath(t *testing.T) {
	setupTestIO(t)

	err := appendToFile("/nonexistent/dir/file", "bash", []string{}, "/usr/bin")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to open file")
}

func TestAppendToFile_DefaultFormat(t *testing.T) {
	setupTestIO(t)
	tempFile := filepath.Join(t.TempDir(), "default")

	// Test unknown format falls back to bash-style export.
	err := appendToFile(tempFile, "unknown", []string{}, "/tools/bin:/usr/bin")
	require.NoError(t, err)

	content, err := os.ReadFile(tempFile)
	require.NoError(t, err)
	assert.Contains(t, string(content), "export PATH=")
}

func TestEmitGitHubEnv_MultiplePaths(t *testing.T) {
	setupTestIO(t)

	// Test with multiple paths - should not error.
	err := emitGitHubEnv([]string{"/path1", "/path2", "/path3"})
	assert.NoError(t, err)
}

func TestEmitGitHubEnv_EmptyPaths(t *testing.T) {
	setupTestIO(t)

	// Test with empty paths - should not error.
	err := emitGitHubEnv([]string{})
	assert.NoError(t, err)
}

func TestEmitGitHubEnv_SinglePath(t *testing.T) {
	setupTestIO(t)

	err := emitGitHubEnv([]string{"/single/path"})
	assert.NoError(t, err)
}

func TestEmitBashEnv(t *testing.T) {
	setupTestIO(t)

	err := emitBashEnv("/tools/bin:/usr/bin")
	assert.NoError(t, err)
}

func TestEmitBashEnv_WithSingleQuotes(t *testing.T) {
	setupTestIO(t)

	// Should properly escape single quotes.
	err := emitBashEnv("/path/with'quote:/usr/bin")
	assert.NoError(t, err)
}

func TestEmitDotenvEnv(t *testing.T) {
	setupTestIO(t)

	err := emitDotenvEnv("/tools/bin:/usr/bin")
	assert.NoError(t, err)
}

func TestEmitFishEnv(t *testing.T) {
	setupTestIO(t)

	err := emitFishEnv("/tools/bin:/usr/bin")
	assert.NoError(t, err)
}

func TestEmitPowershellEnv(t *testing.T) {
	setupTestIO(t)

	err := emitPowershellEnv("/tools/bin:/usr/bin")
	assert.NoError(t, err)
}

func TestEmitPowershellEnv_EscapesSpecialChars(t *testing.T) {
	setupTestIO(t)

	// Should escape double quotes and dollar signs.
	err := emitPowershellEnv(`/path/$var:/path/"quoted"`)
	assert.NoError(t, err)
}

func TestEmitJSONPath(t *testing.T) {
	setupTestIO(t)

	toolPaths := []ToolPath{
		{Tool: "terraform", Version: "1.5.0", Path: "/tools/terraform/1.5.0/bin"},
		{Tool: "helm", Version: "3.12.0", Path: "/tools/helm/3.12.0/bin"},
	}

	err := emitJSONPath(toolPaths, "/tools/terraform/1.5.0/bin:/tools/helm/3.12.0/bin:/usr/bin")
	assert.NoError(t, err)
}

func TestEmitJSONPath_EmptyTools(t *testing.T) {
	setupTestIO(t)

	err := emitJSONPath([]ToolPath{}, "/usr/bin")
	assert.NoError(t, err)
}

// Tests for format helper functions.

func TestFormatBashContent(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		expected string
	}{
		{
			name:     "simple path",
			path:     "/usr/bin:/usr/local/bin",
			expected: "export PATH='/usr/bin:/usr/local/bin'\n",
		},
		{
			name:     "path with single quote",
			path:     "/path/with'quote:/bin",
			expected: "export PATH='/path/with'\\''quote:/bin'\n",
		},
		{
			name:     "empty path",
			path:     "",
			expected: "export PATH=''\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatBashContent(tt.path)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestFormatDotenvContent(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		expected string
	}{
		{
			name:     "simple path",
			path:     "/usr/bin:/usr/local/bin",
			expected: "PATH='/usr/bin:/usr/local/bin'\n",
		},
		{
			name:     "path with single quote",
			path:     "/path/with'quote:/bin",
			expected: "PATH='/path/with'\\''quote:/bin'\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatDotenvContent(tt.path)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestFormatFishContent(t *testing.T) {
	sep := string(os.PathListSeparator)
	tests := []struct {
		name     string
		path     string
		contains []string
	}{
		{
			name:     "simple paths",
			path:     "/usr/bin" + sep + "/usr/local/bin",
			contains: []string{"set -gx PATH", "'/usr/bin'", "'/usr/local/bin'"},
		},
		{
			name:     "path with single quote",
			path:     "/path/with'quote" + sep + "/bin",
			contains: []string{"set -gx PATH", `\'`, "'/bin'"},
		},
		{
			name:     "single path",
			path:     "/usr/bin",
			contains: []string{"set -gx PATH", "'/usr/bin'"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatFishContent(tt.path)
			for _, c := range tt.contains {
				assert.Contains(t, result, c)
			}
		})
	}
}

func TestFormatPowershellContent(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		contains []string
	}{
		{
			name:     "simple path",
			path:     "/usr/bin:/usr/local/bin",
			contains: []string{"$env:PATH", "/usr/bin:/usr/local/bin"},
		},
		{
			name:     "path with double quote",
			path:     `/path/"quoted":/bin`,
			contains: []string{"$env:PATH", "`\""},
		},
		{
			name:     "path with dollar sign",
			path:     "/path/$var:/bin",
			contains: []string{"$env:PATH", "`$"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatPowershellContent(tt.path)
			for _, c := range tt.contains {
				assert.Contains(t, result, c)
			}
		})
	}
}

func TestFormatGitHubContent(t *testing.T) {
	tests := []struct {
		name        string
		pathEntries []string
		expected    string
	}{
		{
			name:        "multiple entries",
			pathEntries: []string{"/path1", "/path2", "/path3"},
			expected:    "/path1\n/path2\n/path3\n",
		},
		{
			name:        "single entry",
			pathEntries: []string{"/single"},
			expected:    "/single\n",
		},
		{
			name:        "empty entries",
			pathEntries: []string{},
			expected:    "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatGitHubContent(tt.pathEntries)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestFormatContentForFile(t *testing.T) {
	tests := []struct {
		name        string
		format      string
		pathEntries []string
		finalPath   string
		contains    string
	}{
		{
			name:        "github format",
			format:      "github",
			pathEntries: []string{"/path1", "/path2"},
			finalPath:   "",
			contains:    "/path1\n/path2\n",
		},
		{
			name:      "bash format",
			format:    "bash",
			finalPath: "/usr/bin",
			contains:  "export PATH=",
		},
		{
			name:      "dotenv format",
			format:    "dotenv",
			finalPath: "/usr/bin",
			contains:  "PATH=",
		},
		{
			name:      "fish format",
			format:    "fish",
			finalPath: "/usr/bin",
			contains:  "set -gx PATH",
		},
		{
			name:      "powershell format",
			format:    "powershell",
			finalPath: "/usr/bin",
			contains:  "$env:PATH",
		},
		{
			name:      "json format",
			format:    "json",
			finalPath: "/usr/bin",
			contains:  "final_path",
		},
		{
			name:      "unknown format defaults to bash",
			format:    "unknown-format",
			finalPath: "/usr/bin",
			contains:  "export PATH=",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatContentForFile(tt.format, tt.pathEntries, tt.finalPath)
			assert.Contains(t, result, tt.contains)
		})
	}
}
