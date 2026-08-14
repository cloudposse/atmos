package toolchain

import (
	"path/filepath"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAddToolToVersionsConcurrentUpdatesPreserveAllTools(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".tool-versions")
	var wg sync.WaitGroup
	for _, spec := range [][2]string{{"hashicorp/terraform", "1.0.0"}, {"helm/helm", "3.0.0"}} {
		wg.Add(1)
		go func(tool, version string) {
			defer wg.Done()
			assert.NoError(t, AddToolToVersions(path, tool, version))
		}(spec[0], spec[1])
	}
	wg.Wait()

	versions, err := LoadToolVersions(path)
	require.NoError(t, err)
	assert.Equal(t, []string{"1.0.0"}, versions.Tools["hashicorp/terraform"])
	assert.Equal(t, []string{"3.0.0"}, versions.Tools["helm/helm"])
}

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

// setupToolchainTestEnv sets up the test environment with HOME and ATMOS_TOOLCHAIN_PATH.
func setupToolchainTestEnv(t *testing.T, tempDir string) {
	t.Helper()
	t.Setenv("HOME", tempDir)
	t.Setenv("ATMOS_TOOLCHAIN_PATH", tempDir)
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

		// asDefault mirrors asdf's own "set" convention (asdf's docs describe `asdf set
		// <tool> <version>` as equivalent to `echo "<tool> <version>" > .tool-versions`):
		// the whole line becomes exactly the new version, full stop. Callers (set,
		// add --default, update) all document that replacing the default never leaves a
		// stale extra version pinned.
		toolVersions, err = LoadToolVersions(filePath)
		require.NoError(t, err)
		versions := toolVersions.Tools["terraform"]
		assert.Equal(t, []string{"1.5.7"}, versions,
			"setting a new default fully replaces the line -- 1.5.5 and 1.5.6 must not survive as stale entries")
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
		assert.Equal(t, []string{"1.5.6"}, versions,
			"setting an already-pinned version as default still fully replaces the line, matching asdf's set semantics")
	})

	t.Run("Returns error for empty version", func(t *testing.T) {
		tempDir := t.TempDir()
		filePath := filepath.Join(tempDir, DefaultToolVersionsFilePath)

		err := AddToolToVersionsAsDefault(filePath, "terraform", "")
		assert.Error(t, err)
		assert.ErrorIs(t, err, ErrInvalidToolSpec)
	})

	t.Run("Sets an already-tracked version as the sole default under the same key", func(t *testing.T) {
		// Regression test: "atmos toolchain set jq 1.7.1" must make 1.7.1 the sole
		// entry when .tool-versions already contains "jq 1.9.0 1.7.1" -- both
		// versions tracked under the same "jq" key. AddVersionToTool's asDefault
		// path always fully replaces (see its doc comment): the stale 1.9.0 must
		// not survive as a second entry.
		tempDir := t.TempDir()
		filePath := filepath.Join(tempDir, DefaultToolVersionsFilePath)

		err := AddToolToVersions(filePath, "jq", "1.9.0")
		require.NoError(t, err)
		err = AddToolToVersions(filePath, "jq", "1.7.1")
		require.NoError(t, err)

		err = AddToolToVersionsAsDefault(filePath, "jq", "1.7.1")
		require.NoError(t, err)

		toolVersions, err := LoadToolVersions(filePath)
		require.NoError(t, err)
		assert.Equal(t, []string{"1.7.1"}, toolVersions.Tools["jq"])
	})

	t.Run("Promotes an already-tracked version under a different (alias/canonical) key", func(t *testing.T) {
		// Regression test: when the version is already tracked under a different
		// key form than the one the caller passed (e.g. the file stores the
		// canonical "opentofu/opentofu" entry but the caller asks to promote a
		// version by the "opentofu" alias), findDuplicateKey finds the conflict.
		// Setting asDefault=true must still promote the version within its
		// existing key instead of silently doing nothing -- and, per
		// AddVersionToTool's asDefault contract, fully replace the list there
		// rather than leaving the old version pinned alongside it.
		tempDir := t.TempDir()
		filePath := filepath.Join(tempDir, DefaultToolVersionsFilePath)

		err := AddToolToVersions(filePath, "opentofu/opentofu", "1.10.3")
		require.NoError(t, err)
		err = AddToolToVersions(filePath, "opentofu/opentofu", "1.10.2")
		require.NoError(t, err)

		// Promote 1.10.2 to default using the alias form of the tool name.
		err = AddToolToVersionsAsDefault(filePath, "opentofu", "1.10.2")
		require.NoError(t, err)

		toolVersions, err := LoadToolVersions(filePath)
		require.NoError(t, err)
		assert.Equal(t, []string{"1.10.2"}, toolVersions.Tools["opentofu/opentofu"])
		assert.NotContains(t, toolVersions.Tools, "opentofu", "should not create a second, disconnected alias entry")
	})

	// TestAddToolToVersionsAsDefault/Single-version_tool_ends_up_with_exactly_one_version
	// reproduces the most common real-world case (a tool pinned to a single version, e.g. from
	// `add`, then bumped via `set`, `add --default`, or `update`). set's and update's own docs
	// promise a tool is never left pinned to two versions at once -- this is the simplest
	// possible repro of that guarantee failing: the old default was silently kept as a stale
	// second entry instead of being replaced.
	t.Run("Single-version tool ends up with exactly one version", func(t *testing.T) {
		tempDir := t.TempDir()
		filePath := filepath.Join(tempDir, DefaultToolVersionsFilePath)

		toolVersions := &ToolVersions{Tools: make(map[string][]string)}
		AddVersionToTool(toolVersions, "jqlang/jq", "1.7.1", false)
		err := SaveToolVersions(filePath, toolVersions)
		require.NoError(t, err)

		setupToolchainTestEnv(t, tempDir)

		err = AddToolToVersionsAsDefault(filePath, "jqlang/jq", "1.8.2")
		require.NoError(t, err)

		toolVersions, err = LoadToolVersions(filePath)
		require.NoError(t, err)
		versions := toolVersions.Tools["jqlang/jq"]
		assert.Equal(t, []string{"1.8.2"}, versions,
			"a single-version tool must end up pinned to exactly the new version -- the old default must not survive as a stale second entry")
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
			result := LookupToolVersionOrLatest(tt.tool, tt.toolVersions, resolver)

			assert.Equal(t, tt.expectedKey, result.ResolvedKey, "resolvedKey mismatch")
			assert.Equal(t, tt.expectedVersion, result.Version, "version mismatch")
			assert.Equal(t, tt.expectedFound, result.Found, "found mismatch")
			assert.Equal(t, tt.expectedLatest, result.UsedLatest, "usedLatest mismatch")
		})
	}
}
