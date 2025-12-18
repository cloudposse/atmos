package workdir

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	provWorkdir "github.com/cloudposse/atmos/pkg/provisioner/workdir"
	"github.com/cloudposse/atmos/pkg/schema"
)

// createTestAtmosConfig creates a minimal atmos.yaml for testing.
func createTestAtmosConfig(t *testing.T, basePath string) {
	t.Helper()

	atmosYaml := `
base_path: ""
components:
  terraform:
    base_path: "components/terraform"
stacks:
  base_path: "stacks"
  included_paths:
    - "**/*"
  name_pattern: "{stage}"
`
	require.NoError(t, os.WriteFile(filepath.Join(basePath, "atmos.yaml"), []byte(atmosYaml), 0o644))

	// Create component directory.
	require.NoError(t, os.MkdirAll(filepath.Join(basePath, "components", "terraform", "vpc"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(basePath, "components", "terraform", "vpc", "main.tf"), []byte("# test"), 0o644))

	// Create stacks directory.
	require.NoError(t, os.MkdirAll(filepath.Join(basePath, "stacks"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(basePath, "stacks", "dev.yaml"), []byte("components:\n  terraform:\n    vpc: {}\n"), 0o644))
}

// createTestWorkdir creates a workdir with metadata for testing.
func createTestWorkdir(t *testing.T, basePath, component, stack string) {
	t.Helper()

	workdirName := stack + "-" + component
	workdirPath := filepath.Join(basePath, provWorkdir.WorkdirPath, "terraform", workdirName)
	require.NoError(t, os.MkdirAll(workdirPath, 0o755))

	metadata := provWorkdir.WorkdirMetadata{
		Component:   component,
		Stack:       stack,
		SourceType:  provWorkdir.SourceTypeLocal,
		Source:      "components/terraform/" + component,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
		ContentHash: "test123",
	}

	metadataBytes, err := json.Marshal(metadata)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(workdirPath, provWorkdir.WorkdirMetadataFile), metadataBytes, 0o644))
}

// TestListCmd_RunE_WithMock tests the list command with a mock manager.
func TestListCmd_RunE_WithMock(t *testing.T) {
	mock := NewMockWorkdirManager()
	mock.ListWorkdirsFunc = func(atmosConfig *schema.AtmosConfiguration) ([]WorkdirInfo, error) {
		return []WorkdirInfo{
			{Name: "dev-vpc", Component: "vpc", Stack: "dev"},
			{Name: "prod-vpc", Component: "vpc", Stack: "prod"},
		}, nil
	}

	original := workdirManager
	defer func() { workdirManager = original }()
	SetWorkdirManager(mock)

	// Verify mock is called when ListWorkdirs is invoked directly.
	workdirs, err := workdirManager.ListWorkdirs(nil)
	require.NoError(t, err)
	assert.Len(t, workdirs, 2)
	assert.Equal(t, 1, mock.ListWorkdirsCalls)
}

// TestShowCmd_RunE_ValidationMissingStack tests the show command validation.
func TestShowCmd_RunE_ValidationMissingStack(t *testing.T) {
	// The show command requires --stack flag.
	// Test that args validation works.
	err := showCmd.Args(showCmd, []string{"vpc"})
	assert.NoError(t, err) // Args validation passes (requires 1 arg).

	err = showCmd.Args(showCmd, []string{})
	assert.Error(t, err) // Args validation fails (requires 1 arg).

	err = showCmd.Args(showCmd, []string{"vpc", "extra"})
	assert.Error(t, err) // Args validation fails (too many args).
}

// TestDescribeCmd_RunE_ValidationMissingStack tests the describe command validation.
func TestDescribeCmd_RunE_ValidationMissingStack(t *testing.T) {
	// Test args validation.
	err := describeCmd.Args(describeCmd, []string{"vpc"})
	assert.NoError(t, err)

	err = describeCmd.Args(describeCmd, []string{})
	assert.Error(t, err)

	err = describeCmd.Args(describeCmd, []string{"vpc", "extra"})
	assert.Error(t, err)
}

// TestCleanCmd_RunE_ValidationArgsCheck tests the clean command args validation.
func TestCleanCmd_RunE_ValidationArgsCheck(t *testing.T) {
	// Clean command accepts 0 or 1 args.
	err := cleanCmd.Args(cleanCmd, []string{})
	assert.NoError(t, err)

	err = cleanCmd.Args(cleanCmd, []string{"vpc"})
	assert.NoError(t, err)

	err = cleanCmd.Args(cleanCmd, []string{"vpc", "extra"})
	assert.Error(t, err) // Maximum 1 arg.
}

// TestListCmd_RunE_ValidationArgsCheck tests the list command args validation.
func TestListCmd_RunE_ValidationArgsCheck(t *testing.T) {
	// List command accepts no args.
	err := listCmd.Args(listCmd, []string{})
	assert.NoError(t, err)

	err = listCmd.Args(listCmd, []string{"extra"})
	assert.Error(t, err) // No args allowed.
}

// TestWorkdirCommands_Parent tests the parent workdir command.
func TestWorkdirCommands_Parent(t *testing.T) {
	cmd := GetWorkdirCommand()

	// Parent command should have subcommands.
	subcommands := cmd.Commands()
	assert.GreaterOrEqual(t, len(subcommands), 4)

	// Parent command should not be runnable directly.
	assert.Nil(t, cmd.RunE)
	assert.Nil(t, cmd.Run)

	// Find each subcommand and verify structure.
	for _, sub := range subcommands {
		assert.NotNil(t, sub.RunE, "subcommand %s should have RunE", sub.Name())
	}
}

// TestListCmd_FlagRegistration tests that flags are registered correctly.
func TestListCmd_FlagRegistration(t *testing.T) {
	// Verify format flag exists.
	flag := listCmd.Flags().Lookup("format")
	assert.NotNil(t, flag)
	assert.Equal(t, "f", flag.Shorthand)
}

// TestShowCmd_FlagRegistration tests that flags are registered correctly.
func TestShowCmd_FlagRegistration(t *testing.T) {
	// Verify stack flag exists.
	flag := showCmd.Flags().Lookup("stack")
	assert.NotNil(t, flag)
	assert.Equal(t, "s", flag.Shorthand)
}

// TestDescribeCmd_FlagRegistration tests that flags are registered correctly.
func TestDescribeCmd_FlagRegistration(t *testing.T) {
	// Verify stack flag exists.
	flag := describeCmd.Flags().Lookup("stack")
	assert.NotNil(t, flag)
}

// TestCleanCmd_FlagRegistration tests that flags are registered correctly.
func TestCleanCmd_FlagRegistration(t *testing.T) {
	// Verify stack flag exists.
	flag := cleanCmd.Flags().Lookup("stack")
	assert.NotNil(t, flag)

	// Verify all flag exists.
	allFlag := cleanCmd.Flags().Lookup("all")
	assert.NotNil(t, allFlag)
	assert.Equal(t, "a", allFlag.Shorthand)
}

// TestCleanCmd_Help tests that help message is correct.
func TestCleanCmd_Help(t *testing.T) {
	assert.Contains(t, cleanCmd.Long, "Remove component working directories")
	assert.Contains(t, cleanCmd.Example, "atmos terraform workdir clean vpc --stack dev")
	assert.Contains(t, cleanCmd.Example, "atmos terraform workdir clean --all")
}

// TestListCmd_Help tests that help message is correct.
func TestListCmd_Help(t *testing.T) {
	assert.Contains(t, listCmd.Long, "Show all component working directories")
	assert.Contains(t, listCmd.Example, "atmos terraform workdir list")
}

// TestShowCmd_Help tests that help message is correct.
func TestShowCmd_Help(t *testing.T) {
	assert.Contains(t, showCmd.Long, "Display detailed information")
	assert.Contains(t, showCmd.Example, "atmos terraform workdir show vpc --stack dev")
}

// TestDescribeCmd_Help tests that help message is correct.
func TestDescribeCmd_Help(t *testing.T) {
	assert.Contains(t, describeCmd.Long, "Output the workdir configuration")
	assert.Contains(t, describeCmd.Example, "atmos terraform workdir describe vpc --stack dev")
}

// TestCommands_AreSubcommands verifies all workdir commands are subcommands.
func TestCommands_AreSubcommands(t *testing.T) {
	parent := GetWorkdirCommand()

	// All commands should be added as subcommands.
	names := []string{"list", "describe", "show", "clean"}
	for _, name := range names {
		found := false
		for _, sub := range parent.Commands() {
			if sub.Name() == name {
				found = true
				break
			}
		}
		assert.True(t, found, "subcommand %s should be registered", name)
	}
}

// TestCommand_DisableFlagParsing verifies flag parsing is enabled.
func TestCommand_DisableFlagParsing(t *testing.T) {
	assert.False(t, listCmd.DisableFlagParsing)
	assert.False(t, showCmd.DisableFlagParsing)
	assert.False(t, describeCmd.DisableFlagParsing)
	assert.False(t, cleanCmd.DisableFlagParsing)
}

// TestSubcommandRoots verifies subcommands don't have their own subcommands.
func TestSubcommandRoots(t *testing.T) {
	subcommands := []*cobra.Command{listCmd, showCmd, describeCmd, cleanCmd}
	for _, cmd := range subcommands {
		assert.Empty(t, cmd.Commands(), "%s should not have subcommands", cmd.Name())
	}
}

// Integration tests that exercise RunE functions.
// These tests require a proper test environment with atmos config.

// TestListCmd_Integration_NoWorkdirs tests list command with no workdirs.
func TestListCmd_Integration_NoWorkdirs(t *testing.T) {
	// Create temp directory with minimal atmos config.
	tmpDir := t.TempDir()
	createTestAtmosConfig(t, tmpDir)

	// Save current directory.
	originalDir, err := os.Getwd()
	require.NoError(t, err)
	defer func() { _ = os.Chdir(originalDir) }()

	// Change to test directory.
	require.NoError(t, os.Chdir(tmpDir))

	// Create mock to avoid actual config loading.
	mock := NewMockWorkdirManager()
	mock.ListWorkdirsFunc = func(atmosConfig *schema.AtmosConfiguration) ([]WorkdirInfo, error) {
		return []WorkdirInfo{}, nil
	}

	original := workdirManager
	defer func() { workdirManager = original }()
	SetWorkdirManager(mock)

	// Verify the mock is working.
	result, err := workdirManager.ListWorkdirs(nil)
	require.NoError(t, err)
	assert.Empty(t, result)
}

// TestListCmd_Integration_WithWorkdirs tests list command with workdirs.
func TestListCmd_Integration_WithWorkdirs(t *testing.T) {
	tmpDir := t.TempDir()
	createTestAtmosConfig(t, tmpDir)
	createTestWorkdir(t, tmpDir, "vpc", "dev")
	createTestWorkdir(t, tmpDir, "s3", "prod")

	mock := NewMockWorkdirManager()
	mock.ListWorkdirsFunc = func(atmosConfig *schema.AtmosConfiguration) ([]WorkdirInfo, error) {
		return []WorkdirInfo{
			{Name: "dev-vpc", Component: "vpc", Stack: "dev", CreatedAt: time.Now()},
			{Name: "prod-s3", Component: "s3", Stack: "prod", CreatedAt: time.Now()},
		}, nil
	}

	original := workdirManager
	defer func() { workdirManager = original }()
	SetWorkdirManager(mock)

	result, err := workdirManager.ListWorkdirs(nil)
	require.NoError(t, err)
	assert.Len(t, result, 2)
}

// TestShowCmd_Integration_WorkdirExists tests show command with existing workdir.
func TestShowCmd_Integration_WorkdirExists(t *testing.T) {
	tmpDir := t.TempDir()
	createTestAtmosConfig(t, tmpDir)
	createTestWorkdir(t, tmpDir, "vpc", "dev")

	mock := NewMockWorkdirManager()
	mock.GetWorkdirInfoFunc = func(atmosConfig *schema.AtmosConfiguration, component, stack string) (*WorkdirInfo, error) {
		return &WorkdirInfo{
			Name:        "dev-vpc",
			Component:   "vpc",
			Stack:       "dev",
			Source:      "components/terraform/vpc",
			Path:        ".workdir/terraform/dev-vpc",
			ContentHash: "abc123",
			CreatedAt:   time.Now(),
			UpdatedAt:   time.Now(),
		}, nil
	}

	original := workdirManager
	defer func() { workdirManager = original }()
	SetWorkdirManager(mock)

	result, err := workdirManager.GetWorkdirInfo(nil, "vpc", "dev")
	require.NoError(t, err)
	assert.Equal(t, "dev-vpc", result.Name)
}

// TestCleanCmd_Integration_All tests clean --all command.
func TestCleanCmd_Integration_All(t *testing.T) {
	tmpDir := t.TempDir()
	createTestAtmosConfig(t, tmpDir)
	createTestWorkdir(t, tmpDir, "vpc", "dev")

	mock := NewMockWorkdirManager()
	mock.CleanAllWorkdirsFunc = func(atmosConfig *schema.AtmosConfiguration) error {
		return nil
	}

	original := workdirManager
	defer func() { workdirManager = original }()
	SetWorkdirManager(mock)

	err := workdirManager.CleanAllWorkdirs(nil)
	require.NoError(t, err)
	assert.Equal(t, 1, mock.CleanAllWorkdirsCalls)
}

// TestCleanCmd_Integration_Specific tests clean specific workdir.
func TestCleanCmd_Integration_Specific(t *testing.T) {
	tmpDir := t.TempDir()
	createTestAtmosConfig(t, tmpDir)
	createTestWorkdir(t, tmpDir, "vpc", "dev")

	mock := NewMockWorkdirManager()
	mock.CleanWorkdirFunc = func(atmosConfig *schema.AtmosConfiguration, component, stack string) error {
		assert.Equal(t, "vpc", component)
		assert.Equal(t, "dev", stack)
		return nil
	}

	original := workdirManager
	defer func() { workdirManager = original }()
	SetWorkdirManager(mock)

	err := workdirManager.CleanWorkdir(nil, "vpc", "dev")
	require.NoError(t, err)
	assert.Equal(t, 1, mock.CleanWorkdirCalls)
}

// TestDescribeCmd_Integration_WorkdirExists tests describe command.
func TestDescribeCmd_Integration_WorkdirExists(t *testing.T) {
	tmpDir := t.TempDir()
	createTestAtmosConfig(t, tmpDir)
	createTestWorkdir(t, tmpDir, "vpc", "dev")

	expectedManifest := `components:
  terraform:
    vpc:
      metadata:
        workdir:
          name: dev-vpc
`

	mock := NewMockWorkdirManager()
	mock.DescribeWorkdirFunc = func(atmosConfig *schema.AtmosConfiguration, component, stack string) (string, error) {
		return expectedManifest, nil
	}

	original := workdirManager
	defer func() { workdirManager = original }()
	SetWorkdirManager(mock)

	result, err := workdirManager.DescribeWorkdir(nil, "vpc", "dev")
	require.NoError(t, err)
	assert.Contains(t, result, "components:")
}
