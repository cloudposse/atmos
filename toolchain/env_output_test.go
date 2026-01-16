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
