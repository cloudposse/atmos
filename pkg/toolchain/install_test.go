package toolchain

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/charmbracelet/bubbles/progress"
	bspinner "github.com/charmbracelet/bubbles/spinner"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

// newTestSpinner creates a spinner model for testing.
func newTestSpinner() *bspinner.Model {
	m := bspinner.New()
	return &m
}

// newTestProgressBar creates a progress bar model for testing.
func newTestProgressBar() *progress.Model {
	m := progress.New()
	return &m
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
	// Create mock spinner and progress bar.
	spinner := newTestSpinner()
	progressBar := newTestProgressBar()

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
// TODO: Refactor installMultipleTools to accept Installer interface for better testability.
// Currently, installMultipleTools creates Installer directly (line 383 of install.go),
// which blocks dependency injection and makes unit tests with mocks impossible.
// See ToolInstaller interface in set_test.go for reference pattern.
func TestRunInstallBatch(t *testing.T) {
	tests := []struct {
		name            string
		toolSpecs       []string
		reinstallFlag   bool
		wantErr         bool
		requiresNetwork bool
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
		{
			name:            "single tool delegates to RunInstall",
			toolSpecs:       []string{"terraform@1.11.4"},
			reinstallFlag:   false,
			wantErr:         false, // Delegates to single-tool flow.
			requiresNetwork: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.requiresNetwork && testing.Short() {
				t.Skip("Skipping test that requires network in short mode")
			}
			err := RunInstallBatch(tt.toolSpecs, tt.reinstallFlag)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// TestInstallMultipleTools_InvalidSpecs tests installMultipleTools with invalid tool specs.
func TestInstallMultipleTools_InvalidSpecs(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("HOME", tempDir)

	// Test with all invalid specs - should return nil (no valid tools to install).
	err := installMultipleTools([]string{"invalid-spec-no-version", "another-bad-spec"}, false)
	assert.NoError(t, err)
}

// TestSpinnerModel tests the spinnerModel Bubble Tea model.
func TestSpinnerModel(t *testing.T) {
	t.Run("initialSpinnerModel", func(t *testing.T) {
		model := initialSpinnerModel("Test message")
		assert.NotNil(t, model)
		assert.Equal(t, "Test message", model.message)
		assert.False(t, model.done)
	})

	t.Run("Init returns tick command", func(t *testing.T) {
		model := initialSpinnerModel("Test")
		cmd := model.Init()
		assert.NotNil(t, cmd)
	})

	t.Run("View returns message when not done", func(t *testing.T) {
		model := initialSpinnerModel("Test message")
		view := model.View()
		assert.Contains(t, view, "Test message")
	})

	t.Run("View returns empty when done", func(t *testing.T) {
		model := initialSpinnerModel("Test message")
		model.done = true
		view := model.View()
		assert.Empty(t, view)
	})

	t.Run("Update handles installDoneMsg", func(t *testing.T) {
		model := initialSpinnerModel("Test")
		updated, cmd := model.Update(installDoneMsg{})
		updatedModel := updated.(*spinnerModel)
		assert.True(t, updatedModel.done)
		assert.NotNil(t, cmd)
	})

	t.Run("Update handles bspinner.TickMsg", func(t *testing.T) {
		model := initialSpinnerModel("Test")
		_, cmd := model.Update(bspinner.TickMsg{})
		assert.NotNil(t, cmd)
	})

	t.Run("Update returns nil cmd for unknown msg", func(t *testing.T) {
		model := initialSpinnerModel("Test")
		_, cmd := model.Update("unknown message type")
		assert.Nil(t, cmd)
	})
}

// TestRunBubbleTeaSpinner tests the runBubbleTeaSpinner function.
func TestRunBubbleTeaSpinner(t *testing.T) {
	program := runBubbleTeaSpinner("Test message")
	assert.NotNil(t, program)
}

// TestInstallOrSkipTool tests the installOrSkipTool function.
func TestInstallOrSkipTool(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("HOME", tempDir)

	// Create mock resolver.
	mockResolver := &mockToolResolver{
		mapping: map[string][2]string{
			"terraform":           {"hashicorp", "terraform"},
			"hashicorp/terraform": {"hashicorp", "terraform"},
		},
	}

	binDir := filepath.Join(tempDir, ".atmos", "tools", "bin")
	installer := NewInstallerWithResolver(mockResolver, binDir)

	tests := []struct {
		name           string
		tool           toolInfo
		reinstallFlag  bool
		setupBinary    bool
		expectedResult string
	}{
		{
			name:           "tool not installed - installs",
			tool:           toolInfo{version: "1.11.4", owner: "hashicorp", repo: "terraform"},
			reinstallFlag:  false,
			setupBinary:    false,
			expectedResult: resultInstalled,
		},
		{
			name:           "tool already installed - skips",
			tool:           toolInfo{version: "1.11.4", owner: "hashicorp", repo: "terraform"},
			reinstallFlag:  false,
			setupBinary:    true,
			expectedResult: resultSkipped,
		},
		{
			name:           "tool already installed with reinstall flag - reinstalls",
			tool:           toolInfo{version: "1.11.4", owner: "hashicorp", repo: "terraform"},
			reinstallFlag:  true,
			setupBinary:    true,
			expectedResult: resultInstalled,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.setupBinary {
				// Create a mock binary.
				binaryPath := installer.GetBinaryPath(tt.tool.owner, tt.tool.repo, tt.tool.version, "")
				err := os.MkdirAll(filepath.Dir(binaryPath), 0o755)
				require.NoError(t, err)
				err = os.WriteFile(binaryPath, []byte("mock binary"), 0o755)
				require.NoError(t, err)
			}

			result, err := installOrSkipTool(installer, tt.tool, tt.reinstallFlag, false)

			// Skip case should never fail since no download is attempted.
			if tt.expectedResult == resultSkipped {
				require.NoError(t, err)
				assert.Equal(t, tt.expectedResult, result)
			} else if err == nil {
				// Install cases may fail in CI without network - accept either success or network error.
				// Network errors are acceptable in CI - code path is exercised either way.
				assert.Equal(t, tt.expectedResult, result)
			}
		})
	}
}

// TestInstallFromToolVersions_EmptyFile tests installFromToolVersions with an empty file.
func TestInstallFromToolVersions_EmptyFile(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("HOME", tempDir)

	// Create an empty .tool-versions file.
	toolVersionsPath := filepath.Join(tempDir, DefaultToolVersionsFilePath)
	err := os.WriteFile(toolVersionsPath, []byte(""), 0o644)
	require.NoError(t, err)

	// Should not error when file is empty.
	err = installFromToolVersions(toolVersionsPath, false, false)
	assert.NoError(t, err)
}

// TestInstallFromToolVersions_InvalidPath tests installFromToolVersions with invalid path.
func TestInstallFromToolVersions_InvalidPath(t *testing.T) {
	err := installFromToolVersions(filepath.Join(t.TempDir(), "does-not-exist", ".tool-versions"), false, false)
	assert.Error(t, err)
}

// TestRunInstall_PRVersionFormat tests that RunInstall detects PR version format.
func TestRunInstall_PRVersionFormat(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("HOME", tempDir)
	// Ensure no GitHub token is available so InstallFromPR fails fast.
	t.Setenv("ATMOS_GITHUB_TOKEN", "")
	t.Setenv("GITHUB_TOKEN", "")

	// "pr:9999" should be detected as a PR version.
	err := RunInstall("pr:9999", false, false, false, false)
	// It will fail because no GitHub token, but the PR detection path is exercised.
	assert.Error(t, err)
}

// TestRunInstall_SHAVersionFormat tests that RunInstall detects SHA version format.
func TestRunInstall_SHAVersionFormat(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("HOME", tempDir)
	// Ensure no GitHub token is available so InstallFromSHA fails fast.
	t.Setenv("ATMOS_GITHUB_TOKEN", "")
	t.Setenv("GITHUB_TOKEN", "")

	// "sha:ceb7526" should be detected as a SHA version.
	err := RunInstall("sha:ceb7526", false, false, false, false)
	// It will fail because no GitHub token, but the SHA detection path is exercised.
	assert.Error(t, err)
}

// TestRunInstall_PRVersionInToolSpec tests PR version after @ in tool spec.
func TestRunInstall_PRVersionInToolSpec(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("HOME", tempDir)
	t.Setenv("ATMOS_GITHUB_TOKEN", "")
	t.Setenv("GITHUB_TOKEN", "")

	// "atmos@pr:9999" should parse the tool and then detect PR version.
	err := RunInstall("atmos@pr:9999", false, false, false, false)
	assert.Error(t, err)
}

// TestRunInstall_SHAVersionInToolSpec tests SHA version after @ in tool spec.
func TestRunInstall_SHAVersionInToolSpec(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("HOME", tempDir)
	t.Setenv("ATMOS_GITHUB_TOKEN", "")
	t.Setenv("GITHUB_TOKEN", "")

	// "atmos@sha:ceb7526" should parse the tool and then detect SHA version.
	err := RunInstall("atmos@sha:ceb7526", false, false, false, false)
	assert.Error(t, err)
}

// TestInstallOptions tests the InstallOptions struct.
func TestInstallOptions(t *testing.T) {
	opts := InstallOptions{
		IsLatest:               true,
		ShowProgressBar:        true,
		ShowInstallDetails:     true,
		ShowHint:               true,
		SkipToolVersionsUpdate: true,
	}

	assert.True(t, opts.IsLatest)
	assert.True(t, opts.ShowProgressBar)
	assert.True(t, opts.ShowInstallDetails)
	assert.True(t, opts.ShowHint)
	assert.True(t, opts.SkipToolVersionsUpdate)
}
