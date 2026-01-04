package toolchain

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestListCommand_WithInstalledTools(t *testing.T) {
	tempDir := t.TempDir()
	t.Chdir(tempDir)
	t.Log(tempDir)
	toolVersionsFile := filepath.Join(tempDir, DefaultToolVersionsFilePath)
	toolsDir := filepath.Join(tempDir, ".tools")
	atmosConfig := &schema.AtmosConfiguration{
		Toolchain: schema.Toolchain{
			VersionsFile: toolVersionsFile,
			ToolsDir:     toolsDir,
		},
	}

	// Create a .tool-versions file with some tools
	toolVersions := &ToolVersions{
		Tools: map[string][]string{
			"terraform": {"1.11.4"},
			"kubectl":   {"1.28.0"},
		},
	}
	err := SaveToolVersions(atmosConfig.Toolchain.VersionsFile, toolVersions)
	require.NoError(t, err)
	// Create mock installed binaries
	terraformPath := filepath.Join(toolsDir, "bin", "hashicorp", "terraform", "1.11.4")
	err = os.MkdirAll(terraformPath, defaultMkdirPermissions)
	require.NoError(t, err)

	terraformBinary := filepath.Join(terraformPath, "terraform")
	err = os.WriteFile(terraformBinary, []byte("mock terraform binary"), defaultMkdirPermissions)
	require.NoError(t, err)

	// Set modification time for testing
	modTime := time.Date(2023, 12, 1, 10, 30, 0, 0, time.UTC)
	err = os.Chtimes(terraformBinary, modTime, modTime)
	require.NoError(t, err)

	kubectlPath := filepath.Join(toolsDir, "bin", "kubernetes", "kubectl", "1.28.0")
	err = os.MkdirAll(kubectlPath, defaultMkdirPermissions)
	require.NoError(t, err)

	kubectlBinary := filepath.Join(kubectlPath, "kubectl")
	err = os.WriteFile(kubectlBinary, []byte("mock kubectl binary"), defaultMkdirPermissions)
	require.NoError(t, err)

	// Test listing tools
	SetAtmosConfig(atmosConfig)
	err = RunList()
	require.NoError(t, err, "Should successfully list installed tools")
}

func TestListCommand_EmptyToolVersionsFile(t *testing.T) {
	tempDir := t.TempDir()
	t.Chdir(tempDir)
	toolVersionsFile := filepath.Join(tempDir, DefaultToolVersionsFilePath)

	// Create an empty .tool-versions file
	toolVersions := &ToolVersions{
		Tools: map[string][]string{},
	}
	err := SaveToolVersions(toolVersionsFile, toolVersions)
	require.NoError(t, err)

	SetAtmosConfig(&schema.AtmosConfiguration{
		Toolchain: schema.Toolchain{
			VersionsFile: toolVersionsFile,
		},
	})
	err = RunList()

	require.NoError(t, err, "Should handle empty tool-versions file gracefully")
}

func TestListCommand_NonExistentToolVersionsFile(t *testing.T) {
	tempDir := t.TempDir()
	t.Chdir(tempDir)
	// Test listing with non-existent file
	SetAtmosConfig(&schema.AtmosConfiguration{
		Toolchain: schema.Toolchain{
			VersionsFile: filepath.Join(tempDir, "non-existent"),
		},
	})
	err := RunList()
	require.NoError(t, err, "Should handle missing tool-versions file gracefully with helpful message")
}

func TestListCommand_ToolsNotInstalled(t *testing.T) {
	tempDir := t.TempDir()
	toolVersionsFile := filepath.Join(tempDir, DefaultToolVersionsFilePath)

	// Create a .tool-versions file with tools that aren't installed
	toolVersions := &ToolVersions{
		Tools: map[string][]string{
			"terraform": {"1.11.4"},
			"kubectl":   {"1.28.0"},
		},
	}
	err := SaveToolVersions(toolVersionsFile, toolVersions)
	require.NoError(t, err)
	SetAtmosConfig(&schema.AtmosConfiguration{
		Toolchain: schema.Toolchain{
			VersionsFile: toolVersionsFile,
		},
	})
	err = RunList()
	require.NoError(t, err, "Should handle tools that aren't installed gracefully")
}

func TestListCommand_MixedInstalledAndNotInstalled(t *testing.T) {
	tempDir := t.TempDir()
	toolVersionsFile := filepath.Join(tempDir, DefaultToolVersionsFilePath)
	toolsDir := filepath.Join(tempDir, ".tools")

	// Create a .tool-versions file with mixed tools
	toolVersions := &ToolVersions{
		Tools: map[string][]string{
			"terraform": {"1.11.4"}, // This will be installed
			"kubectl":   {"1.28.0"}, // This will not be installed
		},
	}
	err := SaveToolVersions(toolVersionsFile, toolVersions)
	require.NoError(t, err)

	// Create only terraform binary
	terraformPath := filepath.Join(toolsDir, "bin", "hashicorp", "terraform", "1.11.4")
	err = os.MkdirAll(terraformPath, defaultMkdirPermissions)
	require.NoError(t, err)

	terraformBinary := filepath.Join(terraformPath, "terraform")
	err = os.WriteFile(terraformBinary, []byte("mock terraform binary"), defaultMkdirPermissions)
	require.NoError(t, err)

	SetAtmosConfig(&schema.AtmosConfiguration{
		Toolchain: schema.Toolchain{
			VersionsFile: toolVersionsFile,
			ToolsDir:     toolsDir,
		},
	})
	err = RunList()
	require.NoError(t, err, "Should list only installed tools")
}

func TestListCommand_WithLatestVersion(t *testing.T) {
	tempDir := t.TempDir()
	t.Chdir(tempDir)
	toolVersionsFile := filepath.Join(tempDir, DefaultToolVersionsFilePath)
	toolsDir := filepath.Join(tempDir, ".tools")

	// Create a .tool-versions file with latest version
	toolVersions := &ToolVersions{
		Tools: map[string][]string{
			"terraform": {"latest"},
		},
	}
	err := SaveToolVersions(toolVersionsFile, toolVersions)
	require.NoError(t, err)

	// Create mock latest file
	latestPath := filepath.Join(toolsDir, "bin", "hashicorp", "terraform", "latest")
	err = os.MkdirAll(latestPath, defaultMkdirPermissions)
	require.NoError(t, err)

	latestFile := filepath.Join(latestPath, "latest")
	err = os.WriteFile(latestFile, []byte("1.11.4"), defaultFileWritePermissions)
	require.NoError(t, err)

	terraformBinary := filepath.Join(latestPath, "terraform")
	err = os.WriteFile(terraformBinary, []byte("mock terraform binary"), defaultMkdirPermissions)
	require.NoError(t, err)

	SetAtmosConfig(&schema.AtmosConfiguration{
		Toolchain: schema.Toolchain{
			VersionsFile: toolVersionsFile,
			ToolsDir:     toolsDir,
		},
	})
	// Test listing with latest version
	err = RunList()
	require.NoError(t, err, "Should handle latest version correctly")
}

func TestListCommand_NoArgs(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	SetAtmosConfig(&schema.AtmosConfiguration{})
	err := RunList()
	// Should handle missing .tool-versions file gracefully
	require.NoError(t, err, "Should handle missing tool-versions file gracefully with helpful message")
}

func TestFormatFileSize(t *testing.T) {
	tests := []struct {
		size     int64
		expected string
	}{
		{0, "0 B"},
		{1023, "1023 B"},
		{1024, "1.0 KB"},
		{1536, "1.5 KB"},
		{1048576, "1.0 MB"},
		{1572864, "1.5 MB"},
		{1073741824, "1.0 GB"},
		{1610612736, "1.5 GB"},
	}

	for _, test := range tests {
		result := formatFileSize(test.size)
		assert.Equal(t, test.expected, result, "Size %d should format to %s", test.size, test.expected)
	}
}

func TestListCommand_WithCanonicalNames(t *testing.T) {
	tempDir := t.TempDir()
	toolVersionsFile := filepath.Join(tempDir, DefaultToolVersionsFilePath)
	toolsDir := filepath.Join(tempDir, ".tools")

	// Create a .tool-versions file with canonical names
	toolVersions := &ToolVersions{
		Tools: map[string][]string{
			"hashicorp/terraform": {"1.11.4"},
		},
	}
	err := SaveToolVersions(toolVersionsFile, toolVersions)
	require.NoError(t, err)

	// Create mock installed binary with canonical name
	terraformPath := filepath.Join(toolsDir, "bin", "hashicorp", "terraform", "1.11.4")
	err = os.MkdirAll(terraformPath, defaultMkdirPermissions)
	require.NoError(t, err)

	terraformBinary := filepath.Join(terraformPath, "terraform")
	err = os.WriteFile(terraformBinary, []byte("mock terraform binary"), defaultMkdirPermissions)
	require.NoError(t, err)
	SetAtmosConfig(&schema.AtmosConfiguration{
		Toolchain: schema.Toolchain{
			VersionsFile: toolVersionsFile,
			ToolsDir:     toolsDir,
		},
	})
	// Test listing with canonical names
	err = RunList()
	require.NoError(t, err, "Should handle canonical names correctly")
}

func TestListCommand_WithMultipleVersions(t *testing.T) {
	tempDir := t.TempDir()
	toolVersionsFile := filepath.Join(tempDir, DefaultToolVersionsFilePath)
	toolsDir := filepath.Join(tempDir, ".tools")

	// Create a .tool-versions file with multiple versions
	toolVersions := &ToolVersions{
		Tools: map[string][]string{
			"terraform": {"1.11.4", "1.9.8"},
		},
	}
	err := SaveToolVersions(toolVersionsFile, toolVersions)
	require.NoError(t, err)

	// Create mock installed binaries for both versions
	terraformPath1 := filepath.Join(toolsDir, "bin", "hashicorp", "terraform", "1.11.4")
	err = os.MkdirAll(terraformPath1, defaultMkdirPermissions)
	require.NoError(t, err)

	terraformBinary1 := filepath.Join(terraformPath1, "terraform")
	err = os.WriteFile(terraformBinary1, []byte("mock terraform binary"), defaultMkdirPermissions)
	require.NoError(t, err)

	terraformPath2 := filepath.Join(toolsDir, "bin", "hashicorp", "terraform", "1.9.8")
	err = os.MkdirAll(terraformPath2, defaultMkdirPermissions)
	require.NoError(t, err)

	terraformBinary2 := filepath.Join(terraformPath2, "terraform")
	err = os.WriteFile(terraformBinary2, []byte("mock terraform binary"), defaultMkdirPermissions)
	require.NoError(t, err)

	SetAtmosConfig(&schema.AtmosConfiguration{
		Toolchain: schema.Toolchain{
			VersionsFile: toolVersionsFile,
			ToolsDir:     toolsDir,
		},
	})
	err = RunList()
	require.NoError(t, err, "Should handle multiple versions correctly")
}

// Tests for Atmos version listing functions.

func TestNormalizeVersion(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"v1.0.0", "1.0.0"},
		{"1.0.0", "1.0.0"},
		{"v2.3.4", "2.3.4"},
		{"", ""},
		{"v", ""},
		{"latest", "latest"},
	}

	for _, tt := range tests {
		result := normalizeVersion(tt.input)
		assert.Equal(t, tt.expected, result, "normalizeVersion(%q) should return %q", tt.input, tt.expected)
	}
}

func TestSortAtmosVersionRows(t *testing.T) {
	rows := []atmosVersionRow{
		{version: "1.0.0"},
		{version: "2.0.0"},
		{version: "1.5.0"},
		{version: "1.10.0"},
	}

	sortAtmosVersionRows(rows)

	// Should be sorted newest first.
	assert.Equal(t, "2.0.0", rows[0].version)
	assert.Equal(t, "1.10.0", rows[1].version)
	assert.Equal(t, "1.5.0", rows[2].version)
	assert.Equal(t, "1.0.0", rows[3].version)
}

func TestSortAtmosVersionRows_InvalidSemver(t *testing.T) {
	rows := []atmosVersionRow{
		{version: "abc"},
		{version: "xyz"},
		{version: "def"},
	}

	// Should not panic with invalid semver, falls back to lexicographic.
	sortAtmosVersionRows(rows)

	assert.Equal(t, "xyz", rows[0].version)
	assert.Equal(t, "def", rows[1].version)
	assert.Equal(t, "abc", rows[2].version)
}

func TestRunListAtmosVersions_NoInstalledVersions(t *testing.T) {
	tempDir := t.TempDir()

	SetAtmosConfig(&schema.AtmosConfiguration{
		Toolchain: schema.Toolchain{
			InstallPath: tempDir,
		},
	})

	versions, err := RunListAtmosVersions()
	require.NoError(t, err)
	assert.Empty(t, versions, "Should return empty list when no versions installed")
}

func TestRunListAtmosVersions_WithInstalledVersions(t *testing.T) {
	tempDir := t.TempDir()

	// Create mock installed Atmos binaries (binDir = InstallPath + "/bin").
	atmosPath1 := filepath.Join(tempDir, "bin", "cloudposse", "atmos", "1.0.0")
	err := os.MkdirAll(atmosPath1, defaultMkdirPermissions)
	require.NoError(t, err)
	err = os.WriteFile(filepath.Join(atmosPath1, "atmos"), []byte("mock"), defaultMkdirPermissions)
	require.NoError(t, err)

	atmosPath2 := filepath.Join(tempDir, "bin", "cloudposse", "atmos", "1.1.0")
	err = os.MkdirAll(atmosPath2, defaultMkdirPermissions)
	require.NoError(t, err)
	err = os.WriteFile(filepath.Join(atmosPath2, "atmos"), []byte("mock"), defaultMkdirPermissions)
	require.NoError(t, err)

	SetAtmosConfig(&schema.AtmosConfiguration{
		Toolchain: schema.Toolchain{
			InstallPath: tempDir,
		},
	})

	versions, err := RunListAtmosVersions()
	require.NoError(t, err)
	assert.Len(t, versions, 2, "Should return 2 installed versions")
	assert.Contains(t, versions, "1.0.0")
	assert.Contains(t, versions, "1.1.0")
}

func TestRunListInstalledAtmosVersions_NoVersions(t *testing.T) {
	tempDir := t.TempDir()

	SetAtmosConfig(&schema.AtmosConfiguration{
		Toolchain: schema.Toolchain{
			InstallPath: tempDir,
		},
	})

	err := RunListInstalledAtmosVersions("1.0.0")
	require.NoError(t, err, "Should handle no installed versions gracefully")
}

func TestRunListInstalledAtmosVersions_WithVersions(t *testing.T) {
	tempDir := t.TempDir()

	// Create mock installed Atmos binary.
	atmosPath := filepath.Join(tempDir, "bin", "cloudposse", "atmos", "1.199.0")
	err := os.MkdirAll(atmosPath, defaultMkdirPermissions)
	require.NoError(t, err)
	err = os.WriteFile(filepath.Join(atmosPath, "atmos"), []byte("mock atmos binary"), defaultMkdirPermissions)
	require.NoError(t, err)

	// Set modification time.
	modTime := time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC)
	err = os.Chtimes(filepath.Join(atmosPath, "atmos"), modTime, modTime)
	require.NoError(t, err)

	SetAtmosConfig(&schema.AtmosConfiguration{
		Toolchain: schema.Toolchain{
			InstallPath: tempDir,
		},
	})

	err = RunListInstalledAtmosVersions("1.199.0")
	require.NoError(t, err, "Should display installed versions table")
}

func TestRunListInstalledAtmosVersions_CurrentVersionNotInstalled(t *testing.T) {
	tempDir := t.TempDir()

	// Create mock installed Atmos binary for a different version.
	atmosPath := filepath.Join(tempDir, "bin", "cloudposse", "atmos", "1.198.0")
	err := os.MkdirAll(atmosPath, defaultMkdirPermissions)
	require.NoError(t, err)
	err = os.WriteFile(filepath.Join(atmosPath, "atmos"), []byte("mock"), defaultMkdirPermissions)
	require.NoError(t, err)

	SetAtmosConfig(&schema.AtmosConfiguration{
		Toolchain: schema.Toolchain{
			InstallPath: tempDir,
		},
	})

	// Current version is 1.200.0 which is not installed.
	err = RunListInstalledAtmosVersions("1.200.0")
	require.NoError(t, err, "Should handle current version not being in installed list")
}

func TestBuildAtmosVersionRowsSimple(t *testing.T) {
	tempDir := t.TempDir()

	// Create mock installed Atmos binary.
	atmosPath := filepath.Join(tempDir, "bin", "cloudposse", "atmos", "1.199.0")
	err := os.MkdirAll(atmosPath, defaultMkdirPermissions)
	require.NoError(t, err)
	binaryPath := filepath.Join(atmosPath, "atmos")
	err = os.WriteFile(binaryPath, []byte("mock atmos binary content"), defaultMkdirPermissions)
	require.NoError(t, err)

	SetAtmosConfig(&schema.AtmosConfiguration{
		Toolchain: schema.Toolchain{
			InstallPath: tempDir,
		},
	})

	installer := NewInstaller()
	versions := []string{"1.199.0"}

	rows := buildAtmosVersionRowsSimple(installer, versions, "1.199.0")

	require.Len(t, rows, 1)
	assert.Equal(t, "1.199.0", rows[0].version)
	assert.True(t, rows[0].isActive, "Current version should be marked as active")
	assert.NotEqual(t, notAvailablePlaceholder, rows[0].size)
	assert.NotEqual(t, notAvailablePlaceholder, rows[0].installDate)
}

func TestBuildAtmosVersionRowsSimple_DevBuild(t *testing.T) {
	tempDir := t.TempDir()

	// Create mock installed Atmos binary for different version.
	atmosPath := filepath.Join(tempDir, "bin", "cloudposse", "atmos", "1.198.0")
	err := os.MkdirAll(atmosPath, defaultMkdirPermissions)
	require.NoError(t, err)
	err = os.WriteFile(filepath.Join(atmosPath, "atmos"), []byte("mock"), defaultMkdirPermissions)
	require.NoError(t, err)

	SetAtmosConfig(&schema.AtmosConfiguration{
		Toolchain: schema.Toolchain{
			InstallPath: tempDir,
		},
	})

	installer := NewInstaller()
	versions := []string{"1.198.0"}

	// Current version is a dev build not in installed versions.
	rows := buildAtmosVersionRowsSimple(installer, versions, "1.200.0-dev")

	require.Len(t, rows, 2, "Should include installed version + current dev version")

	// Find the dev version row.
	var devRow *atmosVersionRow
	for i := range rows {
		if rows[i].version == "1.200.0-dev (current)" {
			devRow = &rows[i]
			break
		}
	}
	require.NotNil(t, devRow, "Should include current dev version")
	assert.True(t, devRow.isActive)
}

// Tests for table column width functions.

func TestTruncateColumnsProportionally(t *testing.T) {
	// Test when excess is small.
	widths := columnWidths{
		alias:       20,
		registry:    30,
		binary:      15,
		version:     15,
		status:      3,
		installDate: 20,
		size:        12,
	}

	// Reduce by 20 chars.
	result := truncateColumnsProportionally(widths, 20)

	// Verify that columns were reduced.
	assert.Less(t, result.registry, widths.registry, "Registry should be reduced")
	assert.Less(t, result.alias, widths.alias, "Alias should be reduced")
	assert.Equal(t, widths.status, result.status, "Status should not be reduced")
}

func TestTruncateColumnsProportionally_LargeExcess(t *testing.T) {
	widths := columnWidths{
		alias:       20,
		registry:    30,
		binary:      15,
		version:     15,
		status:      3,
		installDate: 20,
		size:        12,
	}

	// Try to reduce by more than possible - should hit minimums.
	result := truncateColumnsProportionally(widths, 100)

	// Should not go below minimums.
	assert.GreaterOrEqual(t, result.alias, 6, "Alias should not go below minimum")
	assert.GreaterOrEqual(t, result.registry, 8, "Registry should not go below minimum")
	assert.GreaterOrEqual(t, result.binary, 6, "Binary should not go below minimum")
	assert.GreaterOrEqual(t, result.version, 8, "Version should not go below minimum")
	assert.GreaterOrEqual(t, result.installDate, 12, "InstallDate should not go below minimum")
	assert.GreaterOrEqual(t, result.size, 8, "Size should not go below minimum")
}

func TestCalculateColumnWidths_NarrowTerminal(t *testing.T) {
	rows := []toolRow{
		{
			alias:       "tf",
			registry:    "hashicorp/terraform",
			binary:      "terraform",
			version:     "1.0.0",
			status:      " ●",
			installDate: "2024-01-01 10:00",
			size:        "100 MB",
		},
	}

	// Calculate widths for wide terminal first.
	wideWidths := calculateColumnWidths(rows, 200)
	wideTotal := calculateTotalWidth(wideWidths)

	// Narrow terminal should trigger truncation and result in smaller widths.
	narrowWidths := calculateColumnWidths(rows, 80)
	narrowTotal := calculateTotalWidth(narrowWidths)

	// Narrow should be less than or equal to wide.
	assert.LessOrEqual(t, narrowTotal, wideTotal, "Narrow terminal should produce smaller or equal total width")
}

func TestCalculateColumnWidths_WideTerminal(t *testing.T) {
	rows := []toolRow{
		{
			alias:       "tf",
			registry:    "hashicorp/terraform",
			binary:      "terraform",
			version:     "1.0.0",
			status:      " ●",
			installDate: "2024-01-01 10:00",
			size:        "100 MB",
		},
	}

	// Wide terminal should not need truncation.
	widths := calculateColumnWidths(rows, 200)

	// Should have reasonable widths.
	assert.Greater(t, widths.registry, 10)
	assert.Greater(t, widths.binary, 5)
}

func TestEnsureMinimumHeaderWidths(t *testing.T) {
	// Start with zero widths.
	widths := columnWidths{}

	result := ensureMinimumHeaderWidths(widths)

	// Should have minimum header widths.
	assert.GreaterOrEqual(t, result.alias, len("ALIAS"))
	assert.GreaterOrEqual(t, result.registry, len("REGISTRY"))
	assert.GreaterOrEqual(t, result.binary, len("BINARY"))
	assert.GreaterOrEqual(t, result.version, len("VERSION"))
	assert.GreaterOrEqual(t, result.installDate, len("INSTALL DATE"))
	assert.GreaterOrEqual(t, result.size, len("SIZE"))
}

func TestAddColumnBuffers(t *testing.T) {
	widths := columnWidths{
		alias:       10,
		registry:    10,
		binary:      10,
		version:     10,
		status:      2,
		installDate: 10,
		size:        10,
	}

	result := addColumnBuffers(widths)

	// Each column should have buffer added (except status which gets 1).
	assert.Equal(t, widths.alias+4, result.alias)
	assert.Equal(t, widths.registry+4, result.registry)
	assert.Equal(t, widths.binary+4, result.binary)
	assert.Equal(t, widths.version+4, result.version)
	assert.Equal(t, widths.status+1, result.status)
	assert.Equal(t, widths.installDate+4, result.installDate)
	assert.Equal(t, widths.size+4, result.size)
}

func TestCalculateContentWidths(t *testing.T) {
	rows := []toolRow{
		{
			alias:       "short",
			registry:    "medium/registry",
			binary:      "bin",
			version:     "1.0.0",
			status:      " ●",
			installDate: "2024-01-01 10:00",
			size:        "100 MB",
		},
		{
			alias:       "longer-alias",
			registry:    "very-long-org/very-long-registry-name",
			binary:      "longer-binary",
			version:     "1.10.0",
			status:      " ●",
			installDate: "2024-01-01 10:00",
			size:        "1.5 GB",
		},
	}

	widths := calculateContentWidths(rows)

	// Should pick the maximum from each column.
	assert.Equal(t, len("longer-alias"), widths.alias)
	assert.Equal(t, len("very-long-org/very-long-registry-name"), widths.registry)
	assert.Equal(t, len("longer-binary"), widths.binary)
	assert.Equal(t, len("1.10.0"), widths.version)
}
