package toolchain

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
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

func TestGetCurrentPath_EmptyFallback(t *testing.T) {
	// Test that empty PATH returns fallback system paths.
	t.Setenv("PATH", "")

	result := getCurrentPath()
	// Should return OS-specific fallback paths.
	if runtime.GOOS == "windows" {
		// On Windows, should contain System32 paths (if SystemRoot is set).
		// If no SystemRoot, result may be empty.
		if os.Getenv("SystemRoot") != "" || os.Getenv("WINDIR") != "" {
			assert.Contains(t, result, "System32")
		}
	} else {
		// On Unix, should return standard system directories.
		assert.Contains(t, result, "/usr/local/bin")
		assert.Contains(t, result, "/usr/bin")
		assert.Contains(t, result, "/bin")
	}
}

func TestConstructFinalPath_BothEmpty(t *testing.T) {
	// Edge case: both pathEntries and currentPath are empty.
	result := constructFinalPath([]string{}, "")
	// Should just be the separator.
	assert.Equal(t, string(os.PathListSeparator), result)
}

func TestResolveDirPath_RelativeReturnsDir(t *testing.T) {
	// Test that relative flag returns the directory portion.
	// Use filepath.FromSlash to convert to platform-appropriate path separators.
	inputPath := filepath.FromSlash("/home/user/.tools/terraform/1.5.0/bin/terraform")
	expectedDir := filepath.FromSlash("/home/user/.tools/terraform/1.5.0/bin")

	result, err := resolveDirPath(inputPath, true)
	assert.NoError(t, err)
	assert.Equal(t, expectedDir, result)
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

// TestBuildPathEntriesWithLocator_SuccessWithInstalledTools tests the success path when tools are installed and can be resolved.
func TestBuildPathEntriesWithLocator_SuccessWithInstalledTools(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockLocator := NewMockInstallLocator(ctrl)

	// Setup expectations.
	mockLocator.EXPECT().ParseToolSpec("terraform").Return("hashicorp", "terraform", nil)
	mockLocator.EXPECT().FindBinaryPath("hashicorp", "terraform", "1.5.0").
		Return(filepath.FromSlash("/tools/hashicorp/terraform/1.5.0/bin/terraform"), nil)

	toolVersions := &ToolVersions{
		Tools: map[string][]string{
			"terraform": {"1.5.0"},
		},
	}

	pathEntries, toolPaths, err := buildPathEntriesWithLocator(toolVersions, mockLocator, true)

	require.NoError(t, err)
	assert.Len(t, pathEntries, 1)
	assert.Len(t, toolPaths, 1)
	assert.Equal(t, "terraform", toolPaths[0].Tool)
	assert.Equal(t, "1.5.0", toolPaths[0].Version)
}

// TestBuildPathEntriesWithLocator_MultipleTools tests multiple tools being resolved.
func TestBuildPathEntriesWithLocator_MultipleTools(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockLocator := NewMockInstallLocator(ctrl)

	// Setup expectations for multiple tools.
	mockLocator.EXPECT().ParseToolSpec("terraform").Return("hashicorp", "terraform", nil)
	mockLocator.EXPECT().FindBinaryPath("hashicorp", "terraform", "1.5.0").
		Return(filepath.FromSlash("/tools/hashicorp/terraform/1.5.0/bin/terraform"), nil)

	mockLocator.EXPECT().ParseToolSpec("kubectl").Return("kubernetes", "kubectl", nil)
	mockLocator.EXPECT().FindBinaryPath("kubernetes", "kubectl", "1.28.0").
		Return(filepath.FromSlash("/tools/kubernetes/kubectl/1.28.0/bin/kubectl"), nil)

	toolVersions := &ToolVersions{
		Tools: map[string][]string{
			"terraform": {"1.5.0"},
			"kubectl":   {"1.28.0"},
		},
	}

	pathEntries, toolPaths, err := buildPathEntriesWithLocator(toolVersions, mockLocator, true)

	require.NoError(t, err)
	assert.Len(t, pathEntries, 2)
	assert.Len(t, toolPaths, 2)

	// Verify sorting (alphabetical by tool name).
	assert.Equal(t, "kubectl", toolPaths[0].Tool)
	assert.Equal(t, "terraform", toolPaths[1].Tool)
}

// TestBuildPathEntriesWithLocator_Deduplication tests that duplicate paths are deduplicated.
func TestBuildPathEntriesWithLocator_Deduplication(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockLocator := NewMockInstallLocator(ctrl)

	// Two tools that resolve to the same directory.
	sameBinDir := filepath.FromSlash("/tools/shared/bin/tool1")
	sameBinDir2 := filepath.FromSlash("/tools/shared/bin/tool2")

	mockLocator.EXPECT().ParseToolSpec("tool1").Return("owner", "tool1", nil)
	mockLocator.EXPECT().FindBinaryPath("owner", "tool1", "1.0.0").Return(sameBinDir, nil)

	mockLocator.EXPECT().ParseToolSpec("tool2").Return("owner", "tool2", nil)
	mockLocator.EXPECT().FindBinaryPath("owner", "tool2", "2.0.0").Return(sameBinDir2, nil)

	toolVersions := &ToolVersions{
		Tools: map[string][]string{
			"tool1": {"1.0.0"},
			"tool2": {"2.0.0"},
		},
	}

	pathEntries, toolPaths, err := buildPathEntriesWithLocator(toolVersions, mockLocator, true)

	require.NoError(t, err)
	// Path entries should be deduplicated (both tools in same bin dir).
	assert.Len(t, pathEntries, 1)
	// But toolPaths should contain both tools.
	assert.Len(t, toolPaths, 2)
}

// TestBuildPathEntriesWithLocator_MixedSuccessAndFailure tests when some tools resolve and some fail.
func TestBuildPathEntriesWithLocator_MixedSuccessAndFailure(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockLocator := NewMockInstallLocator(ctrl)

	// First tool succeeds.
	mockLocator.EXPECT().ParseToolSpec("terraform").Return("hashicorp", "terraform", nil)
	mockLocator.EXPECT().FindBinaryPath("hashicorp", "terraform", "1.5.0").
		Return(filepath.FromSlash("/tools/hashicorp/terraform/1.5.0/bin/terraform"), nil)

	// Second tool fails to resolve.
	mockLocator.EXPECT().ParseToolSpec("unknowntool").Return("", "", errors.New("tool not found"))

	toolVersions := &ToolVersions{
		Tools: map[string][]string{
			"terraform":   {"1.5.0"},
			"unknowntool": {"1.0.0"},
		},
	}

	pathEntries, toolPaths, err := buildPathEntriesWithLocator(toolVersions, mockLocator, true)

	require.NoError(t, err)
	// Should have the successful tool only.
	assert.Len(t, pathEntries, 1)
	assert.Len(t, toolPaths, 1)
	assert.Equal(t, "terraform", toolPaths[0].Tool)
}

// TestBuildPathEntriesWithLocator_FindBinaryPathFails tests when FindBinaryPath fails.
func TestBuildPathEntriesWithLocator_FindBinaryPathFails(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockLocator := NewMockInstallLocator(ctrl)

	// ParseToolSpec succeeds but FindBinaryPath fails.
	mockLocator.EXPECT().ParseToolSpec("terraform").Return("hashicorp", "terraform", nil)
	mockLocator.EXPECT().FindBinaryPath("hashicorp", "terraform", "1.5.0").
		Return("", errors.New("binary not found"))

	// Another tool succeeds.
	mockLocator.EXPECT().ParseToolSpec("kubectl").Return("kubernetes", "kubectl", nil)
	mockLocator.EXPECT().FindBinaryPath("kubernetes", "kubectl", "1.28.0").
		Return(filepath.FromSlash("/tools/kubernetes/kubectl/1.28.0/bin/kubectl"), nil)

	toolVersions := &ToolVersions{
		Tools: map[string][]string{
			"terraform": {"1.5.0"},
			"kubectl":   {"1.28.0"},
		},
	}

	pathEntries, toolPaths, err := buildPathEntriesWithLocator(toolVersions, mockLocator, true)

	require.NoError(t, err)
	// Should only have kubectl.
	assert.Len(t, pathEntries, 1)
	assert.Len(t, toolPaths, 1)
	assert.Equal(t, "kubectl", toolPaths[0].Tool)
}

// TestBuildPathEntriesWithLocator_EmptyVersionsSkipped tests that tools with empty version arrays are skipped.
func TestBuildPathEntriesWithLocator_EmptyVersionsSkipped(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockLocator := NewMockInstallLocator(ctrl)

	// Only terraform should be processed (kubectl has empty versions).
	mockLocator.EXPECT().ParseToolSpec("terraform").Return("hashicorp", "terraform", nil)
	mockLocator.EXPECT().FindBinaryPath("hashicorp", "terraform", "1.5.0").
		Return(filepath.FromSlash("/tools/hashicorp/terraform/1.5.0/bin/terraform"), nil)

	toolVersions := &ToolVersions{
		Tools: map[string][]string{
			"terraform": {"1.5.0"},
			"kubectl":   {}, // Empty versions should be skipped.
		},
	}

	pathEntries, toolPaths, err := buildPathEntriesWithLocator(toolVersions, mockLocator, true)

	require.NoError(t, err)
	assert.Len(t, pathEntries, 1)
	assert.Len(t, toolPaths, 1)
	assert.Equal(t, "terraform", toolPaths[0].Tool)
}

// TestBuildPathEntriesWithLocator_NoToolsFound tests that ErrToolNotFound is returned when no tools resolve.
func TestBuildPathEntriesWithLocator_NoToolsFound(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockLocator := NewMockInstallLocator(ctrl)

	// All tools fail to resolve.
	mockLocator.EXPECT().ParseToolSpec("terraform").Return("", "", errors.New("not found"))
	mockLocator.EXPECT().ParseToolSpec("kubectl").Return("", "", errors.New("not found"))

	toolVersions := &ToolVersions{
		Tools: map[string][]string{
			"terraform": {"1.5.0"},
			"kubectl":   {"1.28.0"},
		},
	}

	_, _, err := buildPathEntriesWithLocator(toolVersions, mockLocator, true)

	require.Error(t, err)
	assert.ErrorIs(t, err, ErrToolNotFound)
}

// TestBuildPathEntriesWithLocator_UsesFirstVersion tests that only the first version is used.
func TestBuildPathEntriesWithLocator_UsesFirstVersion(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockLocator := NewMockInstallLocator(ctrl)

	// Should only call FindBinaryPath with the first version.
	mockLocator.EXPECT().ParseToolSpec("terraform").Return("hashicorp", "terraform", nil)
	mockLocator.EXPECT().FindBinaryPath("hashicorp", "terraform", "1.5.0").
		Return(filepath.FromSlash("/tools/hashicorp/terraform/1.5.0/bin/terraform"), nil)

	toolVersions := &ToolVersions{
		Tools: map[string][]string{
			"terraform": {"1.5.0", "1.4.0", "1.3.0"}, // Multiple versions, only first should be used.
		},
	}

	pathEntries, toolPaths, err := buildPathEntriesWithLocator(toolVersions, mockLocator, true)

	require.NoError(t, err)
	assert.Len(t, pathEntries, 1)
	assert.Len(t, toolPaths, 1)
	assert.Equal(t, "1.5.0", toolPaths[0].Version)
}

// TestBuildPathEntriesWithLocator_RelativeFlagFalse tests absolute path resolution.
func TestBuildPathEntriesWithLocator_RelativeFlagFalse(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockLocator := NewMockInstallLocator(ctrl)

	// Use a relative path to test absolute conversion.
	mockLocator.EXPECT().ParseToolSpec("terraform").Return("hashicorp", "terraform", nil)
	mockLocator.EXPECT().FindBinaryPath("hashicorp", "terraform", "1.5.0").
		Return(filepath.FromSlash(".tools/terraform/bin/terraform"), nil)

	toolVersions := &ToolVersions{
		Tools: map[string][]string{
			"terraform": {"1.5.0"},
		},
	}

	pathEntries, toolPaths, err := buildPathEntriesWithLocator(toolVersions, mockLocator, false)

	require.NoError(t, err)
	assert.Len(t, pathEntries, 1)
	// With relativeFlag=false, path should be absolute.
	assert.True(t, filepath.IsAbs(pathEntries[0]), "Expected absolute path, got: %s", pathEntries[0])
	assert.True(t, filepath.IsAbs(toolPaths[0].Path), "Expected absolute path, got: %s", toolPaths[0].Path)
}

// TestBuildPathEntriesWithLocator_SortingConsistency tests that output is sorted alphabetically.
func TestBuildPathEntriesWithLocator_SortingConsistency(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockLocator := NewMockInstallLocator(ctrl)

	// Setup tools in non-alphabetical order.
	mockLocator.EXPECT().ParseToolSpec("zebra").Return("owner", "zebra", nil)
	mockLocator.EXPECT().FindBinaryPath("owner", "zebra", "1.0.0").
		Return(filepath.FromSlash("/tools/zebra/bin/zebra"), nil)

	mockLocator.EXPECT().ParseToolSpec("alpha").Return("owner", "alpha", nil)
	mockLocator.EXPECT().FindBinaryPath("owner", "alpha", "1.0.0").
		Return(filepath.FromSlash("/tools/alpha/bin/alpha"), nil)

	mockLocator.EXPECT().ParseToolSpec("middle").Return("owner", "middle", nil)
	mockLocator.EXPECT().FindBinaryPath("owner", "middle", "1.0.0").
		Return(filepath.FromSlash("/tools/middle/bin/middle"), nil)

	toolVersions := &ToolVersions{
		Tools: map[string][]string{
			"zebra":  {"1.0.0"},
			"alpha":  {"1.0.0"},
			"middle": {"1.0.0"},
		},
	}

	pathEntries, toolPaths, err := buildPathEntriesWithLocator(toolVersions, mockLocator, true)

	require.NoError(t, err)
	assert.Len(t, pathEntries, 3)
	assert.Len(t, toolPaths, 3)

	// Verify toolPaths are sorted alphabetically.
	assert.Equal(t, "alpha", toolPaths[0].Tool)
	assert.Equal(t, "middle", toolPaths[1].Tool)
	assert.Equal(t, "zebra", toolPaths[2].Tool)
}
