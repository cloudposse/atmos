package pro

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/pro/install"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestProCommandProvider(t *testing.T) {
	provider := &ProCommandProvider{}

	t.Run("command is properly initialized", func(t *testing.T) {
		cmd := provider.GetCommand()
		assert.NotNil(t, cmd)
		assert.Equal(t, "pro", cmd.Use)
		assert.Contains(t, cmd.Short, "premium features")
	})

	t.Run("command name and group", func(t *testing.T) {
		assert.Equal(t, "pro", provider.GetName())
		assert.Equal(t, "Pro Features", provider.GetGroup())
	})

	t.Run("not experimental", func(t *testing.T) {
		assert.False(t, provider.IsExperimental())
	})

	t.Run("has install subcommand", func(t *testing.T) {
		cmd := provider.GetCommand()
		found := false
		for _, sub := range cmd.Commands() {
			if sub.Use == "install" {
				found = true
				break
			}
		}
		assert.True(t, found, "expected install subcommand")
	})

	t.Run("has lock subcommand", func(t *testing.T) {
		cmd := provider.GetCommand()
		found := false
		for _, sub := range cmd.Commands() {
			if sub.Use == "lock" {
				found = true
				break
			}
		}
		assert.True(t, found, "expected lock subcommand")
	})

	t.Run("has unlock subcommand", func(t *testing.T) {
		cmd := provider.GetCommand()
		found := false
		for _, sub := range cmd.Commands() {
			if sub.Use == "unlock" {
				found = true
				break
			}
		}
		assert.True(t, found, "expected unlock subcommand")
	})
}

func TestLockCmd(t *testing.T) {
	t.Run("lock command is properly initialized", func(t *testing.T) {
		assert.NotNil(t, lockCmd)
		assert.Equal(t, "lock", lockCmd.Use)
		assert.Contains(t, lockCmd.Short, "Lock")
	})

	t.Run("lock command has required flags", func(t *testing.T) {
		componentFlag := lockCmd.PersistentFlags().Lookup("component")
		assert.NotNil(t, componentFlag)
		assert.Equal(t, "c", componentFlag.Shorthand)

		stackFlag := lockCmd.PersistentFlags().Lookup("stack")
		assert.NotNil(t, stackFlag)
		assert.Equal(t, "s", stackFlag.Shorthand)

		messageFlag := lockCmd.PersistentFlags().Lookup("message")
		assert.NotNil(t, messageFlag)
		assert.Equal(t, "m", messageFlag.Shorthand)

		ttlFlag := lockCmd.PersistentFlags().Lookup("ttl")
		assert.NotNil(t, ttlFlag)
		assert.Equal(t, "t", ttlFlag.Shorthand)
	})
}

func TestUnlockCmd(t *testing.T) {
	t.Run("unlock command is properly initialized", func(t *testing.T) {
		assert.NotNil(t, unlockCmd)
		assert.Equal(t, "unlock", unlockCmd.Use)
		assert.Contains(t, unlockCmd.Short, "Unlock")
	})

	t.Run("unlock command has required flags", func(t *testing.T) {
		componentFlag := unlockCmd.PersistentFlags().Lookup("component")
		assert.NotNil(t, componentFlag)
		assert.Equal(t, "c", componentFlag.Shorthand)

		stackFlag := unlockCmd.PersistentFlags().Lookup("stack")
		assert.NotNil(t, stackFlag)
		assert.Equal(t, "s", stackFlag.Shorthand)
	})
}

func TestInstallCmd(t *testing.T) {
	t.Run("install command is properly initialized", func(t *testing.T) {
		assert.NotNil(t, installCmd)
		assert.Equal(t, "install", installCmd.Use)
		assert.Contains(t, installCmd.Short, "Install")
	})

	t.Run("install command has flags", func(t *testing.T) {
		yesFlag := installCmd.Flags().Lookup("yes")
		assert.NotNil(t, yesFlag)
		assert.Equal(t, "y", yesFlag.Shorthand)

		forceFlag := installCmd.Flags().Lookup("force")
		assert.NotNil(t, forceFlag)
		assert.Equal(t, "", forceFlag.Shorthand)

		dryRunFlag := installCmd.Flags().Lookup("dry-run")
		assert.NotNil(t, dryRunFlag)
	})
}

func TestProCommandProvider_NilReturns(t *testing.T) {
	provider := &ProCommandProvider{}

	t.Run("flags builder is nil", func(t *testing.T) {
		assert.Nil(t, provider.GetFlagsBuilder())
	})

	t.Run("positional args builder is nil", func(t *testing.T) {
		assert.Nil(t, provider.GetPositionalArgsBuilder())
	})

	t.Run("compatibility flags is nil", func(t *testing.T) {
		assert.Nil(t, provider.GetCompatibilityFlags())
	})

	t.Run("aliases is nil", func(t *testing.T) {
		assert.Nil(t, provider.GetAliases())
	})
}

func TestReportResult(t *testing.T) {
	t.Run("all file categories", func(t *testing.T) {
		result := &install.InstallResult{
			CreatedFiles: []string{"a.yaml", "b.yaml"},
			UpdatedFiles: []string{"c.yaml"},
			SkippedFiles: []string{"d.yaml"},
		}
		// Should not panic.
		require.NotPanics(t, func() {
			reportResult(result)
		})
	})

	t.Run("empty result", func(t *testing.T) {
		result := &install.InstallResult{}
		require.NotPanics(t, func() {
			reportResult(result)
		})
	})
}

func TestReportDryRun(t *testing.T) {
	t.Run("all file categories", func(t *testing.T) {
		result := &install.InstallResult{
			CreatedFiles: []string{"a.yaml", "b.yaml"},
			UpdatedFiles: []string{"c.yaml"},
			SkippedFiles: []string{"d.yaml"},
		}
		require.NotPanics(t, func() {
			reportDryRun(result)
		})
	})

	t.Run("empty result", func(t *testing.T) {
		result := &install.InstallResult{}
		require.NotPanics(t, func() {
			reportDryRun(result)
		})
	})
}

func TestResolveInstallPaths_DefaultPaths(t *testing.T) {
	// When config cannot be loaded (no atmos.yaml), defaults should be returned.
	origDir, err := os.Getwd()
	require.NoError(t, err)

	tmpDir := t.TempDir()
	require.NoError(t, os.Chdir(tmpDir))
	t.Cleanup(func() {
		require.NoError(t, os.Chdir(origDir))
	})

	info := &schema.ConfigAndStacksInfo{}
	basePath, stacksBasePath := resolveInstallPaths(info)

	assert.Equal(t, ".", basePath)
	assert.Equal(t, "stacks", stacksBasePath)
}

func TestResolveInstallPaths_WithConfig(t *testing.T) {
	origDir, err := os.Getwd()
	require.NoError(t, err)

	tmpDir := t.TempDir()
	require.NoError(t, os.Chdir(tmpDir))
	t.Cleanup(func() {
		require.NoError(t, os.Chdir(origDir))
	})

	// Write a minimal atmos.yaml with custom stacks path.
	configContent := []byte("base_path: .\nstacks:\n  base_path: custom-stacks\n")
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "atmos.yaml"), configContent, 0o644))

	info := &schema.ConfigAndStacksInfo{}
	basePath, stacksBasePath := resolveInstallPaths(info)

	// BasePath defaults to "." when empty, stacks path comes from config.
	assert.NotEmpty(t, basePath)
	assert.Equal(t, "custom-stacks", stacksBasePath)
}

func TestResolveFromGlobalFlags(t *testing.T) {
	origDir, err := os.Getwd()
	require.NoError(t, err)

	tmpDir := t.TempDir()
	require.NoError(t, os.Chdir(tmpDir))
	t.Cleanup(func() {
		require.NoError(t, os.Chdir(origDir))
	})

	cmd := &cobra.Command{Use: "test"}
	v := viper.New()

	basePath, stacksBasePath := resolveFromGlobalFlags(cmd, v)

	// Without atmos.yaml, should return defaults.
	assert.Equal(t, ".", basePath)
	assert.Equal(t, "stacks", stacksBasePath)
}

func TestRunInstall_DryRun(t *testing.T) {
	origDir, err := os.Getwd()
	require.NoError(t, err)

	tmpDir := t.TempDir()
	require.NoError(t, os.Chdir(tmpDir))
	t.Cleanup(func() {
		require.NoError(t, os.Chdir(origDir))
		viper.Reset()
	})

	// Use a fresh viper to avoid state leaking.
	v := viper.GetViper()
	v.Set("dry-run", true)
	v.Set("yes", false)
	v.Set("force", false)

	cmd := &cobra.Command{Use: "install"}
	installParser.RegisterFlags(cmd)

	err = runInstall(cmd, nil)
	assert.NoError(t, err)
}

func TestRunInstall_YesFlag(t *testing.T) {
	origDir, err := os.Getwd()
	require.NoError(t, err)

	tmpDir := t.TempDir()
	require.NoError(t, os.Chdir(tmpDir))
	t.Cleanup(func() {
		require.NoError(t, os.Chdir(origDir))
		viper.Reset()
	})

	v := viper.GetViper()
	v.Set("dry-run", false)
	v.Set("yes", true)
	v.Set("force", false)

	cmd := &cobra.Command{Use: "install"}
	installParser.RegisterFlags(cmd)

	err = runInstall(cmd, nil)
	assert.NoError(t, err)

	// Verify files were created.
	assert.FileExists(t, filepath.Join(tmpDir, "atmos.yaml"))
	assert.FileExists(t, filepath.Join(tmpDir, ".github", "workflows", "atmos-pro-terraform-plan.yaml"))
}

func TestRunInstall_YesFlag_ExistingFiles_NoPrompt(t *testing.T) {
	origDir, err := os.Getwd()
	require.NoError(t, err)

	tmpDir := t.TempDir()
	require.NoError(t, os.Chdir(tmpDir))
	t.Cleanup(func() {
		require.NoError(t, os.Chdir(origDir))
		viper.Reset()
	})

	// Pre-create a workflow file to trigger the conflict path.
	wfDir := filepath.Join(tmpDir, ".github", "workflows")
	require.NoError(t, os.MkdirAll(wfDir, 0o755))
	planFile := filepath.Join(wfDir, "atmos-pro-terraform-plan.yaml")
	require.NoError(t, os.WriteFile(planFile, []byte("original"), 0o644))

	v := viper.GetViper()
	v.Set("dry-run", false)
	v.Set("yes", true)
	v.Set("force", false)

	cmd := &cobra.Command{Use: "install"}
	installParser.RegisterFlags(cmd)

	// --yes should succeed without prompts, even with existing files (non-TTY).
	err = runInstall(cmd, nil)
	assert.NoError(t, err)

	// Existing file should be preserved (skipped, not overwritten).
	content, readErr := os.ReadFile(planFile)
	require.NoError(t, readErr)
	assert.Equal(t, "original", string(content))
}

func TestRunInstall_NoConfirmation_NonTTY(t *testing.T) {
	origDir, err := os.Getwd()
	require.NoError(t, err)

	tmpDir := t.TempDir()
	require.NoError(t, os.Chdir(tmpDir))
	t.Cleanup(func() {
		require.NoError(t, os.Chdir(origDir))
		viper.Reset()
	})

	v := viper.GetViper()
	v.Set("dry-run", false)
	v.Set("yes", false)
	v.Set("force", false)

	cmd := &cobra.Command{Use: "install"}
	installParser.RegisterFlags(cmd)

	// In non-TTY mode (tests), confirmation prompt returns an error.
	err = runInstall(cmd, nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "confirmation failed")
}

func TestPromptOpenWorkspace_NonTTY(t *testing.T) {
	// In non-TTY mode (tests), promptOpenWorkspace should return without error.
	require.NotPanics(t, func() {
		promptOpenWorkspace()
	})
}

func TestPromptOverwrite(t *testing.T) {
	// In non-TTY mode (test environment), promptOverwrite returns an error.
	_, err := promptOverwrite("test-file.yaml")
	assert.Error(t, err)
}

func TestWorkspaceURL(t *testing.T) {
	assert.Contains(t, workspaceURL, "atmos-pro.com")
	assert.Contains(t, workspaceURL, "onboarding")
}

func TestEmbeddedMarkdown(t *testing.T) {
	t.Run("install long markdown is loaded", func(t *testing.T) {
		assert.NotEmpty(t, installLongMarkdown)
		assert.Contains(t, installLongMarkdown, "Install Atmos Pro")
	})

	t.Run("next steps markdown is loaded", func(t *testing.T) {
		assert.NotEmpty(t, nextStepsMarkdown)
		assert.Contains(t, nextStepsMarkdown, "Next Steps")
	})
}
