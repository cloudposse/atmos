package hooks

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/toolchain"
)

// TestInstallDepsAddsInstalledManifestTools verifies that hooks put .tool-versions
// tools on the hook PATH only when they are already installed, and never install them.
func TestInstallDepsAddsInstalledManifestTools(t *testing.T) {
	for _, tc := range []struct {
		name      string
		installed bool
	}{
		{"installed manifest tool joins the hook PATH", true},
		{"missing manifest tool is neither installed nor added", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			previous := toolchain.GetAtmosConfig()
			t.Cleanup(func() { toolchain.SetAtmosConfig(previous) })
			config := &schema.AtmosConfiguration{
				BasePath: root, BasePathAbsolute: root,
				Toolchain: schema.Toolchain{InstallPath: filepath.Join(root, "tools")},
			}
			require.NoError(t, os.WriteFile(filepath.Join(root, ".tool-versions"), []byte("jqlang/jq 1.7.1\n"), 0o600))
			binaryDir := filepath.Join(root, "tools", "bin", "jqlang", "jq", "1.7.1")
			if tc.installed {
				require.NoError(t, os.MkdirAll(binaryDir, 0o755))
				name := "jq"
				if runtime.GOOS == "windows" {
					name += ".exe"
				}
				require.NoError(t, os.WriteFile(filepath.Join(binaryDir, name), []byte("fixture"), 0o755))
			}

			h := &Hooks{}
			require.NoError(t, h.installDeps(config, &schema.ConfigAndStacksInfo{}, nil))

			if tc.installed {
				assert.Contains(t, h.toolchainPATH, binaryDir)
				return
			}
			assert.Empty(t, h.toolchainPATH)
			assert.NoDirExists(t, binaryDir, "hooks must not download manifest tools")
		})
	}
}

// hooksToolchainFixture builds a config whose toolchain.install policy is never, so no test in this
// file can reach the network, and returns a helper that installs a stub binary at the toolchain
// layout <install>/bin/<owner>/<repo>/<version>/<bin> and returns its directory.
func hooksToolchainFixture(t *testing.T, manifest string) (*schema.AtmosConfiguration, func(owner, repo, version string) string) {
	t.Helper()

	root := t.TempDir()
	previous := toolchain.GetAtmosConfig()
	t.Cleanup(func() { toolchain.SetAtmosConfig(previous) })
	config := &schema.AtmosConfiguration{
		BasePath: root, BasePathAbsolute: root,
		Toolchain: schema.Toolchain{
			InstallPath: filepath.Join(root, "tools"),
			Install:     schema.ToolchainInstallNever,
		},
	}
	require.NoError(t, os.WriteFile(filepath.Join(root, ".tool-versions"), []byte(manifest), 0o600))

	install := func(owner, repo, version string) string {
		dir := filepath.Join(root, "tools", "bin", owner, repo, version)
		require.NoError(t, os.MkdirAll(dir, 0o755))
		name := repo
		if runtime.GOOS == "windows" {
			name += ".exe"
		}
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte("fixture"), 0o755))
		return dir
	}
	return config, install
}

// TestInstallDepsExplicitDependencies verifies that explicit hook dependencies are pinned onto the
// hook PATH, win over a .tool-versions entry for the same tool, and fail loudly (without
// downloading) when they are not installed under toolchain.install=never.
func TestInstallDepsExplicitDependencies(t *testing.T) {
	info := &schema.ConfigAndStacksInfo{ComponentFromArg: "vpc", Stack: "dev"}

	t.Run("installed explicit dependency joins the hook PATH next to installed manifest tools", func(t *testing.T) {
		config, install := hooksToolchainFixture(t, "jqlang/jq 1.7.1\n")
		jqDir := install("jqlang", "jq", "1.7.1")
		yqDir := install("mikefarah", "yq", "4.40.0")

		h := &Hooks{}
		require.NoError(t, h.installDeps(config, info, map[string]string{"mikefarah/yq": "4.40.0"}))

		assert.Contains(t, h.toolchainPATH, yqDir)
		assert.Contains(t, h.toolchainPATH, jqDir)
	})

	t.Run("explicit dependency overrides the manifest version of the same tool", func(t *testing.T) {
		config, install := hooksToolchainFixture(t, "jqlang/jq 1.7.1\n")
		manifestDir := install("jqlang", "jq", "1.7.1")
		pinnedDir := install("jqlang", "jq", "1.6.0")

		h := &Hooks{}
		require.NoError(t, h.installDeps(config, info, map[string]string{"jqlang/jq": "1.6.0"}))

		assert.Contains(t, h.toolchainPATH, pinnedDir)
		assert.NotContains(t, h.toolchainPATH, manifestDir, "the explicit pin must replace the manifest entry")
	})

	t.Run("missing explicit dependency is an install error and is not downloaded", func(t *testing.T) {
		config, _ := hooksToolchainFixture(t, "")
		missingDir := filepath.Join(config.Toolchain.InstallPath, "bin", "mikefarah", "yq", "4.40.0")

		h := &Hooks{}
		err := h.installDeps(config, info, map[string]string{"mikefarah/yq": "4.40.0"})

		require.ErrorIs(t, err, errUtils.ErrToolInstall)
		require.ErrorIs(t, err, errUtils.ErrToolNotInstalled)
		assert.Empty(t, h.toolchainPATH, "a failed install must not leave a toolchain PATH behind")
		assert.NoDirExists(t, missingDir)
	})
}
