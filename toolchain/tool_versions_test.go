package toolchain

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAddToolToVersionsDuplicateCheck(t *testing.T) {
	tempDir := t.TempDir()
	filePath := filepath.Join(tempDir, DefaultToolVersionsFilePath)

	// First, add the full name version
	err := AddToolToVersions(filePath, "opentofu/opentofu", "1.10.3")
	require.NoError(t, err)

	// Load the file to verify it was added
	toolVersions, err := LoadToolVersions(filePath)
	require.NoError(t, err)
	assert.Contains(t, toolVersions.Tools, "opentofu/opentofu")
	assert.Equal(t, []string{"1.10.3"}, toolVersions.Tools["opentofu/opentofu"])

	// Now try to add the alias version - this should be skipped due to duplicate check
	err = AddToolToVersions(filePath, "opentofu", "1.10.3")
	require.NoError(t, err) // Should not error, but should skip adding

	// Load the file again to verify the alias was NOT added
	toolVersions, err = LoadToolVersions(filePath)
	require.NoError(t, err)
	assert.Contains(t, toolVersions.Tools, "opentofu/opentofu")
	assert.NotContains(t, toolVersions.Tools, "opentofu") // Should not have the alias
	assert.Equal(t, []string{"1.10.3"}, toolVersions.Tools["opentofu/opentofu"])
}

func TestAddToolToVersionsReverseDuplicateCheck(t *testing.T) {
	tempDir := t.TempDir()
	filePath := filepath.Join(tempDir, DefaultToolVersionsFilePath)

	// First, add the alias version
	err := AddToolToVersions(filePath, "opentofu", "1.10.3")
	require.NoError(t, err)

	// Load the file to verify it was added
	toolVersions, err := LoadToolVersions(filePath)
	require.NoError(t, err)
	assert.Contains(t, toolVersions.Tools, "opentofu")
	assert.Equal(t, []string{"1.10.3"}, toolVersions.Tools["opentofu"])

	// Now try to add the full name version - this should be skipped due to duplicate check
	err = AddToolToVersions(filePath, "opentofu/opentofu", "1.10.3")
	require.NoError(t, err) // Should not error, but should skip adding

	// Load the file again to verify the full name was NOT added
	toolVersions, err = LoadToolVersions(filePath)
	require.NoError(t, err)
	assert.Contains(t, toolVersions.Tools, "opentofu")
	assert.NotContains(t, toolVersions.Tools, "opentofu/opentofu") // Should not have the full name
	assert.Equal(t, []string{"1.10.3"}, toolVersions.Tools["opentofu"])
}

func TestAddToolToVersionsDuplicateCheckWithExistingAlias(t *testing.T) {
	tempDir := t.TempDir()
	filePath := filepath.Join(tempDir, DefaultToolVersionsFilePath)

	// First, add the alias version (this simulates what happens when someone manually adds "opentofu 1.10.2")
	err := AddToolToVersions(filePath, "opentofu", "1.10.2")
	require.NoError(t, err)

	// Load the file to verify it was added
	toolVersions, err := LoadToolVersions(filePath)
	require.NoError(t, err)
	assert.Contains(t, toolVersions.Tools, "opentofu")
	assert.Equal(t, []string{"1.10.2"}, toolVersions.Tools["opentofu"])

	// Now try to add the full name version - this should be skipped due to duplicate check
	// This simulates what happens when InstallSingleTool calls AddToolToVersions with "opentofu/opentofu"
	err = AddToolToVersions(filePath, "opentofu/opentofu", "1.10.2")
	require.NoError(t, err) // Should not error, but should skip adding

	// Load the file again to verify the full name was NOT added
	toolVersions, err = LoadToolVersions(filePath)
	require.NoError(t, err)
	assert.Contains(t, toolVersions.Tools, "opentofu")
	assert.NotContains(t, toolVersions.Tools, "opentofu/opentofu") // Should not have the full name
	assert.Equal(t, []string{"1.10.2"}, toolVersions.Tools["opentofu"])
}

func TestAddToolToVersionsDuplicateCheckWithMultipleVersions(t *testing.T) {
	tempDir := t.TempDir()
	filePath := filepath.Join(tempDir, DefaultToolVersionsFilePath)

	// First, add the alias version with multiple versions (like in your example)
	err := AddToolToVersions(filePath, "opentofu", "1.10.3")
	require.NoError(t, err)
	err = AddToolToVersions(filePath, "opentofu", "1.10.2")
	require.NoError(t, err)

	// Load the file to verify it was added
	toolVersions, err := LoadToolVersions(filePath)
	require.NoError(t, err)
	assert.Contains(t, toolVersions.Tools, "opentofu")
	assert.Equal(t, []string{"1.10.3", "1.10.2"}, toolVersions.Tools["opentofu"])

	// Now try to add the full name version for 1.10.2 - this should be skipped due to duplicate check
	err = AddToolToVersions(filePath, "opentofu/opentofu", "1.10.2")
	require.NoError(t, err) // Should not error, but should skip adding

	// Load the file again to verify the full name was NOT added
	toolVersions, err = LoadToolVersions(filePath)
	require.NoError(t, err)
	assert.Contains(t, toolVersions.Tools, "opentofu")
	assert.NotContains(t, toolVersions.Tools, "opentofu/opentofu") // Should not have the full name
	assert.Equal(t, []string{"1.10.3", "1.10.2"}, toolVersions.Tools["opentofu"])
}

func TestGetDefaultVersion(t *testing.T) {
	tests := []struct {
		name           string
		toolVersions   *ToolVersions
		tool           string
		expectedVer    string
		expectedExists bool
	}{
		{
			name: "Tool with versions returns first version",
			toolVersions: &ToolVersions{
				Tools: map[string][]string{
					"terraform": {"1.5.7", "1.5.6", "1.5.5"},
				},
			},
			tool:           "terraform",
			expectedVer:    "1.5.7",
			expectedExists: true,
		},
		{
			name: "Tool with no versions returns empty and false",
			toolVersions: &ToolVersions{
				Tools: map[string][]string{
					"terraform": {},
				},
			},
			tool:           "terraform",
			expectedVer:    "",
			expectedExists: false,
		},
		{
			name: "Non-existent tool returns empty and false",
			toolVersions: &ToolVersions{
				Tools: map[string][]string{
					"terraform": {"1.5.7"},
				},
			},
			tool:           "opentofu",
			expectedVer:    "",
			expectedExists: false,
		},
		{
			name: "Empty ToolVersions returns empty and false",
			toolVersions: &ToolVersions{
				Tools: map[string][]string{},
			},
			tool:           "terraform",
			expectedVer:    "",
			expectedExists: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			version, exists := GetDefaultVersion(tt.toolVersions, tt.tool)
			assert.Equal(t, tt.expectedVer, version)
			assert.Equal(t, tt.expectedExists, exists)
		})
	}
}

func TestGetAllVersions(t *testing.T) {
	tests := []struct {
		name             string
		toolVersions     *ToolVersions
		tool             string
		expectedVersions []string
	}{
		{
			name: "Tool with multiple versions",
			toolVersions: &ToolVersions{
				Tools: map[string][]string{
					"terraform": {"1.5.7", "1.5.6", "1.5.5"},
				},
			},
			tool:             "terraform",
			expectedVersions: []string{"1.5.7", "1.5.6", "1.5.5"},
		},
		{
			name: "Tool with no versions returns nil slice",
			toolVersions: &ToolVersions{
				Tools: map[string][]string{
					"terraform": {},
				},
			},
			tool:             "terraform",
			expectedVersions: []string{},
		},
		{
			name: "Non-existent tool returns nil",
			toolVersions: &ToolVersions{
				Tools: map[string][]string{
					"terraform": {"1.5.7"},
				},
			},
			tool:             "opentofu",
			expectedVersions: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			versions := GetAllVersions(tt.toolVersions, tt.tool)
			assert.Equal(t, tt.expectedVersions, versions)
		})
	}
}

// setupToolchainTestEnv sets up the test environment with HOME, ATMOS_TOOLS_DIR, and tools.yaml.
func setupToolchainTestEnv(t *testing.T, tempDir string) {
	t.Helper()
	t.Setenv("HOME", tempDir)
	t.Setenv("ATMOS_TOOLS_DIR", tempDir)
	toolsConfigPath := filepath.Join(tempDir, "tools.yaml")
	err := os.WriteFile(toolsConfigPath, []byte("aliases:\n  terraform: hashicorp/terraform\n"), 0o644)
	require.NoError(t, err)
	t.Setenv("ATMOS_TOOLS_CONFIG_FILE", toolsConfigPath)
}

func TestAddToolToVersionsAsDefault(t *testing.T) {
	t.Run("Adds tool as default (first position)", func(t *testing.T) {
		tempDir := t.TempDir()
		filePath := filepath.Join(tempDir, DefaultToolVersionsFilePath)

		// Add initial versions using internal test helper that bypasses duplicate checking
		toolVersions := &ToolVersions{Tools: make(map[string][]string)}
		AddVersionToTool(toolVersions, "terraform", "1.5.5", false)
		AddVersionToTool(toolVersions, "terraform", "1.5.6", false)
		err := SaveToolVersions(filePath, toolVersions)
		require.NoError(t, err)

		// Set up test environment
		setupToolchainTestEnv(t, tempDir)

		// Add as default
		err = AddToolToVersionsAsDefault(filePath, "terraform", "1.5.7")
		require.NoError(t, err)

		// Verify it's first
		toolVersions, err = LoadToolVersions(filePath)
		require.NoError(t, err)
		versions := toolVersions.Tools["terraform"]
		require.NotEmpty(t, versions)
		assert.Equal(t, "1.5.7", versions[0], "Default version should be first")
		assert.Contains(t, versions, "1.5.5")
		assert.Contains(t, versions, "1.5.6")
	})

	t.Run("Updates existing tool to default", func(t *testing.T) {
		tempDir := t.TempDir()
		filePath := filepath.Join(tempDir, DefaultToolVersionsFilePath)

		// Set up initial versions
		toolVersions := &ToolVersions{Tools: make(map[string][]string)}
		AddVersionToTool(toolVersions, "terraform", "1.5.5", false)
		AddVersionToTool(toolVersions, "terraform", "1.5.6", false)
		AddVersionToTool(toolVersions, "terraform", "1.5.7", false)
		err := SaveToolVersions(filePath, toolVersions)
		require.NoError(t, err)

		// Set up test environment
		setupToolchainTestEnv(t, tempDir)

		// Set existing version as default
		err = AddToolToVersionsAsDefault(filePath, "terraform", "1.5.6")
		require.NoError(t, err)

		toolVersions, err = LoadToolVersions(filePath)
		require.NoError(t, err)
		versions := toolVersions.Tools["terraform"]
		assert.Equal(t, "1.5.6", versions[0], "1.5.6 should be first")
	})

	t.Run("Returns error for empty version", func(t *testing.T) {
		tempDir := t.TempDir()
		filePath := filepath.Join(tempDir, DefaultToolVersionsFilePath)

		err := AddToolToVersionsAsDefault(filePath, "terraform", "")
		assert.Error(t, err)
		assert.ErrorIs(t, err, ErrInvalidToolSpec)
	})
}

func TestLookupToolVersion(t *testing.T) {
	tests := []struct {
		name            string
		tool            string
		toolVersions    *ToolVersions
		mapping         map[string][2]string // mock resolver mapping
		expectedKey     string
		expectedVersion string
		expectedFound   bool
	}{
		{
			name: "raw tool name found",
			tool: "terraform",
			toolVersions: &ToolVersions{
				Tools: map[string][]string{
					"terraform": {"1.5.7"},
				},
			},
			mapping:         nil,
			expectedKey:     "terraform",
			expectedVersion: "1.5.7",
			expectedFound:   true,
		},
		{
			name: "alias resolves and found",
			tool: "terraform",
			toolVersions: &ToolVersions{
				Tools: map[string][]string{
					"hashicorp/terraform": {"1.6.0"},
				},
			},
			mapping: map[string][2]string{
				"terraform": {"hashicorp", "terraform"},
			},
			expectedKey:     "hashicorp/terraform",
			expectedVersion: "1.6.0",
			expectedFound:   true,
		},
		{
			name: "not found - no alias",
			tool: "kubectl",
			toolVersions: &ToolVersions{
				Tools: map[string][]string{},
			},
			mapping:         nil,
			expectedKey:     "",
			expectedVersion: "",
			expectedFound:   false,
		},
		{
			name: "not found - alias resolves but not in versions",
			tool: "kubectl",
			toolVersions: &ToolVersions{
				Tools: map[string][]string{},
			},
			mapping: map[string][2]string{
				"kubectl": {"kubernetes", "kubectl"},
			},
			expectedKey:     "",
			expectedVersion: "",
			expectedFound:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resolver := &mockToolResolver{mapping: tt.mapping}
			key, version, found := LookupToolVersion(tt.tool, tt.toolVersions, resolver)

			assert.Equal(t, tt.expectedKey, key, "resolvedKey mismatch")
			assert.Equal(t, tt.expectedVersion, version, "version mismatch")
			assert.Equal(t, tt.expectedFound, found, "found mismatch")
		})
	}
}

func TestLookupToolVersionOrLatest(t *testing.T) {
	tests := []struct {
		name            string
		tool            string
		toolVersions    *ToolVersions
		mapping         map[string][2]string
		expectedKey     string
		expectedVersion string
		expectedFound   bool
		expectedLatest  bool
	}{
		{
			name: "Tool found by raw name",
			tool: "terraform",
			toolVersions: &ToolVersions{
				Tools: map[string][]string{
					"terraform": {"1.5.7", "1.5.6"},
				},
			},
			mapping:         map[string][2]string{},
			expectedKey:     "terraform",
			expectedVersion: "1.5.7",
			expectedFound:   true,
			expectedLatest:  false,
		},
		{
			name: "Tool found by alias resolution",
			tool: "terraform",
			toolVersions: &ToolVersions{
				Tools: map[string][]string{
					"hashicorp/terraform": {"1.5.7"},
				},
			},
			mapping: map[string][2]string{
				"terraform": {"hashicorp", "terraform"},
			},
			expectedKey:     "hashicorp/terraform",
			expectedVersion: "1.5.7",
			expectedFound:   true,
			expectedLatest:  false,
		},
		{
			name: "Alias resolves but not in toolVersions returns latest",
			tool: "terraform",
			toolVersions: &ToolVersions{
				Tools: map[string][]string{},
			},
			mapping: map[string][2]string{
				"terraform": {"hashicorp", "terraform"},
			},
			expectedKey:     "hashicorp/terraform",
			expectedVersion: "latest",
			expectedFound:   false,
			expectedLatest:  true,
		},
		{
			name: "Tool not found and no alias resolution",
			tool: "unknowntool",
			toolVersions: &ToolVersions{
				Tools: map[string][]string{},
			},
			mapping:         map[string][2]string{},
			expectedKey:     "",
			expectedVersion: "",
			expectedFound:   false,
			expectedLatest:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resolver := &mockToolResolver{mapping: tt.mapping}
			key, version, found, usedLatest := LookupToolVersionOrLatest(tt.tool, tt.toolVersions, resolver)

			assert.Equal(t, tt.expectedKey, key, "resolvedKey mismatch")
			assert.Equal(t, tt.expectedVersion, version, "version mismatch")
			assert.Equal(t, tt.expectedFound, found, "found mismatch")
			assert.Equal(t, tt.expectedLatest, usedLatest, "usedLatest mismatch")
		})
	}
}
