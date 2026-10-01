package toolchain

import (
	"bytes"
	"encoding/json"
	stdio "io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/data"
	iolib "github.com/cloudposse/atmos/pkg/io"
	"github.com/cloudposse/atmos/pkg/schema"
)

// testStreams is a minimal io.Streams implementation for capturing data-channel output in tests.
type testStreams struct {
	stdin  stdio.Reader
	stdout stdio.Writer
	stderr stdio.Writer
}

func (ts *testStreams) Input() stdio.Reader     { return ts.stdin }
func (ts *testStreams) Output() stdio.Writer    { return ts.stdout }
func (ts *testStreams) Error() stdio.Writer     { return ts.stderr }
func (ts *testStreams) RawOutput() stdio.Writer { return ts.stdout }
func (ts *testStreams) RawError() stdio.Writer  { return ts.stderr }

// installVersionForTest makes FindBinaryPath resolve owner/repo@version as installed by
// pointing ATMOS_XDG_CACHE_HOME at a temp dir and placing an executable at the exact path
// GetBinaryPath expects: <cache>/atmos/toolchain/bin/<owner>/<repo>/<version>/<repo>.
func installVersionForTest(t *testing.T, owner, repo, version string) {
	t.Helper()
	cacheDir := t.TempDir()
	t.Setenv("ATMOS_XDG_CACHE_HOME", cacheDir)

	versionDir := filepath.Join(cacheDir, "atmos", "toolchain", "bin", owner, repo, version)
	require.NoError(t, os.MkdirAll(versionDir, defaultMkdirPermissions))
	require.NoError(t, os.WriteFile(filepath.Join(versionDir, repo), []byte("fake binary"), 0o755))
}

// captureDataOutput redirects the pkg/data writer to a buffer for the duration of the test.
func captureDataOutput(t *testing.T) *bytes.Buffer {
	t.Helper()
	stdout := &bytes.Buffer{}
	streams := &testStreams{stdin: &bytes.Buffer{}, stdout: stdout, stderr: &bytes.Buffer{}}
	ioCtx, err := iolib.NewContext(iolib.WithStreams(streams))
	require.NoError(t, err)
	data.InitWriter(ioCtx)
	t.Cleanup(data.Reset)
	return stdout
}

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
	// Use 0o755 for executable binary files (defaultMkdirPermissions is for directories)
	err = os.WriteFile(binaryPath, []byte("fake binary"), 0o755)
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
				t.Setenv("HOME", tt.binaryDir)
			}

			// Avoid mutating tt.filePath directly; use a local variable.
			filePath := tt.filePath
			if filePath == "" {
				filePath = createTempToolVersionsFile(t, tt.toolVersions)
			}
			SetAtmosConfig(&schema.AtmosConfiguration{
				Toolchain: schema.Toolchain{
					VersionsFile: filePath,
				},
			})
			// Run the function
			err := ListToolVersions(tt.showAll, tt.limit, tt.toolName, "table")

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

func TestListToolVersions_FormatPlain(t *testing.T) {
	filePath := createTempToolVersionsFile(t, "owner/repo 1.0.0 2.0.0\n")
	binaryDir := createTempBinary(t, "owner", "repo", "1.0.0")
	t.Setenv("HOME", binaryDir)
	SetAtmosConfig(&schema.AtmosConfiguration{
		Toolchain: schema.Toolchain{VersionsFile: filePath},
	})

	stdout := captureDataOutput(t)

	err := ListToolVersions(false, 0, "owner/repo", "plain")
	require.NoError(t, err)
	assert.Equal(t, "1.0.0\n", stdout.String())
}

func TestListToolVersions_FormatPlainWithAllRejected(t *testing.T) {
	err := ListToolVersions(true, 10, "owner/repo", "plain")
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrToolchainPlainFormatWithAllFlag)
}

func TestListToolVersions_UnsupportedFormatRejected(t *testing.T) {
	// A direct caller passing an unvalidated format (bypassing the cmd-layer check) must still
	// be rejected rather than silently falling back to table output.
	err := ListToolVersions(false, 10, "owner/repo", "xml")
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrInvalidFlagValue)
}

func TestListToolVersions_FormatJSON(t *testing.T) {
	filePath := createTempToolVersionsFile(t, "owner/repo 1.0.0 2.0.0\n")
	installVersionForTest(t, "owner", "repo", "1.0.0")
	SetAtmosConfig(&schema.AtmosConfiguration{
		Toolchain: schema.Toolchain{VersionsFile: filePath},
	})

	stdout := captureDataOutput(t)

	err := ListToolVersions(false, 0, "owner/repo", "json")
	require.NoError(t, err)

	var got ToolVersionInfo
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &got))
	assert.Equal(t, ToolVersionInfo{Tool: "owner/repo", Version: "1.0.0", Installed: true}, got)
}

func TestListToolVersions_FormatJSONWithAll(t *testing.T) {
	filePath := createTempToolVersionsFile(t, "owner/repo 1.0.0\n")
	installVersionForTest(t, "owner", "repo", "1.0.0")
	SetAtmosConfig(&schema.AtmosConfiguration{
		Toolchain: schema.Toolchain{VersionsFile: filePath},
	})

	mock := NewMockGitHubAPI()
	mock.SetReleases("owner", "repo", []string{"1.0.0"})
	SetGitHubAPI(mock)
	t.Cleanup(ResetGitHubAPI)

	stdout := captureDataOutput(t)

	err := ListToolVersions(true, 5, "owner/repo", "json")
	require.NoError(t, err)

	var got ToolVersionList
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &got))
	assert.Equal(t, "owner/repo", got.Tool)
	assert.NotEmpty(t, got.Versions)
	require.Len(t, got.Versions, 1)
	assert.Equal(t, "1.0.0", got.Versions[0].Version)
	assert.True(t, got.Versions[0].Installed)
	assert.True(t, got.Versions[0].Default)
}

func TestGetDefaultFromFile(t *testing.T) {
	tests := []struct {
		name        string
		fileContent string
		resolvedKey string
		toolName    string
		expected    string
	}{
		{
			name:        "Returns default version for resolved key",
			fileContent: "hashicorp/terraform 1.5.7 1.5.6\n",
			resolvedKey: "hashicorp/terraform",
			toolName:    "terraform",
			expected:    "1.5.7",
		},
		{
			name:        "Falls back to tool name",
			fileContent: "terraform 1.5.7 1.5.6\n",
			resolvedKey: "hashicorp/terraform",
			toolName:    "terraform",
			expected:    "1.5.7",
		},
		{
			name:        "Returns empty for non-existent tool",
			fileContent: "terraform 1.5.7\n",
			resolvedKey: "hashicorp/vault",
			toolName:    "vault",
			expected:    "",
		},
		{
			name:        "Returns empty for non-existent file",
			fileContent: "",
			resolvedKey: "hashicorp/terraform",
			toolName:    "terraform",
			expected:    "",
		},
		{
			name:        "Returns empty for empty versions",
			fileContent: "terraform \n",
			resolvedKey: "hashicorp/terraform",
			toolName:    "terraform",
			expected:    "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var filePath string
			if tt.fileContent != "" {
				tempDir := t.TempDir()
				filePath = filepath.Join(tempDir, DefaultToolVersionsFilePath)
				err := os.WriteFile(filePath, []byte(tt.fileContent), defaultFileWritePermissions)
				require.NoError(t, err)
			} else {
				filePath = "/nonexistent/path/.tool-versions"
			}

			result := getDefaultFromFile(filePath, tt.resolvedKey, tt.toolName)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestMin(t *testing.T) {
	tests := []struct {
		name     string
		a        int
		b        int
		expected int
	}{
		{
			name:     "a is smaller",
			a:        5,
			b:        10,
			expected: 5,
		},
		{
			name:     "b is smaller",
			a:        10,
			b:        5,
			expected: 5,
		},
		{
			name:     "equal values",
			a:        7,
			b:        7,
			expected: 7,
		},
		{
			name:     "negative values",
			a:        -5,
			b:        -10,
			expected: -10,
		},
		{
			name:     "zero values",
			a:        0,
			b:        5,
			expected: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := min(tt.a, tt.b)
			assert.Equal(t, tt.expected, result)
		})
	}
}
