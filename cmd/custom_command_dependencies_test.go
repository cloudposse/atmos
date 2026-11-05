package cmd

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/toolchain"
)

// Compile-time sentinels for the schema fields these tests configure.
var (
	_ = schema.Command{Dependencies: &schema.Dependencies{Tools: map[string]string{}}}
	_ = schema.Toolchain{Install: schema.ToolchainInstallNever}
)

// useNeverInstallToolchain points the toolchain at a temporary install path with
// toolchain.install=never, so a custom command's dependency resolution can never download anything.
func useNeverInstallToolchain(t *testing.T, atmosConfig *schema.AtmosConfiguration) string {
	t.Helper()

	installPath := filepath.Join(t.TempDir(), "tools")
	atmosConfig.Toolchain.InstallPath = installPath
	atmosConfig.Toolchain.Install = schema.ToolchainInstallNever

	previous := toolchain.GetAtmosConfig()
	toolchain.SetAtmosConfig(atmosConfig)
	t.Cleanup(func() { toolchain.SetAtmosConfig(previous) })

	return installPath
}

// TestCustomCommandIntegration_UnresolvableDependencyFailsBeforeSteps verifies that a custom command
// whose dependencies.tools names a tool that is not installed (under toolchain.install=never, where
// nothing may be downloaded) fails fast and never runs its steps, while the same command succeeds,
// runs its steps, and exposes the pinned tool on PATH once the tool is installed.
func TestCustomCommandIntegration_UnresolvableDependencyFailsBeforeSteps(t *testing.T) {
	if testing.Short() {
		t.Skipf("Skipping integration test in short mode")
	}

	const (
		owner   = "mikefarah"
		repo    = "yq"
		version = "4.40.0"
	)
	tools := map[string]string{owner + "/" + repo: version}

	t.Run("missing dependency exits without running steps", func(t *testing.T) {
		_ = NewTestKit(t)
		atmosConfig := loadAuthMockAtmosConfig(t)
		useNeverInstallToolchain(t, &atmosConfig)
		marker := filepath.Join(t.TempDir(), "marker.txt")

		exited := registerAndRunCustomCommand(t, &atmosConfig, &schema.Command{
			Name:         "ep-missing-dependency",
			Dependencies: &schema.Dependencies{Tools: tools},
			Steps:        schema.Tasks{{Type: "shell", Command: customCommandWriteHelperCommand(t, marker, "ran")}},
		})

		assert.True(t, exited, "an uninstalled dependency under toolchain.install=never must fail the command")
		assert.NoFileExists(t, marker, "no step may run when the command's dependencies cannot be resolved")
	})

	t.Run("installed dependency runs steps with the tool on PATH", func(t *testing.T) {
		_ = NewTestKit(t)
		atmosConfig := loadAuthMockAtmosConfig(t)
		installPath := useNeverInstallToolchain(t, &atmosConfig)
		marker := filepath.Join(t.TempDir(), "marker.txt")

		binaryDir := filepath.Join(installPath, "bin", owner, repo, version)
		require.NoError(t, os.MkdirAll(binaryDir, 0o755))
		binaryName := repo
		if runtime.GOOS == "windows" {
			binaryName += ".exe"
		}
		require.NoError(t, os.WriteFile(filepath.Join(binaryDir, binaryName), []byte("fixture"), 0o755))

		// The command rewrites this process's PATH; register the restore first.
		t.Setenv("PATH", os.Getenv("PATH"))

		exited := registerAndRunCustomCommand(t, &atmosConfig, &schema.Command{
			Name:         "ep-installed-dependency",
			Dependencies: &schema.Dependencies{Tools: tools},
			Steps:        schema.Tasks{{Type: "shell", Command: customCommandWriteHelperCommand(t, marker, "ran")}},
		})

		assert.False(t, exited, "an installed dependency must not fail the command")
		assert.FileExists(t, marker, "steps must run once dependencies resolve")
		assert.Contains(t, os.Getenv("PATH"), binaryDir, "the pinned tool's directory must be on PATH for steps")
	})
}
