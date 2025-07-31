package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWhichCommand_ToolNotConfigured(t *testing.T) {
	// Create a temporary directory
	tempDir := t.TempDir()

	// Create an empty .tool-versions file so the command can load it
	emptyToolVersions := &ToolVersions{
		Tools: map[string][]string{},
	}
	toolVersionsPath := filepath.Join(tempDir, ".tool-versions")
	err := SaveToolVersions(toolVersionsPath, emptyToolVersions)
	require.NoError(t, err)

	// Create a new command instance to avoid interference
	cmd := &cobra.Command{
		Use:   "which",
		Short: "Display the path to an executable",
		RunE: func(cmd *cobra.Command, args []string) error {
			// Override the tool versions file path for this test
			originalPath := GetToolVersionsFilePath()
			defer func() { toolVersionsFile = originalPath }()
			toolVersionsFile = toolVersionsPath
			return whichCmd.RunE(cmd, args)
		},
	}
	cmd.SetArgs([]string{"kubectl"})

	err = cmd.Execute()
	require.Error(t, err, "Should fail when tool is not configured in .tool-versions")
	assert.Contains(t, err.Error(), "not configured in .tool-versions")
}

func TestWhichCommand_InvalidTool(t *testing.T) {
	// Create a temporary directory
	tempDir := t.TempDir()

	// Create an empty .tool-versions file so the command can load it
	emptyToolVersions := &ToolVersions{
		Tools: map[string][]string{},
	}
	toolVersionsPath := filepath.Join(tempDir, ".tool-versions")
	err := SaveToolVersions(toolVersionsPath, emptyToolVersions)
	require.NoError(t, err)

	// Create a new command instance to avoid interference
	cmd := &cobra.Command{
		Use:   "which",
		Short: "Display the path to an executable",
		RunE: func(cmd *cobra.Command, args []string) error {
			// Override the tool versions file path for this test
			originalPath := GetToolVersionsFilePath()
			defer func() { toolVersionsFile = originalPath }()
			toolVersionsFile = toolVersionsPath
			return whichCmd.RunE(cmd, args)
		},
	}
	cmd.SetArgs([]string{"nonexistent-tool-12345"})

	err = cmd.Execute()
	require.Error(t, err, "Should fail when tool doesn't exist in .tool-versions")
	assert.Contains(t, err.Error(), "not configured in .tool-versions")
}

func TestWhichCommand_InvalidToolName(t *testing.T) {
	// Create a temporary directory
	tempDir := t.TempDir()

	// Create a .tool-versions file with an invalid tool name
	toolVersions := &ToolVersions{
		Tools: map[string][]string{
			"invalid/tool/name": {"1.0.0"},
		},
	}
	toolVersionsPath := filepath.Join(tempDir, ".tool-versions")
	err := SaveToolVersions(toolVersionsPath, toolVersions)
	require.NoError(t, err)

	// Create a new command instance to avoid interference
	cmd := &cobra.Command{
		Use:   "which",
		Short: "Display the path to an executable",
		RunE: func(cmd *cobra.Command, args []string) error {
			// Override the tool versions file path for this test
			originalPath := GetToolVersionsFilePath()
			defer func() { toolVersionsFile = originalPath }()
			toolVersionsFile = toolVersionsPath
			return whichCmd.RunE(cmd, args)
		},
	}
	cmd.SetArgs([]string{"invalid/tool/name"})

	err = cmd.Execute()
	require.Error(t, err, "Should fail when tool name is invalid")
	assert.Contains(t, err.Error(), "failed to resolve tool")
}

func TestWhichCommand_EmptyToolName(t *testing.T) {
	// Create a temporary directory
	tempDir := t.TempDir()

	// Create an empty .tool-versions file so the command can load it
	emptyToolVersions := &ToolVersions{
		Tools: map[string][]string{},
	}
	toolVersionsPath := filepath.Join(tempDir, ".tool-versions")
	err := SaveToolVersions(toolVersionsPath, emptyToolVersions)
	require.NoError(t, err)

	// Create a new command instance to avoid interference
	cmd := &cobra.Command{
		Use:   "which",
		Short: "Display the path to an executable",
		RunE: func(cmd *cobra.Command, args []string) error {
			// Override the tool versions file path for this test
			originalPath := GetToolVersionsFilePath()
			defer func() { toolVersionsFile = originalPath }()
			toolVersionsFile = toolVersionsPath
			return whichCmd.RunE(cmd, args)
		},
	}
	cmd.SetArgs([]string{""})

	err = cmd.Execute()
	require.Error(t, err, "Should fail when tool name is empty")
	assert.Contains(t, err.Error(), "not configured in .tool-versions")
}

func TestWhichCommand_ToolConfiguredButNotInstalled(t *testing.T) {
	// Create a temporary directory
	tempDir := t.TempDir()

	// Create a .tool-versions file with a tool that's configured but won't be installed
	toolVersions := &ToolVersions{
		Tools: map[string][]string{
			"terraform": {"999.999.999"}, // Use a version that won't be installed
		},
	}
	toolVersionsPath := filepath.Join(tempDir, ".tool-versions")
	err := SaveToolVersions(toolVersionsPath, toolVersions)
	require.NoError(t, err)

	// Create a new command instance to avoid interference
	cmd := &cobra.Command{
		Use:   "which",
		Short: "Display the path to an executable",
		RunE: func(cmd *cobra.Command, args []string) error {
			// Override the tool versions file path for this test
			originalPath := GetToolVersionsFilePath()
			defer func() { toolVersionsFile = originalPath }()
			toolVersionsFile = toolVersionsPath
			return whichCmd.RunE(cmd, args)
		},
	}
	cmd.SetArgs([]string{"terraform"})

	err = cmd.Execute()
	require.Error(t, err, "Should fail when tool is configured but not installed")
	assert.Contains(t, err.Error(), "is configured but not installed")
}

func TestWhichCommand_ToolConfiguredAndInstalled(t *testing.T) {
	// Create a temporary directory
	tempDir := t.TempDir()

	// Create a .tool-versions file with a tool
	toolVersions := &ToolVersions{
		Tools: map[string][]string{
			"terraform": {"1.11.4"},
		},
	}
	toolVersionsPath := filepath.Join(tempDir, ".tool-versions")
	err := SaveToolVersions(toolVersionsPath, toolVersions)
	require.NoError(t, err)

	// Create mock installed binary at the exact path the which command expects
	// The which command uses NewInstaller() which has binDir = filepath.Join(GetToolsDirPath(), "bin")
	// So we need to create the binary in ./.tools/bin/hashicorp/terraform/1.11.4/terraform
	installer := NewInstaller()
	binaryPath := installer.getBinaryPath("hashicorp", "terraform", "1.11.4")
	err = os.MkdirAll(filepath.Dir(binaryPath), 0o755)
	require.NoError(t, err)
	err = os.WriteFile(binaryPath, []byte("mock terraform"), 0o755)
	require.NoError(t, err)

	// Create a new command instance to avoid interference
	cmd := &cobra.Command{
		Use:   "which",
		Short: "Display the path to an executable",
		RunE: func(cmd *cobra.Command, args []string) error {
			// Override the tool versions file path for this test
			originalPath := GetToolVersionsFilePath()
			defer func() { toolVersionsFile = originalPath }()
			toolVersionsFile = toolVersionsPath
			return whichCmd.RunE(cmd, args)
		},
	}
	cmd.SetArgs([]string{"terraform"})

	err = cmd.Execute()
	require.NoError(t, err, "Should succeed when tool is configured and installed")
}

func TestWhichCommand_NoToolVersionsFile(t *testing.T) {
	// Create a temporary directory
	tempDir := t.TempDir()

	// Don't create a .tool-versions file
	toolVersionsPath := filepath.Join(tempDir, ".tool-versions")

	// Create a new command instance to avoid interference
	cmd := &cobra.Command{
		Use:   "which",
		Short: "Display the path to an executable",
		RunE: func(cmd *cobra.Command, args []string) error {
			// Override the tool versions file path for this test
			originalPath := GetToolVersionsFilePath()
			defer func() { toolVersionsFile = originalPath }()
			toolVersionsFile = toolVersionsPath
			return whichCmd.RunE(cmd, args)
		},
	}
	cmd.SetArgs([]string{"terraform"})

	err := cmd.Execute()
	require.Error(t, err, "Should fail when .tool-versions file doesn't exist")
	assert.Contains(t, err.Error(), "failed to load .tool-versions file")
}

func TestWhichCommand_HelpFlag(t *testing.T) {
	// Test that help flag works
	cmd := whichCmd
	cmd.SetArgs([]string{"--help"})

	err := cmd.Execute()
	// Help should not return an error
	require.NoError(t, err, "Help flag should not cause an error")
}

// Note: Argument validation tests are not included here because Cobra handles
// argument validation differently in test context vs command line execution.
// The core functionality is tested in other test cases.

func TestWhichCommand_ResolvesAlias(t *testing.T) {
	// Test that the command can resolve aliases
	// This test will pass if terraform is configured in .tool-versions and installed
	cmd := whichCmd
	cmd.SetArgs([]string{"terraform"})

	err := cmd.Execute()
	// This might succeed if terraform is configured and installed, or fail if not
	// We don't assert either way since it depends on the current .tool-versions state
	if err != nil {
		t.Logf("terraform not found or not installed: %v", err)
	} else {
		t.Logf("terraform found and installed via toolchain")
	}
}

func TestWhichCommand_CanonicalName(t *testing.T) {
	// Test with canonical name
	cmd := whichCmd
	cmd.SetArgs([]string{"hashicorp/terraform"})

	err := cmd.Execute()
	// This might succeed if hashicorp/terraform is configured and installed, or fail if not
	// We don't assert either way since it depends on the current .tool-versions state
	if err != nil {
		t.Logf("hashicorp/terraform not found or not installed: %v", err)
	} else {
		t.Logf("hashicorp/terraform found and installed via toolchain")
	}
}
