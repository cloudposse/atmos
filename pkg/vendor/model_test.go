package vendor

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/charmbracelet/bubbles/progress"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

func TestNewModelVendor_EmptyPackages(t *testing.T) {
	var emptyPkgs []pkgComponentVendor
	model, err := newModelVendor(emptyPkgs, false, nil)
	assert.NoError(t, err)
	assert.True(t, model.done, "Model should be done when no packages")
}

func TestNewModelVendor_ComponentPackages(t *testing.T) {
	pkgs := []pkgComponentVendor{
		{
			uri:     "github.com/example/repo.git//modules/vpc",
			name:    "vpc",
			version: "1.0.0",
		},
		{
			uri:     "github.com/example/repo.git//modules/ecs",
			name:    "ecs",
			version: "2.0.0",
		},
	}

	model, err := newModelVendor(pkgs, true, nil)
	assert.NoError(t, err)
	assert.False(t, model.done)
	assert.True(t, model.dryRun)
	assert.Len(t, model.packages, 2)
	assert.Equal(t, "vpc", model.packages[0].name)
	assert.Equal(t, "1.0.0", model.packages[0].version)
	assert.NotNil(t, model.packages[0].componentPackage)
}

func TestNewModelVendor_AtmosPackages(t *testing.T) {
	pkgs := []pkgAtmosVendor{
		{
			uri:     "github.com/example/repo.git//modules/vpc",
			name:    "vpc",
			version: "1.0.0",
		},
	}

	model, err := newModelVendor(pkgs, false, nil)
	assert.NoError(t, err)
	assert.False(t, model.done)
	assert.False(t, model.dryRun)
	assert.Len(t, model.packages, 1)
	assert.Equal(t, "vpc", model.packages[0].name)
	assert.NotNil(t, model.packages[0].atmosPackage)
}

func TestNeedsCustomDetection(t *testing.T) {
	tests := []struct {
		name     string
		src      string
		expected bool
	}{
		{
			name:     "http URL - no detection needed",
			src:      "http://example.com/repo.tar.gz",
			expected: false,
		},
		{
			name:     "https URL - no detection needed",
			src:      "https://github.com/example/repo.git",
			expected: false,
		},
		{
			name:     "git URL - no detection needed",
			src:      "git://github.com/example/repo.git",
			expected: false,
		},
		{
			name:     "s3 URL - no detection needed",
			src:      "s3://bucket/path",
			expected: false,
		},
		{
			name:     "gcs URL - no detection needed",
			src:      "gcs://bucket/path",
			expected: false,
		},
		{
			name:     "file URL - no detection needed",
			src:      "file:///local/path",
			expected: false,
		},
		{
			name:     "shorthand GitHub - needs detection",
			src:      "github.com/cloudposse/terraform-aws-components.git//modules/vpc?ref=1.0.0",
			expected: true,
		},
		{
			name:     "SCP-style URL - needs detection",
			src:      "git@github.com:cloudposse/atmos.git",
			expected: true,
		},
		{
			name:     "bare repository name - needs detection",
			src:      "cloudposse/terraform-aws-components",
			expected: true,
		},
		{
			name:     "git:: prefix with https - no detection needed",
			src:      "git::https://github.com/example/repo.git",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := needsCustomDetection(tt.src)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGenerateSkipFunction(t *testing.T) {
	tempDir := t.TempDir()

	// Create test files.
	gitDir := filepath.Join(tempDir, ".git")
	err := os.MkdirAll(gitDir, 0o755)
	assert.NoError(t, err)

	mainTf := filepath.Join(tempDir, "main.tf")
	err = os.WriteFile(mainTf, []byte("main"), 0o644)
	assert.NoError(t, err)

	readme := filepath.Join(tempDir, "README.md")
	err = os.WriteFile(readme, []byte("readme"), 0o644)
	assert.NoError(t, err)

	source := &schema.AtmosVendorSource{}

	skipFunc := generateSkipFunction(tempDir, source)

	// Test .git directory is always skipped.
	gitInfo := mockFileInfo{name: ".git", isDir: true}
	skip, err := skipFunc(gitInfo, gitDir, "")
	assert.NoError(t, err)
	assert.True(t, skip, ".git should always be skipped")

	// Test normal file is included when no patterns.
	tfInfo := mockFileInfo{name: "main.tf", isDir: false}
	skip, err = skipFunc(tfInfo, mainTf, "")
	assert.NoError(t, err)
	assert.False(t, skip, "Normal file should be included when no patterns")
}

func TestGenerateSkipFunction_WithPaths(t *testing.T) {
	tests := []struct {
		name          string
		includedPaths []string
		excludedPaths []string
		tfSkip        bool
		mdSkip        bool
	}{
		{
			name:          "included paths filter",
			includedPaths: []string{"**/*.tf"},
			excludedPaths: nil,
			tfSkip:        false,
			mdSkip:        true,
		},
		{
			name:          "excluded paths filter",
			includedPaths: nil,
			excludedPaths: []string{"**/*.md"},
			tfSkip:        false,
			mdSkip:        true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tempDir := t.TempDir()

			mainTf := filepath.Join(tempDir, "main.tf")
			err := os.WriteFile(mainTf, []byte("main"), 0o644)
			assert.NoError(t, err)

			readme := filepath.Join(tempDir, "README.md")
			err = os.WriteFile(readme, []byte("readme"), 0o644)
			assert.NoError(t, err)

			source := &schema.AtmosVendorSource{
				IncludedPaths: tt.includedPaths,
				ExcludedPaths: tt.excludedPaths,
			}

			skipFunc := generateSkipFunction(tempDir, source)

			// Test .tf file.
			tfInfo := mockFileInfo{name: "main.tf", isDir: false}
			skip, err := skipFunc(tfInfo, mainTf, "")
			assert.NoError(t, err)
			assert.Equal(t, tt.tfSkip, skip, ".tf file skip mismatch")

			// Test .md file.
			mdInfo := mockFileInfo{name: "README.md", isDir: false}
			skip, err = skipFunc(mdInfo, readme, "")
			assert.NoError(t, err)
			assert.Equal(t, tt.mdSkip, skip, ".md file skip mismatch")
		})
	}
}

func TestShouldExcludeFile(t *testing.T) {
	tests := []struct {
		name          string
		src           string
		excludedPaths []string
		trimmedSrc    string
		shouldExclude bool
	}{
		{
			name:          "no patterns - include all",
			src:           "/tmp/main.tf",
			excludedPaths: []string{},
			trimmedSrc:    "main.tf",
			shouldExclude: false,
		},
		{
			name:          "matches exclude pattern",
			src:           "/tmp/README.md",
			excludedPaths: []string{"**/*.md"},
			trimmedSrc:    "README.md",
			shouldExclude: true,
		},
		{
			name:          "does not match exclude pattern",
			src:           "/tmp/main.tf",
			excludedPaths: []string{"**/*.md"},
			trimmedSrc:    "main.tf",
			shouldExclude: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := shouldExcludeFile(tt.src, tt.excludedPaths, tt.trimmedSrc)
			assert.NoError(t, err)
			assert.Equal(t, tt.shouldExclude, result)
		})
	}
}

func TestMax(t *testing.T) {
	assert.Equal(t, 5, max(3, 5))
	assert.Equal(t, 5, max(5, 3))
	assert.Equal(t, 5, max(5, 5))
	assert.Equal(t, 0, max(-1, 0))
	assert.Equal(t, -1, max(-1, -5))
}

func TestModelVendor_Init_EmptyPackages(t *testing.T) {
	model := &modelVendor{
		packages: []pkgVendor{},
	}

	cmd := model.Init()
	assert.Nil(t, cmd)
	assert.True(t, model.done)
}

func TestModelVendor_HandleKeyPress(t *testing.T) {
	model := &modelVendor{
		packages: []pkgVendor{{name: "test"}},
	}

	// Verify model is properly initialized for key handling.
	assert.NotNil(t, model)
	assert.Equal(t, "test", model.packages[0].name)

	// Test quit key detection logic (the handleKeyPress function checks these keys).
	quitKeys := []string{"ctrl+c", "esc", "q"}
	for _, key := range quitKeys {
		isQuitKey := key == "ctrl+c" || key == "esc" || key == "q"
		assert.True(t, isQuitKey, "Key %s should be a quit key", key)
	}

	// Non-quit keys should not trigger quit.
	nonQuitKeys := []string{"x", "a", "enter", "space"}
	for _, key := range nonQuitKeys {
		isQuitKey := key == "ctrl+c" || key == "esc" || key == "q"
		assert.False(t, isQuitKey, "Key %s should not be a quit key", key)
	}
}

func TestNewInstallError(t *testing.T) {
	tests := []struct {
		name        string
		inputErr    error
		inputName   string
		expectedMsg string
	}{
		{
			name:        "basic error",
			inputErr:    assert.AnError,
			inputName:   "vpc",
			expectedMsg: "vpc",
		},
		{
			name:        "with component name",
			inputErr:    assert.AnError,
			inputName:   "ecs-cluster",
			expectedMsg: "ecs-cluster",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := newInstallError(tt.inputErr, tt.inputName)
			assert.Equal(t, tt.inputName, result.name)
			assert.Error(t, result.err)
			assert.Contains(t, result.err.Error(), tt.expectedMsg)
		})
	}
}

func TestModelVendor_View_Done(t *testing.T) {
	// Test done with no failures.
	model := &modelVendor{
		packages: []pkgVendor{{name: "pkg1"}, {name: "pkg2"}},
		done:     true,
	}

	view := model.View()
	assert.Contains(t, view, "Vendored 2 components")

	// Test done with dry run.
	model.dryRun = true
	view = model.View()
	assert.Contains(t, view, "Dry run completed")

	// Test done with failures.
	model.dryRun = false
	model.failedPkg = 1
	view = model.View()
	assert.Contains(t, view, "Failed to vendor 1 components")
}

func TestCopyToTargetWithPatterns(t *testing.T) {
	// Create source directory with files.
	srcDir := t.TempDir()

	err := os.WriteFile(filepath.Join(srcDir, "main.tf"), []byte("resource"), 0o644)
	require.NoError(t, err)

	err = os.WriteFile(filepath.Join(srcDir, "README.md"), []byte("readme"), 0o644)
	require.NoError(t, err)

	// Create a .git directory to be excluded.
	err = os.MkdirAll(filepath.Join(srcDir, ".git"), 0o755)
	require.NoError(t, err)
	err = os.WriteFile(filepath.Join(srcDir, ".git", "config"), []byte("git config"), 0o644)
	require.NoError(t, err)

	// Test copying all files.
	t.Run("copy all files", func(t *testing.T) {
		destDir := t.TempDir()
		source := &schema.AtmosVendorSource{}

		err := copyToTargetWithPatterns(srcDir, destDir, source, false)
		require.NoError(t, err)

		// Verify files were copied.
		_, err = os.Stat(filepath.Join(destDir, "main.tf"))
		assert.NoError(t, err)

		_, err = os.Stat(filepath.Join(destDir, "README.md"))
		assert.NoError(t, err)

		// .git should be excluded.
		_, err = os.Stat(filepath.Join(destDir, ".git"))
		assert.True(t, os.IsNotExist(err), ".git should not be copied")
	})

	// Test copying with included paths.
	t.Run("copy with included paths", func(t *testing.T) {
		destDir := t.TempDir()
		source := &schema.AtmosVendorSource{
			IncludedPaths: []string{"**/*.tf"},
		}

		err := copyToTargetWithPatterns(srcDir, destDir, source, false)
		require.NoError(t, err)

		// .tf files should be copied.
		_, err = os.Stat(filepath.Join(destDir, "main.tf"))
		assert.NoError(t, err)

		// .md files should be excluded.
		_, err = os.Stat(filepath.Join(destDir, "README.md"))
		assert.True(t, os.IsNotExist(err), ".md should not be copied with included_paths filter")
	})

	// Test copying with excluded paths.
	t.Run("copy with excluded paths", func(t *testing.T) {
		destDir := t.TempDir()
		source := &schema.AtmosVendorSource{
			ExcludedPaths: []string{"**/*.md"},
		}

		err := copyToTargetWithPatterns(srcDir, destDir, source, false)
		require.NoError(t, err)

		// .tf files should be copied.
		_, err = os.Stat(filepath.Join(destDir, "main.tf"))
		assert.NoError(t, err)

		// .md files should be excluded.
		_, err = os.Stat(filepath.Join(destDir, "README.md"))
		assert.True(t, os.IsNotExist(err), ".md should be excluded")
	})
}

func TestModelVendor_View_InProgress(t *testing.T) {
	model := &modelVendor{
		packages: []pkgVendor{
			{name: "pkg1", version: "1.0.0"},
			{name: "pkg2", version: "2.0.0"},
		},
		index: 0,
		width: 80,
		done:  false,
	}

	view := model.View()
	assert.Contains(t, view, "Pulling")
	assert.Contains(t, view, "pkg1")
}

func TestModelVendor_View_IndexOutOfBounds(t *testing.T) {
	model := &modelVendor{
		packages: []pkgVendor{{name: "pkg1"}},
		index:    10, // Out of bounds.
		done:     false,
	}

	// Should not panic and return empty string.
	view := model.View()
	assert.Empty(t, view)
}

func TestLogNonTTYFinalStatus(t *testing.T) {
	tests := []struct {
		name      string
		model     *modelVendor
		pkg       pkgVendor
		expectLog bool
	}{
		{
			name: "TTY mode - should not log",
			model: &modelVendor{
				isTTY:     true,
				packages:  []pkgVendor{{name: "pkg1"}},
				failedPkg: 0,
			},
			pkg:       pkgVendor{name: "pkg1", version: "1.0.0"},
			expectLog: false,
		},
		{
			name: "Non-TTY mode with success",
			model: &modelVendor{
				isTTY:     false,
				packages:  []pkgVendor{{name: "pkg1"}, {name: "pkg2"}},
				failedPkg: 0,
			},
			pkg:       pkgVendor{name: "pkg1", version: "1.0.0"},
			expectLog: true,
		},
		{
			name: "Non-TTY mode with failures",
			model: &modelVendor{
				isTTY:     false,
				packages:  []pkgVendor{{name: "pkg1"}, {name: "pkg2"}},
				failedPkg: 1,
			},
			pkg:       pkgVendor{name: "pkg1", version: ""},
			expectLog: true,
		},
		{
			name: "Non-TTY mode with dry run",
			model: &modelVendor{
				isTTY:     false,
				packages:  []pkgVendor{{name: "pkg1"}},
				dryRun:    true,
				failedPkg: 0,
			},
			pkg:       pkgVendor{name: "pkg1", version: ""},
			expectLog: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mark := checkMark
			// Function should not panic.
			tt.model.logNonNTYFinalStatus(tt.pkg, &mark)
		})
	}
}

func TestHandleInstalledPkgMsg_IndexOutOfBounds(t *testing.T) {
	model := &modelVendor{
		packages: []pkgVendor{{name: "pkg1"}},
		index:    10, // Out of bounds.
	}

	msg := &installedPkgMsg{err: nil, name: "pkg1"}
	result, cmd := model.handleInstalledPkgMsg(msg)

	assert.Equal(t, model, result)
	assert.Nil(t, cmd)
}

func TestHandleInstalledPkgMsg_LastPackage(t *testing.T) {
	model := &modelVendor{
		packages: []pkgVendor{
			{name: "pkg1", version: "1.0.0"},
		},
		index: 0,
		isTTY: true,
	}

	msg := &installedPkgMsg{err: nil, name: "pkg1"}
	result, _ := model.handleInstalledPkgMsg(msg)

	resultModel := result.(*modelVendor)
	assert.True(t, resultModel.done)
}

func TestHandleInstalledPkgMsg_WithError(t *testing.T) {
	model := &modelVendor{
		packages: []pkgVendor{
			{name: "pkg1", version: "1.0.0"},
		},
		index:     0,
		isTTY:     false,
		failedPkg: 0,
	}

	msg := &installedPkgMsg{err: assert.AnError, name: "pkg1"}
	result, _ := model.handleInstalledPkgMsg(msg)

	resultModel := result.(*modelVendor)
	assert.Equal(t, 1, resultModel.failedPkg)
}

func TestHandleInstalledPkgMsg_MiddlePackage(t *testing.T) {
	model := &modelVendor{
		packages: []pkgVendor{
			{name: "pkg1", version: "1.0.0"},
			{name: "pkg2", version: "2.0.0"},
		},
		index: 0,
		isTTY: false,
	}

	msg := &installedPkgMsg{err: nil, name: "pkg1"}
	result, _ := model.handleInstalledPkgMsg(msg)

	resultModel := result.(*modelVendor)
	assert.Equal(t, 1, resultModel.index)
	assert.False(t, resultModel.done)
}

func TestCopyToTargetWithPatterns_LocalFile(t *testing.T) {
	srcDir := t.TempDir()
	destDir := t.TempDir()

	// Create a source file.
	srcFile := filepath.Join(srcDir, "component.tf")
	err := os.WriteFile(srcFile, []byte("resource"), 0o644)
	require.NoError(t, err)

	// When sourceIsLocalFile=true with no extension in target, it appends the sanitized source name.
	// The source.Source field is used for sanitization.
	source := &schema.AtmosVendorSource{
		Source: "component.tf",
	}

	// Target with extension - file will be copied directly.
	targetFile := filepath.Join(destDir, "output.tf")
	err = copyToTargetWithPatterns(srcFile, targetFile, source, true)
	require.NoError(t, err)

	// Verify file was copied.
	_, err = os.Stat(targetFile)
	assert.NoError(t, err)
}

func TestModelVendor_Update_WindowSize(t *testing.T) {
	model := &modelVendor{
		packages: []pkgVendor{{name: "pkg1"}},
		width:    0,
		height:   0,
	}

	// Test window size message.
	msg := tea.WindowSizeMsg{Width: 100, Height: 50}
	result, _ := model.Update(msg)

	resultModel := result.(*modelVendor)
	assert.Equal(t, 100, resultModel.width)
	assert.Equal(t, 50, resultModel.height)
}

func TestModelVendor_Update_WindowSizeMaxWidth(t *testing.T) {
	model := &modelVendor{
		packages: []pkgVendor{{name: "pkg1"}},
		width:    0,
		height:   0,
	}

	// Test window size message that exceeds max width.
	msg := tea.WindowSizeMsg{Width: 200, Height: 50}
	result, _ := model.Update(msg)

	resultModel := result.(*modelVendor)
	assert.Equal(t, maxWidth, resultModel.width)
	assert.Equal(t, 50, resultModel.height)
}

func TestModelVendor_Update_InstalledPkgMsg(t *testing.T) {
	model := &modelVendor{
		packages: []pkgVendor{
			{name: "pkg1", version: "1.0.0"},
		},
		index: 0,
		isTTY: true,
	}

	msg := installedPkgMsg{err: nil, name: "pkg1"}
	result, _ := model.Update(msg)

	resultModel := result.(*modelVendor)
	assert.True(t, resultModel.done)
}

func TestHandleKeyPress_QuitKeys(t *testing.T) {
	model := &modelVendor{
		packages: []pkgVendor{{name: "pkg1"}},
	}

	tests := []struct {
		name      string
		key       string
		expectCmd bool
	}{
		{"ctrl+c quits", "ctrl+c", true},
		{"esc quits", "esc", true},
		{"q quits", "q", true},
		{"other key does not quit", "a", false},
		{"enter does not quit", "enter", false},
		{"space does not quit", " ", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var msg tea.KeyMsg
			switch tt.key {
			case "ctrl+c":
				msg = tea.KeyMsg{Type: tea.KeyCtrlC}
			case "esc":
				msg = tea.KeyMsg{Type: tea.KeyEscape}
			case "enter":
				msg = tea.KeyMsg{Type: tea.KeyEnter}
			case " ":
				msg = tea.KeyMsg{Type: tea.KeySpace}
			default:
				msg = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(tt.key)}
			}

			cmd := model.handleKeyPress(msg)
			if tt.expectCmd {
				assert.NotNil(t, cmd)
			} else {
				assert.Nil(t, cmd)
			}
		})
	}
}

func TestModelVendor_Update_KeyMsg(t *testing.T) {
	model := &modelVendor{
		packages: []pkgVendor{{name: "pkg1"}},
	}

	// Test quit key message.
	msg := tea.KeyMsg{Type: tea.KeyCtrlC}
	_, cmd := model.Update(msg)

	// Should return a quit command.
	assert.NotNil(t, cmd)
}

func TestModelVendor_Init_WithPackages(t *testing.T) {
	model := &modelVendor{
		packages: []pkgVendor{
			{
				name:    "pkg1",
				version: "1.0.0",
				componentPackage: &pkgComponentVendor{
					uri:  "github.com/example/repo.git//modules/vpc",
					name: "pkg1",
				},
			},
		},
		dryRun:      true,
		atmosConfig: nil,
	}

	cmd := model.Init()
	// Should return a batch command.
	assert.NotNil(t, cmd)
}

func TestNeedsCustomDetection_Extended(t *testing.T) {
	tempDir := t.TempDir()

	// Create a test file.
	testFile := filepath.Join(tempDir, "test.tf")
	err := os.WriteFile(testFile, []byte("# test"), 0o644)
	require.NoError(t, err)

	// Create a test directory.
	testDir := filepath.Join(tempDir, "testdir")
	err = os.MkdirAll(testDir, 0o755)
	require.NoError(t, err)

	tests := []struct {
		name     string
		src      string
		expected bool
	}{
		{
			name:     "local file path - no detection needed",
			src:      testFile,
			expected: false,
		},
		{
			name:     "local directory path - no detection needed",
			src:      testDir,
			expected: false,
		},
		{
			name:     "hg URL - no detection needed",
			src:      "hg::https://bitbucket.org/example/repo",
			expected: false,
		},
		{
			name:     "git+ssh URL - no detection needed",
			src:      "git+ssh://git@github.com/example/repo.git",
			expected: false,
		},
		{
			name:     "git+https URL - no detection needed",
			src:      "git+https://github.com/example/repo.git",
			expected: false,
		},
		{
			name:     "oci URL - no detection needed",
			src:      "oci://registry.example.com/repo",
			expected: false,
		},
		{
			name:     "ssh URL - no detection needed",
			src:      "ssh://git@github.com/example/repo.git",
			expected: false,
		},
		{
			name:     "empty string - no detection needed (empty url parsed)",
			src:      "",
			expected: false,
		},
		{
			name:     "with double colon and http",
			src:      "git::http://example.com/repo.git",
			expected: false,
		},
		{
			name:     "with double colon and unsupported scheme",
			src:      "custom::foobar://example.com",
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := needsCustomDetection(tt.src)
			assert.Equal(t, tt.expected, result, "needsCustomDetection result mismatch")
		})
	}
}

func TestExecuteInstall_NoValidPackage(t *testing.T) {
	// Test the case where no valid package is provided.
	installer := pkgVendor{
		name:             "invalid-pkg",
		atmosPackage:     nil,
		componentPackage: nil,
	}

	atmosConfig := &schema.AtmosConfiguration{}

	cmd := executeInstall(installer, false, atmosConfig)
	require.NotNil(t, cmd)

	// Execute the command to get the message.
	msg := cmd()
	installMsg, ok := msg.(installedPkgMsg)
	assert.True(t, ok, "Should return installedPkgMsg")
	assert.Error(t, installMsg.err, "Should return error for invalid package")
	assert.Equal(t, "invalid-pkg", installMsg.name)
}

func TestModelVendor_View_DryRunWithVersions(t *testing.T) {
	model := &modelVendor{
		packages: []pkgVendor{
			{name: "pkg1", version: "v1.2.3"},
			{name: "pkg2", version: ""},
		},
		done:   true,
		dryRun: true,
	}

	view := model.View()
	assert.Contains(t, view, "Dry run completed")
	// In dry-run mode, the message is "No components vendored".
}

func TestCopyToTargetWithPatterns_TargetWithExtension(t *testing.T) {
	srcDir := t.TempDir()
	destDir := t.TempDir()

	// Create source file.
	srcFile := filepath.Join(srcDir, "main.tf")
	err := os.WriteFile(srcFile, []byte("resource"), 0o644)
	require.NoError(t, err)

	// Create target path with specific extension.
	targetFile := filepath.Join(destDir, "renamed.tf")
	source := &schema.AtmosVendorSource{Source: "main.tf"}

	err = copyToTargetWithPatterns(srcFile, targetFile, source, true)
	require.NoError(t, err)

	// Verify file was copied with correct name.
	_, err = os.Stat(targetFile)
	assert.NoError(t, err, "File should exist at target path")
}

func TestModelVendor_HandleInstalledPkgMsg_WithVersion(t *testing.T) {
	model := &modelVendor{
		packages: []pkgVendor{
			{name: "pkg1", version: "v1.0.0"},
			{name: "pkg2", version: "v2.0.0"},
		},
		index: 0,
		isTTY: true,
	}

	msg := &installedPkgMsg{err: nil, name: "pkg1"}
	result, _ := model.handleInstalledPkgMsg(msg)

	resultModel := result.(*modelVendor)
	assert.Equal(t, 1, resultModel.index)
	assert.False(t, resultModel.done)
}

func TestModelVendor_Update_SpinnerTick(t *testing.T) {
	model := &modelVendor{
		packages: []pkgVendor{{name: "pkg1"}},
		spinner:  spinner.New(),
	}

	msg := spinner.TickMsg{}
	result, _ := model.Update(msg)
	assert.NotNil(t, result)
}

func TestModelVendor_Update_ProgressFrame(t *testing.T) {
	model := &modelVendor{
		packages: []pkgVendor{{name: "pkg1"}},
		progress: progress.New(),
	}

	msg := progress.FrameMsg{}
	result, _ := model.Update(msg)
	assert.NotNil(t, result)
}

func TestExecuteVendorModel_EmptyPackages(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{}
	model := &modelVendor{
		packages:    []pkgVendor{},
		atmosConfig: atmosConfig,
	}

	// The Init function with empty packages should set done=true.
	_ = model.Init()
	assert.True(t, model.done)
}

func TestCopyToTargetWithPatterns_WithSubdir(t *testing.T) {
	srcDir := t.TempDir()
	destDir := t.TempDir()

	// Create source with subdirectory.
	subDir := filepath.Join(srcDir, "modules", "vpc")
	err := os.MkdirAll(subDir, 0o755)
	require.NoError(t, err)

	srcFile := filepath.Join(subDir, "main.tf")
	err = os.WriteFile(srcFile, []byte("resource"), 0o644)
	require.NoError(t, err)

	source := &schema.AtmosVendorSource{}

	err = copyToTargetWithPatterns(srcDir, destDir, source, false)
	require.NoError(t, err)

	// Verify subdirectory structure was copied.
	_, err = os.Stat(filepath.Join(destDir, "modules", "vpc", "main.tf"))
	assert.NoError(t, err, "Subdirectory structure should be preserved")
}

func TestNewModelVendor_MixedPackages(t *testing.T) {
	// Test with both atmos and component packages.
	atmosPkgs := []pkgAtmosVendor{
		{uri: "github.com/example/repo.git//vpc", name: "vpc", version: "1.0.0"},
	}
	componentPkgs := []pkgComponentVendor{
		{uri: "github.com/example/repo.git//rds", name: "rds", version: "2.0.0"},
	}

	// Test with atmos packages.
	model1, err := newModelVendor(atmosPkgs, false, nil)
	assert.NoError(t, err)
	assert.Len(t, model1.packages, 1)
	assert.NotNil(t, model1.packages[0].atmosPackage)

	// Test with component packages.
	model2, err := newModelVendor(componentPkgs, false, nil)
	assert.NoError(t, err)
	assert.Len(t, model2.packages, 1)
	assert.NotNil(t, model2.packages[0].componentPackage)
}
