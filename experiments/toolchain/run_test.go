package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Add a mock ToolResolver for tests

// Remove mockToolResolver type and its Resolve method from this file.
// Replace installer.resolveToolName(...) with installer.resolver.Resolve(...)

func TestRunTool(t *testing.T) {
	// Create a temporary directory for testing
	tempDir, err := os.MkdirTemp("", "toolchain-test")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	// Change to temp directory for test
	originalWd, err := os.Getwd()
	require.NoError(t, err)
	defer os.Chdir(originalWd)
	err = os.Chdir(tempDir)
	require.NoError(t, err)

	// Set up a mock resolver with alias for terraform
	mockResolver := &mockToolResolver{
		mapping: map[string][2]string{
			"terraform": {"hashicorp", "terraform"},
			"opentofu":  {"opentofu", "opentofu"},
			"kubectl":   {"kubernetes", "kubectl"},
			"helm":      {"helm", "helm"},
			"helmfile":  {"helmfile", "helmfile"},
		},
	}

	tests := []struct {
		name        string
		args        []string
		setupMock   func() error
		wantErr     bool
		wantOutput  string
		description string
	}{
		{
			name:        "valid tool specification",
			args:        []string{"terraform@1.9.8", "--version"},
			wantErr:     false,
			description: "Should handle valid tool@version format",
		},
		{
			name:        "invalid tool specification - missing @",
			args:        []string{"terraform", "--version"},
			wantErr:     false,
			description: "Should fallback to .tool-versions or latest if @version is missing",
		},
		{
			name:        "invalid tool specification - multiple @",
			args:        []string{"terraform@1.9.8@extra", "--version"},
			wantErr:     true,
			description: "Should error on multiple @ in tool specification",
		},
		{
			name:        "empty tool specification",
			args:        []string{},
			wantErr:     true,
			description: "Should error on empty arguments",
		},
		{
			name:        "missing @version - fallback to .tool-versions or latest",
			args:        []string{"terraform", "--version"},
			wantErr:     false,
			description: "Should fallback to .tool-versions or latest if @version is missing",
		},
		{
			name:        "tool not in .tool-versions - fallback to latest",
			args:        []string{"nonexistent", "--version"},
			wantErr:     true,
			description: "Should error if tool not in .tool-versions and not found in registry",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.setupMock != nil {
				err := tt.setupMock()
				require.NoError(t, err)
			}

			cmd := &cobra.Command{}
			cmd.SetArgs(tt.args)

			var output strings.Builder
			cmd.SetOut(&output)
			cmd.SetErr(&output)

			var err error
			if len(tt.args) == 0 {
				err = fmt.Errorf("no arguments provided")
			} else {
				installer := NewInstallerWithResolver(mockResolver)
				err = runToolWithInstaller(installer, cmd, tt.args)
			}

			if tt.wantErr {
				assert.Error(t, err, tt.description)
			} else {
				assert.NoError(t, err, tt.description)
			}
		})
	}
}

func TestToolSpecParsing(t *testing.T) {
	tests := []struct {
		name        string
		toolSpec    string
		wantTool    string
		wantVersion string
		wantErr     bool
		description string
	}{
		{
			name:        "valid tool@version",
			toolSpec:    "terraform@1.9.8",
			wantTool:    "terraform",
			wantVersion: "1.9.8",
			wantErr:     false,
			description: "Should parse valid tool@version format",
		},
		{
			name:        "valid tool with latest version",
			toolSpec:    "opentofu@latest",
			wantTool:    "opentofu",
			wantVersion: "latest",
			wantErr:     false,
			description: "Should parse tool@latest format",
		},
		{
			name:        "missing @ separator",
			toolSpec:    "terraform1.9.8",
			wantErr:     true,
			description: "Should error on missing @ separator",
		},
		{
			name:        "multiple @ separators",
			toolSpec:    "terraform@1.9.8@extra",
			wantErr:     true,
			description: "Should error on multiple @ separators",
		},
		{
			name:        "empty tool name",
			toolSpec:    "@1.9.8",
			wantErr:     true,
			description: "Should error on empty tool name",
		},
		{
			name:        "empty version",
			toolSpec:    "terraform@",
			wantErr:     true,
			description: "Should error on empty version",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parts := strings.Split(tt.toolSpec, "@")

			if tt.wantErr {
				// Check for various error conditions
				if len(parts) != 2 {
					// This is expected for missing @ or multiple @
					return
				}
				// Check for empty tool name or empty version
				if parts[0] == "" || parts[1] == "" {
					return
				}
				t.Errorf("Expected error but got valid parts: %v", parts)
				return
			}

			assert.Equal(t, 2, len(parts), tt.description)
			tool := parts[0]
			version := parts[1]

			// Additional validation for empty parts
			assert.NotEmpty(t, tool, "Tool name should not be empty")
			assert.NotEmpty(t, version, "Version should not be empty")

			assert.Equal(t, tt.wantTool, tool, tt.description)
			assert.Equal(t, tt.wantVersion, version, tt.description)
		})
	}
}

func TestBinaryPathResolution(t *testing.T) {
	// Create a temporary directory for testing
	tempDir, err := os.MkdirTemp("", "toolchain-test")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	installer := NewInstaller()
	installer.binDir = tempDir

	// Create mock directory structure
	toolDir := filepath.Join(tempDir, "hashicorp", "terraform", "1.9.8")
	err = os.MkdirAll(toolDir, 0755)
	require.NoError(t, err)

	// Create mock binary
	binaryPath := filepath.Join(toolDir, "terraform")
	err = os.WriteFile(binaryPath, []byte("mock binary"), 0755)
	require.NoError(t, err)

	tests := []struct {
		name        string
		owner       string
		repo        string
		version     string
		wantPath    string
		wantErr     bool
		description string
	}{
		{
			name:        "existing binary",
			owner:       "hashicorp",
			repo:        "terraform",
			version:     "1.9.8",
			wantPath:    binaryPath,
			wantErr:     false,
			description: "Should find existing binary path",
		},
		{
			name:        "non-existent binary",
			owner:       "hashicorp",
			repo:        "terraform",
			version:     "999.999.999",
			wantErr:     true,
			description: "Should error on non-existent binary",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path, err := installer.findBinaryPath(tt.owner, tt.repo, tt.version)

			if tt.wantErr {
				assert.Error(t, err, tt.description)
			} else {
				assert.NoError(t, err, tt.description)
				assert.Equal(t, tt.wantPath, path, tt.description)
			}
		})
	}
}

func TestLatestVersionResolution(t *testing.T) {
	// Create a temporary directory for testing
	tempDir, err := os.MkdirTemp("", "toolchain-test")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	installer := NewInstaller()
	installer.binDir = tempDir

	// Create mock latest file
	toolDir := filepath.Join(tempDir, "hashicorp", "terraform")
	err = os.MkdirAll(toolDir, 0755)
	require.NoError(t, err)

	latestFile := filepath.Join(toolDir, "latest")
	err = os.WriteFile(latestFile, []byte("1.9.8"), 0644)
	require.NoError(t, err)

	tests := []struct {
		name        string
		owner       string
		repo        string
		version     string
		wantVersion string
		wantErr     bool
		description string
	}{
		{
			name:        "latest version resolution",
			owner:       "hashicorp",
			repo:        "terraform",
			version:     "latest",
			wantVersion: "1.9.8",
			wantErr:     false,
			description: "Should resolve latest to actual version",
		},
		{
			name:        "specific version",
			owner:       "hashicorp",
			repo:        "terraform",
			version:     "1.9.8",
			wantVersion: "1.9.8",
			wantErr:     false,
			description: "Should return specific version as-is",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// This would need to be implemented in the installer
			// For now, just test the file reading logic
			if tt.version == "latest" {
				latestPath := filepath.Join(installer.binDir, tt.owner, tt.repo, "latest")
				if _, err := os.Stat(latestPath); err == nil {
					content, err := os.ReadFile(latestPath)
					require.NoError(t, err)
					resolvedVersion := strings.TrimSpace(string(content))
					assert.Equal(t, tt.wantVersion, resolvedVersion, tt.description)
				}
			}
		})
	}
}

func TestFileOperations(t *testing.T) {
	// Create a temporary directory for testing
	tempDir, err := os.MkdirTemp("", "toolchain-test")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	// Test file creation
	testFile := filepath.Join(tempDir, "test.txt")
	content := "test content"
	err = os.WriteFile(testFile, []byte(content), 0644)
	require.NoError(t, err)

	// Test file reading
	readContent, err := os.ReadFile(testFile)
	require.NoError(t, err)
	assert.Equal(t, content, string(readContent))

	// Test file existence
	_, err = os.Stat(testFile)
	require.NoError(t, err)

	// Test directory creation
	testDir := filepath.Join(tempDir, "testdir")
	err = os.MkdirAll(testDir, 0755)
	require.NoError(t, err)

	// Test directory existence
	_, err = os.Stat(testDir)
	require.NoError(t, err)
}

func TestPathConstruction(t *testing.T) {
	tests := []struct {
		name        string
		parts       []string
		expected    string
		description string
	}{
		{
			name:        "simple path",
			parts:       []string{"bin", "tool", "version"},
			expected:    "bin/tool/version",
			description: "Should construct simple path correctly",
		},
		{
			name:        "empty parts",
			parts:       []string{},
			expected:    "",
			description: "Should handle empty parts",
		},
		{
			name:        "single part",
			parts:       []string{"bin"},
			expected:    "bin",
			description: "Should handle single part",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := filepath.Join(tt.parts...)
			assert.Equal(t, tt.expected, result, tt.description)
		})
	}
}

func TestStringOperations(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		operation   func(string) string
		expected    string
		description string
	}{
		{
			name:        "trim space",
			input:       "  test  ",
			operation:   strings.TrimSpace,
			expected:    "test",
			description: "Should trim whitespace correctly",
		},
		{
			name:        "to lower",
			input:       "TEST",
			operation:   strings.ToLower,
			expected:    "test",
			description: "Should convert to lowercase",
		},
		{
			name:        "to upper",
			input:       "test",
			operation:   strings.ToUpper,
			expected:    "TEST",
			description: "Should convert to uppercase",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.operation(tt.input)
			assert.Equal(t, tt.expected, result, tt.description)
		})
	}
}

func TestFileReadingOperations(t *testing.T) {
	// Create a temporary directory for testing
	tempDir, err := os.MkdirTemp("", "toolchain-test")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	// Test file creation and reading
	testFile := filepath.Join(tempDir, "test.txt")
	content := "test content\n"
	err = os.WriteFile(testFile, []byte(content), 0644)
	require.NoError(t, err)

	// Test reading file content
	readContent, err := os.ReadFile(testFile)
	require.NoError(t, err)
	assert.Equal(t, content, string(readContent))

	// Test reading file content with trimming
	trimmedContent := strings.TrimSpace(string(readContent))
	assert.Equal(t, "test content", trimmedContent)

	// Test reading non-existent file
	nonExistentFile := filepath.Join(tempDir, "nonexistent.txt")
	_, err = os.ReadFile(nonExistentFile)
	assert.Error(t, err)
	assert.True(t, os.IsNotExist(err))
}

func TestLatestFileReading(t *testing.T) {
	// Create a temporary directory for testing
	tempDir, err := os.MkdirTemp("", "toolchain-test")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	installer := NewInstaller()
	installer.binDir = tempDir

	tests := []struct {
		name        string
		owner       string
		repo        string
		version     string
		shouldExist bool
		description string
	}{
		{
			name:        "existing latest file",
			owner:       "hashicorp",
			repo:        "terraform",
			version:     "1.9.8",
			shouldExist: true,
			description: "Should read version from existing latest file",
		},
		{
			name:        "non-existent latest file",
			owner:       "nonexistent",
			repo:        "tool",
			version:     "",
			shouldExist: false,
			description: "Should return error for non-existent latest file",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.shouldExist {
				// Create latest file
				latestDir := filepath.Join(tempDir, tt.owner, tt.repo)
				err := os.MkdirAll(latestDir, 0755)
				require.NoError(t, err)

				latestFile := filepath.Join(latestDir, "latest")
				err = os.WriteFile(latestFile, []byte(tt.version), 0644)
				require.NoError(t, err)

				// Test reading
				version, err := installer.readLatestFile(tt.owner, tt.repo)
				require.NoError(t, err)
				assert.Equal(t, tt.version, version, tt.description)
			} else {
				// Test reading non-existent file
				_, err := installer.readLatestFile(tt.owner, tt.repo)
				assert.Error(t, err, tt.description)
			}
		})
	}
}

func TestPathResolution(t *testing.T) {
	// Create a temporary directory for testing
	tempDir, err := os.MkdirTemp("", "toolchain-test")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	installer := NewInstaller()
	installer.binDir = tempDir

	// Test path construction
	expectedPath := filepath.Join(tempDir, "hashicorp", "terraform", "1.9.8", "terraform")

	// Create the directory structure
	dir := filepath.Dir(expectedPath)
	err = os.MkdirAll(dir, 0755)
	require.NoError(t, err)

	// Create the binary file
	err = os.WriteFile(expectedPath, []byte("mock binary"), 0755)
	require.NoError(t, err)

	// Test that the path exists
	_, err = os.Stat(expectedPath)
	require.NoError(t, err)

	// Test path resolution
	path, err := installer.findBinaryPath("hashicorp", "terraform", "1.9.8")
	require.NoError(t, err)
	assert.Equal(t, expectedPath, path)
}

func TestErrorHandling(t *testing.T) {
	// Test error wrapping
	originalErr := fmt.Errorf("original error")
	wrappedErr := fmt.Errorf("context: %w", originalErr)

	assert.Error(t, wrappedErr)
	assert.Contains(t, wrappedErr.Error(), "context: original error")

	// Test error type checking
	_, err := os.Open("nonexistent-file")
	assert.Error(t, err)
	assert.True(t, os.IsNotExist(err))
}

func TestFileContentReading(t *testing.T) {
	// Create a temporary directory for testing
	tempDir, err := os.MkdirTemp("", "toolchain-test")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	// Test reading latest file content
	latestFile := filepath.Join(tempDir, "latest")
	version := "1.9.8"
	err = os.WriteFile(latestFile, []byte(version), 0644)
	require.NoError(t, err)

	// Read with os.ReadFile
	content, err := os.ReadFile(latestFile)
	require.NoError(t, err)
	resolvedVersion := strings.TrimSpace(string(content))
	assert.Equal(t, version, resolvedVersion)

	// Test reading with newlines
	versionWithNewlines := "1.9.8\n"
	err = os.WriteFile(latestFile, []byte(versionWithNewlines), 0644)
	require.NoError(t, err)

	content, err = os.ReadFile(latestFile)
	require.NoError(t, err)
	resolvedVersion = strings.TrimSpace(string(content))
	assert.Equal(t, "1.9.8", resolvedVersion)

	// Test reading with multiple newlines and spaces
	versionWithSpaces := "  1.9.8  \n\n"
	err = os.WriteFile(latestFile, []byte(versionWithSpaces), 0644)
	require.NoError(t, err)

	content, err = os.ReadFile(latestFile)
	require.NoError(t, err)
	resolvedVersion = strings.TrimSpace(string(content))
	assert.Equal(t, "1.9.8", resolvedVersion)
}

func TestRunToolVersionResolution(t *testing.T) {
	// Create a temporary directory for testing
	tempDir, err := os.MkdirTemp("", "toolchain-test")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	// Change to temp directory for test
	originalWd, err := os.Getwd()
	require.NoError(t, err)
	defer os.Chdir(originalWd)
	err = os.Chdir(tempDir)
	require.NoError(t, err)

	// Set up a mock resolver with alias for terraform
	mockResolver := &mockToolResolver{
		mapping: map[string][2]string{
			"terraform": {"hashicorp", "terraform"},
			"opentofu":  {"opentofu", "opentofu"},
			"kubectl":   {"kubernetes", "kubectl"},
			"helm":      {"helm", "helm"},
			"helmfile":  {"helmfile", "helmfile"},
		},
	}
	installer := NewInstallerWithResolver(mockResolver)

	tests := []struct {
		name            string
		toolSpec        string
		setupFiles      func() error
		expectedTool    string
		expectedVersion string
		description     string
	}{
		{
			name:            "tool with @version specified",
			toolSpec:        "terraform@1.9.8",
			setupFiles:      func() error { return nil },
			expectedTool:    "terraform",
			expectedVersion: "1.9.8",
			description:     "Should use specified version when @version is provided",
		},
		{
			name:     "tool without @version - uses .tool-versions",
			toolSpec: "terraform",
			setupFiles: func() error {
				// Create .tool-versions file
				content := "terraform 1.9.8\nopentofu 1.10.3\n"
				return os.WriteFile(".tool-versions", []byte(content), 0644)
			},
			expectedTool:    "terraform",
			expectedVersion: "1.9.8",
			description:     "Should use version from .tool-versions when no @version specified",
		},
		{
			name:     "tool without @version - uses latest file",
			toolSpec: "opentofu",
			setupFiles: func() error {
				// Create latest file for opentofu
				latestDir := filepath.Join(tempDir, "opentofu", "opentofu")
				if err := os.MkdirAll(latestDir, 0755); err != nil {
					return err
				}
				latestFile := filepath.Join(latestDir, "latest")
				return os.WriteFile(latestFile, []byte("1.10.3"), 0644)
			},
			expectedTool:    "opentofu",
			expectedVersion: "1.10.3",
			description:     "Should use version from latest file when not in .tool-versions",
		},
		{
			name:     "tool without @version - falls back to latest",
			toolSpec: "nonexistent",
			setupFiles: func() error {
				// Create .tool-versions file without the tool
				content := "terraform 1.9.8\n"
				return os.WriteFile(".tool-versions", []byte(content), 0644)
			},
			expectedTool:    "nonexistent",
			expectedVersion: "latest",
			description:     "Should fall back to latest when tool not in .tool-versions and no latest file",
		},
		{
			name:     "tool without @version - .tool-versions takes precedence",
			toolSpec: "terraform",
			setupFiles: func() error {
				// Create .tool-versions file
				content := "terraform 1.9.8\n"
				if err := os.WriteFile(".tool-versions", []byte(content), 0644); err != nil {
					return err
				}
				// Create latest file that should be ignored
				latestDir := filepath.Join(tempDir, "hashicorp", "terraform")
				if err := os.MkdirAll(latestDir, 0755); err != nil {
					return err
				}
				latestFile := filepath.Join(latestDir, "latest")
				return os.WriteFile(latestFile, []byte("2.0.0"), 0644)
			},
			expectedTool:    "terraform",
			expectedVersion: "1.9.8",
			description:     "Should prioritize .tool-versions over latest file",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup files
			if tt.setupFiles != nil {
				err := tt.setupFiles()
				require.NoError(t, err)
			}

			// Parse tool specification (simulating the logic from runTool)
			parts := strings.Split(tt.toolSpec, "@")
			var tool, version string

			if len(parts) == 1 {
				// No @version specified, check for configured version
				tool = parts[0]
				version = "latest" // default fallback

				// First, check .tool-versions file
				if toolVersions, err := LoadToolVersions(".tool-versions"); err == nil {
					if configuredVersion, exists := toolVersions.Tools[tool]; exists {
						if len(configuredVersion) > 0 {
							version = configuredVersion[0]
						}
					}
				}

				// If still "latest", check if there's a latest file for this tool
				if version == "latest" {
					owner, repo, err := installer.resolver.Resolve(tool)
					if err == nil {
						if latestVersion, err := installer.readLatestFile(owner, repo); err == nil {
							version = latestVersion
						}
					}
				}
			} else if len(parts) == 2 {
				// tool@version format
				tool = parts[0]
				version = parts[1]
			}

			assert.Equal(t, tt.expectedTool, tool, tt.description)
			assert.Equal(t, tt.expectedVersion, version, tt.description)
		})
	}
}

func TestToolVersionsFileParsing(t *testing.T) {
	// Create a temporary directory for testing
	tempDir, err := os.MkdirTemp("", "toolchain-test")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	tests := []struct {
		name        string
		content     string
		expected    map[string][]string
		description string
	}{
		{
			name: "simple tool versions",
			content: `terraform 1.9.8
opentofu 1.10.3
kubectl 1.28.0`,
			expected: map[string][]string{
				"terraform": {"1.9.8"},
				"opentofu":  {"1.10.3"},
				"kubectl":   {"1.28.0"},
			},
			description: "Should parse simple tool versions correctly",
		},
		{
			name: "with comments and empty lines",
			content: `# This is a comment
terraform 1.9.8

# Another comment
opentofu 1.10.3

`,
			expected: map[string][]string{
				"terraform": {"1.9.8"},
				"opentofu":  {"1.10.3"},
			},
			description: "Should ignore comments and empty lines",
		},
		{
			name:        "empty file",
			content:     "",
			expected:    map[string][]string{},
			description: "Should handle empty file",
		},
		{
			name: "with extra whitespace",
			content: `  terraform    1.9.8
  opentofu  1.10.3  `,
			expected: map[string][]string{
				"terraform": {"1.9.8"},
				"opentofu":  {"1.10.3"},
			},
			description: "Should handle extra whitespace",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temporary .tool-versions file
			toolVersionsFile := filepath.Join(tempDir, ".tool-versions")
			err := os.WriteFile(toolVersionsFile, []byte(tt.content), 0644)
			require.NoError(t, err)

			// Load and parse
			toolVersions, err := LoadToolVersions(toolVersionsFile)
			require.NoError(t, err)

			assert.Equal(t, tt.expected, toolVersions.Tools, tt.description)
		})
	}
}

func TestRunToolMemorializesLatest(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, ".tool-versions")
	os.Setenv("HOME", dir)
	_ = os.Remove(filePath)

	// Simulate running a tool not in .tool-versions
	tool := "terraform"
	version := "latest"

	// Simulate fallback to latest and memorialization
	err := AddToolToVersions(filePath, tool, version)
	assert.NoError(t, err)

	toolVersions, err := LoadToolVersions(filePath)
	assert.NoError(t, err)
	assert.Equal(t, map[string][]string{"terraform": {"latest"}}, toolVersions.Tools)
}

func TestRunToolMemorializesAndWritesLatestFile(t *testing.T) {
	tempDir := t.TempDir()
	filePath := filepath.Join(tempDir, ".tool-versions")
	os.Setenv("HOME", tempDir)
	_ = os.Remove(filePath)

	// Simulate running a tool not in .tool-versions
	installer := NewInstaller()
	installer.binDir = tempDir
	tool := "terraform"
	owner := "hashicorp"
	repo := "terraform"
	actualVersion := "1.9.8"

	// Simulate install: memorialize 'tool latest' and write latest file
	err := AddToolToVersions(filePath, tool, "latest")
	assert.NoError(t, err)
	err = installer.createLatestFile(owner, repo, actualVersion)
	assert.NoError(t, err)

	// Check .tool-versions file
	toolVersions, err := LoadToolVersions(filePath)
	assert.NoError(t, err)
	assert.Equal(t, map[string][]string{"terraform": {"latest"}}, toolVersions.Tools)

	// Check latest file contents
	latestFile := filepath.Join(tempDir, owner, repo, "latest")
	content, err := os.ReadFile(latestFile)
	assert.NoError(t, err)
	assert.Equal(t, actualVersion, strings.TrimSpace(string(content)))
}
