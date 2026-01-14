package toolchain

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/charmbracelet/bubbles/progress"
	bspinner "github.com/charmbracelet/bubbles/spinner"

	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Add a mock ToolResolver for tests.

// newMockSpinner creates a spinner model for testing.
func newMockSpinner() *bspinner.Model {
	s := bspinner.New()
	return &s
}

// newMockProgressBar creates a progress bar model for testing.
func newMockProgressBar() *progress.Model {
	p := progress.New()
	return &p
}

func TestInstallResolvesAliasFromToolVersions(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Chdir(dir)

	// Write a .tool-versions file with the full alias key
	toolVersionsPath := filepath.Join(dir, DefaultToolVersionsFilePath)
	err := os.WriteFile(toolVersionsPath, []byte("opentofu/opentofu 1.10.0\n"), defaultFileWritePermissions)
	require.NoError(t, err)

	// Write a minimal tools.yaml with the alias
	toolsYamlPath := filepath.Join(dir, "tools.yaml")

	toolsYaml := `aliases:
  opentofu: opentofu/opentofu
`
	err = os.WriteFile(toolsYamlPath, []byte(toolsYaml), defaultFileWritePermissions)
	require.NoError(t, err)

	// Simulate install: should resolve alias 'opentofu' to 'opentofu/opentofu'
	mockResolver := &mockToolResolver{
		mapping: map[string][2]string{
			"terraform": {"hashicorp", "terraform"},
			"opentofu":  {"opentofu", "opentofu"},
			"kubectl":   {"kubernetes", "kubectl"},
			"helm":      {"helm", "helm"},
			"helmfile":  {"helmfile", "helmfile"},
		},
	}
	prevConfig := atmosConfig
	SetAtmosConfig(&schema.AtmosConfiguration{
		Toolchain: schema.Toolchain{
			FilePath: toolVersionsPath,
		},
	})
	t.Cleanup(func() { SetAtmosConfig(prevConfig) })
	binDir := filepath.Join(dir, ".atmos", "tools", "bin")
	installer := NewInstallerWithResolver(mockResolver, binDir)
	owner, repo, err := installer.ParseToolSpec("opentofu")
	assert.NoError(t, err)
	assert.Equal(t, "opentofu", owner)
	assert.Equal(t, "opentofu", repo)

	// Now test the install logic (mock actual install)
	// This should not error, as the alias is resolved and found in .tool-versions
	// We'll just call the lookup logic directly
	toolVersions, err := LoadToolVersions(toolVersionsPath)
	assert.NoError(t, err)
	version, exists := toolVersions.Tools[owner+"/"+repo]
	assert.True(t, exists)
	assert.Equal(t, []string{"1.10.0"}, version)
}

func TestRunInstallWithNoArgs(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("HOME", tempDir)

	// Create a .tool-versions file with some tools
	toolVersionsPath := filepath.Join(tempDir, DefaultToolVersionsFilePath)
	toolVersions := &ToolVersions{
		Tools: map[string][]string{
			"terraform": {"1.11.4"},
			"helm":      {"3.17.4"},
		},
	}
	err := SaveToolVersions(toolVersionsPath, toolVersions)
	require.NoError(t, err)

	// Temporarily set the global toolVersionsFile variable
	prev := atmosConfig
	SetAtmosConfig(&schema.AtmosConfiguration{Toolchain: schema.Toolchain{VersionsFile: toolVersionsPath}})
	t.Cleanup(func() { SetAtmosConfig(prev) })

	// Test that runInstall with no arguments doesn't error
	// This prevents regression where the function might error when no specific tool is provided
	err = RunInstall("", false, false, true, false)
	assert.NoError(t, err)
}

// TestRunInstall_WithValidToolSpec tests RunInstall with a valid tool@version specification.
func TestRunInstall_WithValidToolSpec(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("HOME", tempDir)

	// Create a .tool-versions file
	toolVersionsPath := filepath.Join(tempDir, DefaultToolVersionsFilePath)
	toolVersions := &ToolVersions{
		Tools: map[string][]string{},
	}
	err := SaveToolVersions(toolVersionsPath, toolVersions)
	require.NoError(t, err)

	// Set Atmos config
	originalToolVersionsFile := GetToolVersionsFilePath()
	SetAtmosConfig(&schema.AtmosConfiguration{Toolchain: schema.Toolchain{VersionsFile: toolVersionsPath}})
	defer func() {
		SetAtmosConfig(&schema.AtmosConfiguration{Toolchain: schema.Toolchain{VersionsFile: originalToolVersionsFile}})
	}()

	// Test installing a specific tool with version
	err = RunInstall("terraform@1.11.4", false, false, true, false)
	assert.NoError(t, err)

	// Verify the tool was added to .tool-versions (it uses DefaultToolVersionsFilePath which is .tool-versions in HOME)
	actualPath := DefaultToolVersionsFilePath
	updatedToolVersions, err := LoadToolVersions(actualPath)
	require.NoError(t, err)
	assert.Contains(t, updatedToolVersions.Tools, "terraform")
	assert.Contains(t, updatedToolVersions.Tools["terraform"], "1.11.4")
}

// TestRunInstall_WithSetAsDefault tests RunInstall with setAsDefault flag.
func TestRunInstall_WithSetAsDefault(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("HOME", tempDir)

	// Create a .tool-versions file with existing versions
	toolVersionsPath := filepath.Join(tempDir, DefaultToolVersionsFilePath)
	toolVersions := &ToolVersions{
		Tools: map[string][]string{
			"terraform": {"1.11.3", "1.11.2"},
		},
	}
	err := SaveToolVersions(toolVersionsPath, toolVersions)
	require.NoError(t, err)

	// Set Atmos config
	originalToolVersionsFile := GetToolVersionsFilePath()
	SetAtmosConfig(&schema.AtmosConfiguration{Toolchain: schema.Toolchain{VersionsFile: toolVersionsPath}})
	defer func() {
		SetAtmosConfig(&schema.AtmosConfiguration{Toolchain: schema.Toolchain{VersionsFile: originalToolVersionsFile}})
	}()

	// Test installing with setAsDefault=true
	err = RunInstall("terraform@1.11.4", true, false, true, false)
	assert.NoError(t, err)

	// Verify the new version is first (default) in .tool-versions (uses DefaultToolVersionsFilePath)
	actualPath := DefaultToolVersionsFilePath
	updatedToolVersions, err := LoadToolVersions(actualPath)
	require.NoError(t, err)
	assert.Contains(t, updatedToolVersions.Tools, "terraform")
	assert.Equal(t, "1.11.4", updatedToolVersions.Tools["terraform"][0])
}

// TestRunInstall_WithInvalidToolSpec tests RunInstall with an invalid tool specification.
func TestRunInstall_WithInvalidToolSpec(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("HOME", tempDir)

	// Create a .tool-versions file
	toolVersionsPath := filepath.Join(tempDir, DefaultToolVersionsFilePath)
	toolVersions := &ToolVersions{
		Tools: map[string][]string{},
	}
	err := SaveToolVersions(toolVersionsPath, toolVersions)
	require.NoError(t, err)

	// Set Atmos config
	originalToolVersionsFile := GetToolVersionsFilePath()
	SetAtmosConfig(&schema.AtmosConfiguration{Toolchain: schema.Toolchain{VersionsFile: toolVersionsPath}})
	defer func() {
		SetAtmosConfig(&schema.AtmosConfiguration{Toolchain: schema.Toolchain{VersionsFile: originalToolVersionsFile}})
	}()

	// Test with invalid tool spec (no version)
	err = RunInstall("nonexistent-tool", false, false, true, false)
	assert.Error(t, err)
}

// TestRunInstall_WithCanonicalFormat tests RunInstall with owner/repo@version format.
func TestRunInstall_WithCanonicalFormat(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("HOME", tempDir)

	// Create a .tool-versions file
	toolVersionsPath := filepath.Join(tempDir, DefaultToolVersionsFilePath)
	toolVersions := &ToolVersions{
		Tools: map[string][]string{},
	}
	err := SaveToolVersions(toolVersionsPath, toolVersions)
	require.NoError(t, err)

	// Set Atmos config
	originalToolVersionsFile := GetToolVersionsFilePath()
	SetAtmosConfig(&schema.AtmosConfiguration{Toolchain: schema.Toolchain{VersionsFile: toolVersionsPath}})
	defer func() {
		SetAtmosConfig(&schema.AtmosConfiguration{Toolchain: schema.Toolchain{VersionsFile: originalToolVersionsFile}})
	}()

	// Test installing with canonical owner/repo@version format
	err = RunInstall("hashicorp/terraform@1.11.4", false, false, true, false)
	assert.NoError(t, err)

	// Verify the tool was added to .tool-versions (uses DefaultToolVersionsFilePath)
	// Note: The tool may be registered as "terraform" or "hashicorp/terraform" depending on alias resolution
	actualPath := DefaultToolVersionsFilePath
	updatedToolVersions, err := LoadToolVersions(actualPath)
	require.NoError(t, err)
	// Check for either key - the implementation may normalize to the shorter form
	terraformKey := ""
	if _, exists := updatedToolVersions.Tools["terraform"]; exists {
		terraformKey = "terraform"
	} else if _, exists := updatedToolVersions.Tools["hashicorp/terraform"]; exists {
		terraformKey = "hashicorp/terraform"
	}
	assert.NotEmpty(t, terraformKey, "Tool should be registered as either 'terraform' or 'hashicorp/terraform'")
	assert.Contains(t, updatedToolVersions.Tools[terraformKey], "1.11.4")
}

// TestRunInstall_WithLatestKeyword tests RunInstall with the "latest" version keyword.
func TestRunInstall_WithLatestKeyword(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("HOME", tempDir)

	// Create a .tool-versions file
	toolVersionsPath := filepath.Join(tempDir, DefaultToolVersionsFilePath)
	toolVersions := &ToolVersions{
		Tools: map[string][]string{},
	}
	err := SaveToolVersions(toolVersionsPath, toolVersions)
	require.NoError(t, err)

	// Set Atmos config
	originalToolVersionsFile := GetToolVersionsFilePath()
	SetAtmosConfig(&schema.AtmosConfiguration{Toolchain: schema.Toolchain{VersionsFile: toolVersionsPath}})
	defer func() {
		SetAtmosConfig(&schema.AtmosConfiguration{Toolchain: schema.Toolchain{VersionsFile: originalToolVersionsFile}})
	}()

	// Test installing with "latest" version
	// This should resolve to the actual latest version from the registry
	err = RunInstall("terraform@latest", false, false, true, false)
	assert.NoError(t, err)

	// Verify a version was added (we can't predict the exact version, but it should be there)
	actualPath := DefaultToolVersionsFilePath
	updatedToolVersions, err := LoadToolVersions(actualPath)
	require.NoError(t, err)
	assert.Contains(t, updatedToolVersions.Tools, "terraform")
	assert.NotEmpty(t, updatedToolVersions.Tools["terraform"])
}

// TestRunInstall_Reinstall tests RunInstall with reinstallFlag=true.
func TestRunInstall_Reinstall(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("HOME", tempDir)

	// Create a .tool-versions file with existing tools
	toolVersionsPath := filepath.Join(tempDir, DefaultToolVersionsFilePath)
	toolVersions := &ToolVersions{
		Tools: map[string][]string{
			"terraform": {"1.11.4"},
		},
	}
	err := SaveToolVersions(toolVersionsPath, toolVersions)
	require.NoError(t, err)

	// Set Atmos config
	originalToolVersionsFile := GetToolVersionsFilePath()
	SetAtmosConfig(&schema.AtmosConfiguration{Toolchain: schema.Toolchain{VersionsFile: toolVersionsPath}})
	defer func() {
		SetAtmosConfig(&schema.AtmosConfiguration{Toolchain: schema.Toolchain{VersionsFile: originalToolVersionsFile}})
	}()

	// Test reinstalling all tools from .tool-versions
	err = RunInstall("", false, true, true, false)
	assert.NoError(t, err)
}

// TestPrintSummary tests the printSummary function with various scenarios.
func TestPrintSummary(t *testing.T) {
	tests := []struct {
		name      string
		installed int
		failed    int
		skipped   int
		total     int
		showHint  bool
	}{
		{
			name:      "all installed no skipped",
			installed: 3,
			failed:    0,
			skipped:   0,
			total:     3,
			showHint:  true,
		},
		{
			name:      "some installed some skipped",
			installed: 2,
			failed:    0,
			skipped:   1,
			total:     3,
			showHint:  false,
		},
		{
			name:      "some failed no skipped",
			installed: 1,
			failed:    2,
			skipped:   0,
			total:     3,
			showHint:  false,
		},
		{
			name:      "some failed some skipped",
			installed: 1,
			failed:    1,
			skipped:   1,
			total:     3,
			showHint:  true,
		},
		{
			name:      "no tools to install",
			installed: 0,
			failed:    0,
			skipped:   0,
			total:     0,
			showHint:  false,
		},
		{
			name:      "all skipped",
			installed: 0,
			failed:    0,
			skipped:   3,
			total:     3,
			showHint:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// printSummary writes to ui package which handles output formatting.
			// This test verifies the function doesn't panic with various input combinations.
			assert.NotPanics(t, func() {
				printSummary(tt.installed, tt.failed, tt.skipped, tt.total, tt.showHint)
			})
		})
	}
}

// TestPrintFailureSummary tests the printFailureSummary function.
func TestPrintFailureSummary(t *testing.T) {
	tests := []struct {
		name      string
		installed int
		failed    int
		skipped   int
	}{
		{
			name:      "failed with no skipped",
			installed: 1,
			failed:    2,
			skipped:   0,
		},
		{
			name:      "failed with some skipped",
			installed: 1,
			failed:    1,
			skipped:   1,
		},
		{
			name:      "all failed no skipped",
			installed: 0,
			failed:    3,
			skipped:   0,
		},
		{
			name:      "all failed some skipped",
			installed: 0,
			failed:    2,
			skipped:   1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// printFailureSummary writes to ui package which handles output formatting.
			// This test verifies the function doesn't panic with various input combinations.
			assert.NotPanics(t, func() {
				printFailureSummary(tt.installed, tt.failed, tt.skipped)
			})
		})
	}
}

// TestPrintSuccessSummary tests the printSuccessSummary function.
func TestPrintSuccessSummary(t *testing.T) {
	tests := []struct {
		name      string
		installed int
		skipped   int
		showHint  bool
	}{
		{
			name:      "installed with hint",
			installed: 3,
			skipped:   0,
			showHint:  true,
		},
		{
			name:      "installed without hint",
			installed: 3,
			skipped:   0,
			showHint:  false,
		},
		{
			name:      "installed and skipped with hint",
			installed: 2,
			skipped:   1,
			showHint:  true,
		},
		{
			name:      "installed and skipped without hint",
			installed: 2,
			skipped:   1,
			showHint:  false,
		},
		{
			name:      "all skipped with hint",
			installed: 0,
			skipped:   3,
			showHint:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// printSuccessSummary writes to ui package which handles output formatting.
			// This test verifies the function doesn't panic with various input combinations.
			assert.NotPanics(t, func() {
				printSuccessSummary(tt.installed, tt.skipped, tt.showHint)
			})
		})
	}
}

// TestBuildToolList tests the buildToolList function.
func TestBuildToolList(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("HOME", tempDir)

	// Create mock resolver - invalid tools are not in the mapping and return error.
	mockResolver := &mockToolResolver{
		mapping: map[string][2]string{
			"terraform":           {"hashicorp", "terraform"},
			"hashicorp/terraform": {"hashicorp", "terraform"},
			"opentofu":            {"opentofu", "opentofu"},
			"opentofu/opentofu":   {"opentofu", "opentofu"},
			// "invalid-tool-no-owner" is intentionally NOT in the mapping.
		},
	}

	binDir := filepath.Join(tempDir, ".atmos", "tools", "bin")
	installer := NewInstallerWithResolver(mockResolver, binDir)

	tests := []struct {
		name         string
		toolVersions *ToolVersions
		wantCount    int
	}{
		{
			name: "single tool single version",
			toolVersions: &ToolVersions{
				Tools: map[string][]string{
					"terraform": {"1.11.4"},
				},
			},
			wantCount: 1,
		},
		{
			name: "single tool multiple versions",
			toolVersions: &ToolVersions{
				Tools: map[string][]string{
					"terraform": {"1.11.4", "1.11.3"},
				},
			},
			wantCount: 2,
		},
		{
			name: "multiple tools",
			toolVersions: &ToolVersions{
				Tools: map[string][]string{
					"terraform": {"1.11.4"},
					"opentofu":  {"1.10.0"},
				},
			},
			wantCount: 2,
		},
		{
			name: "empty tool versions",
			toolVersions: &ToolVersions{
				Tools: map[string][]string{},
			},
			wantCount: 0,
		},
		{
			name: "invalid tool is skipped",
			toolVersions: &ToolVersions{
				Tools: map[string][]string{
					"invalid-tool-no-owner": {"1.0.0"},
					"terraform":             {"1.11.4"},
				},
			},
			wantCount: 1, // Only terraform should be included.
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			toolList := buildToolList(installer, tt.toolVersions)
			assert.Len(t, toolList, tt.wantCount)
		})
	}
}

// TestShowProgress tests the showProgress function.
func TestShowProgress(t *testing.T) {
	// Create mock spinner and progress bar
	spinner := newMockSpinner()
	progressBar := newMockProgressBar()

	tests := []struct {
		name   string
		tool   toolInfo
		state  progressState
		result string
	}{
		{
			name:   "installed result",
			tool:   toolInfo{version: "1.11.4", owner: "hashicorp", repo: "terraform"},
			state:  progressState{index: 0, total: 3, result: resultInstalled, err: nil},
			result: resultInstalled,
		},
		{
			name:   "skipped result",
			tool:   toolInfo{version: "1.11.4", owner: "hashicorp", repo: "terraform"},
			state:  progressState{index: 1, total: 3, result: resultSkipped, err: nil},
			result: resultSkipped,
		},
		{
			name:   "failed result",
			tool:   toolInfo{version: "1.11.4", owner: "hashicorp", repo: "terraform"},
			state:  progressState{index: 2, total: 3, result: resultFailed, err: assert.AnError},
			result: resultFailed,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// showProgress writes to ui package which handles output formatting.
			// This test verifies the function doesn't panic with various input combinations.
			assert.NotPanics(t, func() {
				showProgress(spinner, progressBar, tt.tool, tt.state)
			})
		})
	}
}

// TestRunInstallBatch tests the RunInstallBatch function.
func TestRunInstallBatch(t *testing.T) {
	tests := []struct {
		name          string
		toolSpecs     []string
		reinstallFlag bool
		wantErr       bool
	}{
		{
			name:          "empty toolSpecs",
			toolSpecs:     []string{},
			reinstallFlag: false,
			wantErr:       false,
		},
		{
			name:          "nil toolSpecs",
			toolSpecs:     nil,
			reinstallFlag: false,
			wantErr:       false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := RunInstallBatch(tt.toolSpecs, tt.reinstallFlag)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
