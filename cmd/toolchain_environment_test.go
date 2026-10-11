package cmd

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/dependencies"
	"github.com/cloudposse/atmos/pkg/schema"
)

// TestRootCommandInheritsInstalledProjectTools verifies the shared baseline and scoped overrides under every install policy.
func TestRootCommandInheritsInstalledProjectTools(t *testing.T) {
	for _, policy := range schema.ToolchainInstallValues {
		t.Run(string(policy), func(t *testing.T) {
			_ = NewTestKit(t)
			root := t.TempDir()
			config := &schema.AtmosConfiguration{BasePathAbsolute: root}
			installPath := useNeverInstallToolchain(t, config)
			config.Toolchain.Install = policy
			manifest := "hashicorp/terraform 1.15.9 1.14.0\njqlang/jq 1.7.1\n"
			require.NoError(t, os.WriteFile(filepath.Join(root, ".tool-versions"), []byte(manifest), 0o600))
			require.NoError(t, os.WriteFile(filepath.Join(root, "atmos.yaml"), []byte("{}\n"), 0o600))
			selected := installCommandTool(t, installPath, "hashicorp", "terraform", "1.15.9")
			otherVersion := installCommandTool(t, installPath, "hashicorp", "terraform", "1.14.0")
			unlisted := installCommandTool(t, installPath, "hashicorp", "packer", "1.11.0")
			t.Setenv("ATMOS_CLI_CONFIG_PATH", root)
			t.Setenv("ATMOS_BASE_PATH", root)
			t.Setenv("ATMOS_TOOLCHAIN_INSTALL_PATH", installPath)
			t.Setenv("ATMOS_TOOLCHAIN_INSTALL", string(policy))
			t.Setenv("PATH", os.Getenv("PATH"))
			t.Chdir(root)

			command := &cobra.Command{
				Use: "test-project-tool-path",
				RunE: func(cmd *cobra.Command, args []string) error {
					// This command has no dependency setup of its own.
					binary, err := exec.LookPath("terraform")
					require.NoError(t, err)
					assert.Equal(t, selected, binary)
					entries := filepath.SplitList(os.Getenv("PATH"))
					assert.NotContains(t, entries, filepath.Dir(otherVersion))
					assert.NotContains(t, entries, filepath.Dir(unlisted))
					assert.NoDirExists(t, filepath.Join(installPath, "bin", "jqlang", "jq"))
					// Repeating startup must not duplicate the baseline.
					RootCmd.PersistentPreRun(cmd, args)
					assert.Equal(t, 1, strings.Count(os.Getenv("PATH"), filepath.Dir(selected)))

					// A later scoped dependency must take priority over the baseline.
					config.Toolchain.Install = schema.ToolchainInstallNever
					env, err := dependencies.ForDependencies(config, map[string]string{"terraform": "1.14.0"})
					require.NoError(t, err)
					assert.Equal(t, otherVersion, env.Resolve("terraform"))
					t.Setenv("PATH", env.PrependToPath(os.Getenv("PATH")))
					binary, err = exec.LookPath("terraform")
					require.NoError(t, err)
					assert.Equal(t, otherVersion, binary)
					return nil
				},
			}
			RootCmd.AddCommand(command)
			t.Cleanup(func() { RootCmd.RemoveCommand(command) })
			RootCmd.SetArgs([]string{command.Name()})
			require.NoError(t, RootCmd.Execute())
			contents, err := os.ReadFile(filepath.Join(root, ".tool-versions"))
			require.NoError(t, err)
			assert.Equal(t, manifest, string(contents))
		})
	}
}

// TestRootCommandContinuesWithoutUsableManifest keeps diagnostic commands usable when baseline preparation fails.
func TestRootCommandContinuesWithoutUsableManifest(t *testing.T) {
	for _, kind := range []string{"unreadable", "conflicting"} {
		t.Run(kind, func(t *testing.T) {
			_ = NewTestKit(t)
			root := t.TempDir()
			config := &schema.AtmosConfiguration{BasePathAbsolute: root}
			installPath := useNeverInstallToolchain(t, config)
			require.NoError(t, os.WriteFile(filepath.Join(root, "atmos.yaml"), []byte("{}\n"), 0o600))
			manifest := filepath.Join(root, ".tool-versions")
			if kind == "unreadable" {
				require.NoError(t, os.Mkdir(manifest, 0o755))
			} else {
				require.NoError(t, os.WriteFile(manifest, []byte("terraform 1.15.8\nhashicorp/terraform 1.15.6\n"), 0o600))
			}
			t.Setenv("ATMOS_CLI_CONFIG_PATH", root)
			t.Setenv("ATMOS_BASE_PATH", root)
			t.Setenv("ATMOS_TOOLCHAIN_INSTALL_PATH", installPath)
			t.Setenv("ATMOS_TOOLCHAIN_INSTALL", "never")
			t.Chdir(root)
			originalPath := os.Getenv("PATH")
			ran := false
			command := &cobra.Command{
				Use: "test-diagnostic-tool-path",
				Run: func(_ *cobra.Command, _ []string) {
					ran = true
					assert.Equal(t, originalPath, os.Getenv("PATH"))
				},
			}
			RootCmd.AddCommand(command)
			t.Cleanup(func() { RootCmd.RemoveCommand(command) })
			RootCmd.SetArgs([]string{command.Name()})

			require.NoError(t, RootCmd.Execute())
			assert.True(t, ran, "a manifest error must not prevent diagnostic commands from running")
			assert.NoDirExists(t, installPath, "startup must not provision any tools")
			// Scoped execution still surfaces the error instead of silently falling back.
			_, err := dependencies.ForComponent(config, "terraform", nil, nil)
			require.Error(t, err)
		})
	}
}

// TestInstalledProjectToolsLeavesPathWithoutDefaults verifies missing and unreadable manifests leave inherited PATH intact.
func TestInstalledProjectToolsLeavesPathWithoutDefaults(t *testing.T) {
	for _, unreadable := range []bool{false, true} {
		t.Run(map[bool]string{false: "absent", true: "unreadable"}[unreadable], func(t *testing.T) {
			_ = NewTestKit(t)
			root := t.TempDir()
			config := &schema.AtmosConfiguration{BasePathAbsolute: root}
			useNeverInstallToolchain(t, config)
			original := os.Getenv("PATH")
			if unreadable {
				require.NoError(t, os.Mkdir(filepath.Join(root, ".tool-versions"), 0o755))
			}

			err := applyInstalledProjectTools(config)

			if unreadable {
				require.ErrorContains(t, err, "failed to load .tool-versions")
			} else {
				require.NoError(t, err)
			}
			assert.Equal(t, original, os.Getenv("PATH"))
		})
	}
}

// installCommandTool creates an executable fixture for PATH resolution without downloading a tool.
func installCommandTool(t *testing.T, installPath, owner, repo, version string) string {
	t.Helper()
	dir := filepath.Join(installPath, "bin", owner, repo, version)
	require.NoError(t, os.MkdirAll(dir, 0o755))
	name := repo
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	binary := filepath.Join(dir, name)
	require.NoError(t, os.WriteFile(binary, []byte("fixture"), 0o755))
	return binary
}
