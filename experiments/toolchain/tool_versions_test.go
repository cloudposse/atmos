package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadToolVersions(t *testing.T) {
	testCases := []struct {
		name          string
		content       string
		expectedTools map[string][]string
		expectedError bool
		errorContains string
	}{
		{
			name:    "valid tool versions",
			content: "terraform 1.5.0\nhelm 3.12.0\nkubectl 1.28.0\n",
			expectedTools: map[string][]string{
				"terraform": {"1.5.0"},
				"helm":      {"3.12.0"},
				"kubectl":   {"1.28.0"},
			},
			expectedError: false,
		},
		{
			name:    "with comments and empty lines",
			content: "# This is a comment\nterraform 1.5.0\n\n# Another comment\nhelm 3.12.0\n",
			expectedTools: map[string][]string{
				"terraform": {"1.5.0"},
				"helm":      {"3.12.0"},
			},
			expectedError: false,
		},
		{
			name:          "tool without version (invalid)",
			content:       "terraform 1.5.0\nhelm\nkubectl 1.28.0\n",
			expectedTools: nil,
			expectedError: true,
			errorContains: "missing version",
		},
		{
			name:          "empty file",
			content:       "",
			expectedTools: map[string][]string{},
			expectedError: false,
		},
		{
			name:          "file with only comments",
			content:       "# Comment 1\n# Comment 2\n",
			expectedTools: map[string][]string{},
			expectedError: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Create temporary file
			tmpFile := filepath.Join(t.TempDir(), ".tool-versions")
			err := os.WriteFile(tmpFile, []byte(tc.content), 0644)
			require.NoError(t, err)

			// Load tool versions
			toolVersions, err := LoadToolVersions(tmpFile)

			if tc.expectedError {
				assert.Error(t, err)
				if tc.errorContains != "" {
					assert.Contains(t, err.Error(), tc.errorContains)
				}
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tc.expectedTools, toolVersions.Tools)
			}
		})
	}
}

func TestSaveToolVersions(t *testing.T) {
	testCases := []struct {
		name          string
		toolVersions  *ToolVersions
		expectedLines []string
		shouldError   bool
		errorContains string
	}{
		{
			name: "multiple tools",
			toolVersions: &ToolVersions{
				Tools: map[string][]string{
					"terraform": {"1.5.0"},
					"helm":      {"3.12.0"},
					"kubectl":   {"1.28.0"},
				},
			},
			expectedLines: []string{
				"helm 3.12.0",
				"kubectl 1.28.0",
				"terraform 1.5.0",
			},
			shouldError: false,
		},
		{
			name: "tool missing version (invalid)",
			toolVersions: &ToolVersions{
				Tools: map[string][]string{
					"terraform": {"1.5.0"},
					"helm":      {""},
				},
			},
			expectedLines: nil,
			shouldError:   true,
			errorContains: "missing a version",
		},
		{
			name: "empty tools",
			toolVersions: &ToolVersions{
				Tools: map[string][]string{},
			},
			expectedLines: []string{},
			shouldError:   false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Create temporary file
			tmpFile := filepath.Join(t.TempDir(), ".tool-versions")

			// Save tool versions
			err := SaveToolVersions(tmpFile, tc.toolVersions)
			if tc.shouldError {
				assert.Error(t, err)
				if tc.errorContains != "" {
					assert.Contains(t, err.Error(), tc.errorContains)
				}
				return
			}
			require.NoError(t, err)

			// Read back the file
			content, err := os.ReadFile(tmpFile)
			require.NoError(t, err)

			// Parse content
			lines := strings.Split(strings.TrimSpace(string(content)), "\n")
			if len(lines) == 1 && lines[0] == "" {
				lines = []string{}
			}

			assert.Equal(t, tc.expectedLines, lines)
		})
	}
}

func TestAddToolToVersions(t *testing.T) {
	testCases := []struct {
		name           string
		initialContent string
		tool           string
		version        string
		expectedTools  map[string][]string
		shouldError    bool
		errorContains  string
	}{
		{
			name:           "add to empty file",
			initialContent: "",
			tool:           "terraform",
			version:        "1.5.0",
			expectedTools: map[string][]string{
				"terraform": {"1.5.0"},
			},
			shouldError: false,
		},
		{
			name:           "add to existing file",
			initialContent: "helm 3.12.0\n",
			tool:           "terraform",
			version:        "1.5.0",
			expectedTools: map[string][]string{
				"helm":      {"3.12.0"},
				"terraform": {"1.5.0"},
			},
			shouldError: false,
		},
		{
			name:           "update existing tool",
			initialContent: "terraform 1.4.0\nhelm 3.12.0\n",
			tool:           "terraform",
			version:        "1.5.0",
			expectedTools: map[string][]string{
				"terraform": {"1.4.0", "1.5.0"},
				"helm":      {"3.12.0"},
			},
			shouldError: false,
		},
		{
			name:           "add tool without version (invalid)",
			initialContent: "helm 3.12.0\n",
			tool:           "terraform",
			version:        "",
			expectedTools:  nil,
			shouldError:    true,
			errorContains:  "without a version",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Create temporary file
			tmpFile := filepath.Join(t.TempDir(), ".tool-versions")

			if tc.initialContent != "" {
				err := os.WriteFile(tmpFile, []byte(tc.initialContent), 0644)
				require.NoError(t, err)
			}

			// Add tool to versions
			err := AddToolToVersions(tmpFile, tc.tool, tc.version)
			if tc.shouldError {
				assert.Error(t, err)
				if tc.errorContains != "" {
					assert.Contains(t, err.Error(), tc.errorContains)
				}
				return
			}
			require.NoError(t, err)

			// Load and verify
			toolVersions, err := LoadToolVersions(tmpFile)
			require.NoError(t, err)
			assert.Equal(t, tc.expectedTools, toolVersions.Tools)
		})
	}
}

func TestRemoveToolFromVersions(t *testing.T) {
	testCases := []struct {
		name           string
		initialContent string
		toolToRemove   string
		expectedTools  map[string][]string
	}{
		{
			name:           "remove existing tool",
			initialContent: "terraform 1.5.0\nhelm 3.12.0\nkubectl 1.28.0\n",
			toolToRemove:   "helm",
			expectedTools: map[string][]string{
				"terraform": {"1.5.0"},
				"kubectl":   {"1.28.0"},
			},
		},
		{
			name:           "remove non-existent tool",
			initialContent: "terraform 1.5.0\nhelm 3.12.0\n",
			toolToRemove:   "kubectl",
			expectedTools: map[string][]string{
				"terraform": {"1.5.0"},
				"helm":      {"3.12.0"},
			},
		},
		{
			name:           "remove last tool",
			initialContent: "terraform 1.5.0\n",
			toolToRemove:   "terraform",
			expectedTools:  map[string][]string{},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Create temporary file
			tmpFile := filepath.Join(t.TempDir(), ".tool-versions")
			err := os.WriteFile(tmpFile, []byte(tc.initialContent), 0644)
			require.NoError(t, err)

			// Remove tool from versions
			err = RemoveToolFromVersions(tmpFile, tc.toolToRemove)
			require.NoError(t, err)

			// Load and verify
			toolVersions, err := LoadToolVersions(tmpFile)
			require.NoError(t, err)
			assert.Equal(t, tc.expectedTools, toolVersions.Tools)
		})
	}
}

func TestGetToolVersion(t *testing.T) {
	testCases := []struct {
		name            string
		content         string
		tool            string
		expectedVersion string
		expectedExists  bool
		shouldError     bool
		errorContains   string
	}{
		{
			name:            "existing tool",
			content:         "terraform 1.5.0\nhelm 3.12.0\n",
			tool:            "terraform",
			expectedVersion: "1.5.0",
			expectedExists:  true,
			shouldError:     false,
		},
		{
			name:            "non-existent tool",
			content:         "terraform 1.5.0\nhelm 3.12.0\n",
			tool:            "kubectl",
			expectedVersion: "",
			expectedExists:  false,
			shouldError:     false,
		},
		{
			name:            "tool without version (invalid)",
			content:         "terraform 1.5.0\nhelm\n",
			tool:            "helm",
			expectedVersion: "",
			expectedExists:  false,
			shouldError:     true,
			errorContains:   "missing version",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Create temporary file
			tmpFile := filepath.Join(t.TempDir(), ".tool-versions")
			err := os.WriteFile(tmpFile, []byte(tc.content), 0644)
			require.NoError(t, err)

			// Get tool version
			version, exists, err := GetToolVersion(tmpFile, tc.tool)
			if tc.shouldError {
				assert.Error(t, err)
				if tc.errorContains != "" {
					assert.Contains(t, err.Error(), tc.errorContains)
				}
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.expectedVersion, version)
			assert.Equal(t, tc.expectedExists, exists)
		})
	}
}

func TestHasToolVersion(t *testing.T) {
	testCases := []struct {
		name           string
		content        string
		tool           string
		version        string
		expectedResult bool
		shouldError    bool
		errorContains  string
	}{
		{
			name:           "matching tool and version",
			content:        "terraform 1.5.0\nhelm 3.12.0\n",
			tool:           "terraform",
			version:        "1.5.0",
			expectedResult: true,
			shouldError:    false,
		},
		{
			name:           "matching tool, different version",
			content:        "terraform 1.5.0\nhelm 3.12.0\n",
			tool:           "terraform",
			version:        "1.4.0",
			expectedResult: false,
			shouldError:    false,
		},
		{
			name:           "non-existent tool",
			content:        "terraform 1.5.0\nhelm 3.12.0\n",
			tool:           "kubectl",
			version:        "1.28.0",
			expectedResult: false,
			shouldError:    false,
		},
		{
			name:           "tool without version (invalid)",
			content:        "terraform 1.5.0\nhelm\n",
			tool:           "helm",
			version:        "",
			expectedResult: false,
			shouldError:    true,
			errorContains:  "missing version",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Create temporary file
			tmpFile := filepath.Join(t.TempDir(), ".tool-versions")
			err := os.WriteFile(tmpFile, []byte(tc.content), 0644)
			require.NoError(t, err)

			// Check if tool version exists
			result, err := HasToolVersion(tmpFile, tc.tool, tc.version)
			if tc.shouldError {
				assert.Error(t, err)
				if tc.errorContains != "" {
					assert.Contains(t, err.Error(), tc.errorContains)
				}
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.expectedResult, result)
		})
	}
}

func TestParseToolVersionsLine(t *testing.T) {
	testCases := []struct {
		name            string
		line            string
		expectedTool    string
		expectedVersion string
		expectedValid   bool
	}{
		{
			name:            "valid tool with version (single space)",
			line:            "terraform 1.5.0",
			expectedTool:    "terraform",
			expectedVersion: "1.5.0",
			expectedValid:   true,
		},
		{
			name:            "multiple spaces (invalid)",
			line:            "terraform    1.5.0",
			expectedTool:    "",
			expectedVersion: "",
			expectedValid:   false,
		},
		{
			name:            "tab separator (invalid)",
			line:            "terraform\t1.5.0",
			expectedTool:    "",
			expectedVersion: "",
			expectedValid:   false,
		},
		{
			name:            "trailing space (valid)",
			line:            "terraform 1.5.0 ",
			expectedTool:    "terraform",
			expectedVersion: "1.5.0",
			expectedValid:   true,
		},
		{
			name:            "leading space (valid)",
			line:            " terraform 1.5.0",
			expectedTool:    "terraform",
			expectedVersion: "1.5.0",
			expectedValid:   true,
		},
		{
			name:            "tool without version (invalid)",
			line:            "helm",
			expectedTool:    "",
			expectedVersion: "",
			expectedValid:   false,
		},
		{
			name:            "comment line",
			line:            "# This is a comment",
			expectedTool:    "",
			expectedVersion: "",
			expectedValid:   false,
		},
		{
			name:            "empty line",
			line:            "",
			expectedTool:    "",
			expectedVersion: "",
			expectedValid:   false,
		},
		{
			name:            "whitespace only",
			line:            "   ",
			expectedTool:    "",
			expectedVersion: "",
			expectedValid:   false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			tool, version, valid := ParseToolVersionsLine(tc.line)
			assert.Equal(t, tc.expectedTool, tool)
			assert.Equal(t, tc.expectedVersion, version)
			assert.Equal(t, tc.expectedValid, valid)
		})
	}
}

func TestValidateToolVersionsFile(t *testing.T) {
	testCases := []struct {
		name          string
		content       string
		expectedError bool
		errorContains string
	}{
		{
			name:          "valid file",
			content:       "terraform 1.5.0\nhelm 3.12.0\n",
			expectedError: false,
		},
		{
			name:          "valid file with comments",
			content:       "# Comment\nterraform 1.5.0\n# Another comment\nhelm 3.12.0\n",
			expectedError: false,
		},
		{
			name:          "valid file with empty lines",
			content:       "terraform 1.5.0\n\nhelm 3.12.0\n",
			expectedError: false,
		},
		{
			name:          "invalid line format",
			content:       "terraform 1.5.0 extra\nhelm 3.12.0\n",
			expectedError: true,
			errorContains: "invalid format at line 1",
		},
		{
			name:          "empty file",
			content:       "",
			expectedError: false,
		},
		{
			name:          "multiple spaces",
			content:       "terraform    1.5.0\nhelm 3.12.0\n",
			expectedError: true,
			errorContains: "invalid format at line 1",
		},
		{
			name:          "tab separator",
			content:       "terraform\t1.5.0\nhelm 3.12.0\n",
			expectedError: true,
			errorContains: "invalid format at line 1",
		},
		{
			name:          "trailing space",
			content:       "terraform 1.5.0 \nhelm 3.12.0\n",
			expectedError: false,
		},
		{
			name:          "leading space",
			content:       " terraform 1.5.0\nhelm 3.12.0\n",
			expectedError: false,
		},
		{
			name:          "valid single space",
			content:       "terraform 1.5.0\nhelm 3.12.0\n",
			expectedError: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Create temporary file
			tmpFile := filepath.Join(t.TempDir(), ".tool-versions")
			err := os.WriteFile(tmpFile, []byte(tc.content), 0644)
			require.NoError(t, err)

			// Validate file
			err = ValidateToolVersionsFile(tmpFile)

			if tc.expectedError {
				assert.Error(t, err)
				if tc.errorContains != "" {
					assert.Contains(t, err.Error(), tc.errorContains)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestMergeToolVersions(t *testing.T) {
	testCases := []struct {
		name           string
		base           *ToolVersions
		override       *ToolVersions
		expectedResult *ToolVersions
	}{
		{
			name: "merge with no conflicts",
			base: &ToolVersions{
				Tools: map[string][]string{
					"terraform": {"1.5.0"},
					"helm":      {"3.12.0"},
				},
			},
			override: &ToolVersions{
				Tools: map[string][]string{
					"kubectl": {"1.28.0"},
				},
			},
			expectedResult: &ToolVersions{
				Tools: map[string][]string{
					"terraform": {"1.5.0"},
					"helm":      {"3.12.0"},
					"kubectl":   {"1.28.0"},
				},
			},
		},
		{
			name: "merge with override",
			base: &ToolVersions{
				Tools: map[string][]string{
					"terraform": {"1.5.0"},
					"helm":      {"3.12.0"},
				},
			},
			override: &ToolVersions{
				Tools: map[string][]string{
					"terraform": {"1.6.0"},
					"kubectl":   {"1.28.0"},
				},
			},
			expectedResult: &ToolVersions{
				Tools: map[string][]string{
					"terraform": {"1.6.0"},
					"helm":      {"3.12.0"},
					"kubectl":   {"1.28.0"},
				},
			},
		},
		{
			name: "merge empty base",
			base: &ToolVersions{
				Tools: map[string][]string{},
			},
			override: &ToolVersions{
				Tools: map[string][]string{
					"terraform": {"1.5.0"},
				},
			},
			expectedResult: &ToolVersions{
				Tools: map[string][]string{
					"terraform": {"1.5.0"},
				},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := MergeToolVersions(tc.base, tc.override)
			assert.Equal(t, tc.expectedResult.Tools, result.Tools)
		})
	}
}

func TestGetToolVersionsFileContent(t *testing.T) {
	testCases := []struct {
		name           string
		content        string
		expectedResult string
	}{
		{
			name:           "simple content",
			content:        "terraform 1.5.0\nhelm 3.12.0\n",
			expectedResult: "terraform 1.5.0\nhelm 3.12.0\n",
		},
		{
			name:           "content with comments",
			content:        "# Comment\nterraform 1.5.0\n# Another comment\n",
			expectedResult: "# Comment\nterraform 1.5.0\n# Another comment\n",
		},
		{
			name:           "empty content",
			content:        "",
			expectedResult: "",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Create temporary file
			tmpFile := filepath.Join(t.TempDir(), ".tool-versions")
			err := os.WriteFile(tmpFile, []byte(tc.content), 0644)
			require.NoError(t, err)

			// Get file content
			result, err := GetToolVersionsFileContent(tmpFile)
			require.NoError(t, err)
			assert.Equal(t, tc.expectedResult, result)
		})
	}
}

func TestToolVersionsAddRemoveCommands(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, ".tool-versions")

	// Add a tool
	addCmd := &cobra.Command{
		Use:   "add <tool> <version>",
		Short: "Add or update a tool and version in .tool-versions",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			filePath, _ := cmd.Flags().GetString("file")
			if filePath == "" {
				filePath = ".tool-versions"
			}
			tool := args[0]
			version := args[1]
			return AddToolToVersions(filePath, tool, version)
		},
	}
	addCmd.Flags().String("file", filePath, "Path to .tool-versions file")
	addCmd.SetArgs([]string{"terraform", "1.5.0"})
	err := addCmd.Execute()
	assert.NoError(t, err)

	// Check file contents
	toolVersions, err := LoadToolVersions(filePath)
	assert.NoError(t, err)
	assert.Equal(t, map[string][]string{"terraform": {"1.5.0"}}, toolVersions.Tools)

	// Add another tool
	addCmd2 := &cobra.Command{
		Use:   "add <tool> <version>",
		Short: "Add or update a tool and version in .tool-versions",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			filePath, _ := cmd.Flags().GetString("file")
			if filePath == "" {
				filePath = ".tool-versions"
			}
			tool := args[0]
			version := args[1]
			return AddToolToVersions(filePath, tool, version)
		},
	}
	addCmd2.Flags().String("file", filePath, "Path to .tool-versions file")
	addCmd2.SetArgs([]string{"helm", "3.12.0"})
	err = addCmd2.Execute()
	assert.NoError(t, err)
	toolVersions, err = LoadToolVersions(filePath)
	assert.NoError(t, err)
	assert.Equal(t, map[string][]string{"terraform": {"1.5.0"}, "helm": {"3.12.0"}}, toolVersions.Tools)

	// Remove a tool
	removeCmd := &cobra.Command{
		Use:   "remove <tool>",
		Short: "Remove a tool from .tool-versions",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			filePath, _ := cmd.Flags().GetString("file")
			if filePath == "" {
				filePath = ".tool-versions"
			}
			tool := args[0]
			return RemoveToolFromVersions(filePath, tool)
		},
	}
	removeCmd.Flags().String("file", filePath, "Path to .tool-versions file")
	removeCmd.SetArgs([]string{"terraform"})
	err = removeCmd.Execute()
	assert.NoError(t, err)
	toolVersions, err = LoadToolVersions(filePath)
	assert.NoError(t, err)
	assert.Equal(t, map[string][]string{"helm": {"3.12.0"}}, toolVersions.Tools)

	// Remove a non-existent tool (should not error)
	removeCmd2 := &cobra.Command{
		Use:   "remove <tool>",
		Short: "Remove a tool from .tool-versions",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			filePath, _ := cmd.Flags().GetString("file")
			if filePath == "" {
				filePath = ".tool-versions"
			}
			tool := args[0]
			return RemoveToolFromVersions(filePath, tool)
		},
	}
	removeCmd2.Flags().String("file", filePath, "Path to .tool-versions file")
	removeCmd2.SetArgs([]string{"nonexistent"})
	err = removeCmd2.Execute()
	assert.NoError(t, err)
	toolVersions, err = LoadToolVersions(filePath)
	assert.NoError(t, err)
	assert.Equal(t, map[string][]string{"helm": {"3.12.0"}}, toolVersions.Tools)

	// Add with missing version (should error)
	addCmd3 := &cobra.Command{
		Use:   "add <tool> <version>",
		Short: "Add or update a tool and version in .tool-versions",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			filePath, _ := cmd.Flags().GetString("file")
			if filePath == "" {
				filePath = ".tool-versions"
			}
			tool := args[0]
			version := args[1]
			return AddToolToVersions(filePath, tool, version)
		},
	}
	addCmd3.Flags().String("file", filePath, "Path to .tool-versions file")
	addCmd3.SetArgs([]string{"foo"})
	err = addCmd3.Execute()
	assert.Error(t, err)

	// Remove with missing tool (should error)
	removeCmd3 := &cobra.Command{
		Use:   "remove <tool>",
		Short: "Remove a tool from .tool-versions",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			filePath, _ := cmd.Flags().GetString("file")
			if filePath == "" {
				filePath = ".tool-versions"
			}
			tool := args[0]
			return RemoveToolFromVersions(filePath, tool)
		},
	}
	removeCmd3.Flags().String("file", filePath, "Path to .tool-versions file")
	removeCmd3.SetArgs([]string{})
	err = removeCmd3.Execute()
	assert.Error(t, err)
}

func TestInstallRespectsAliasesAndRegistersTool(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, ".tool-versions")
	os.Setenv("HOME", dir) // ensure no interference from user config

	// Remove any pre-existing .tool-versions
	_ = os.Remove(filePath)

	// Simulate install: should resolve alias 'terraform' to 'hashicorp/terraform'
	installer := NewInstaller()
	owner, repo, err := installer.parseToolSpec("terraform")
	assert.NoError(t, err)
	assert.Equal(t, "hashicorp", owner)
	assert.Equal(t, "terraform", repo)

	// Simulate install logic (mock actual install)
	version := "1.9.8"
	err = AddToolToVersions(filePath, repo, version)
	assert.NoError(t, err)

	// Check .tool-versions file
	toolVersions, err := LoadToolVersions(filePath)
	assert.NoError(t, err)
	assert.Equal(t, map[string][]string{"terraform": {"1.9.8"}}, toolVersions.Tools)
}

// Update fakeInstaller to implement Resolve(toolName string) (string, string, error) instead of resolveToolName.
// Remove any duplicate mockToolResolver definitions.
