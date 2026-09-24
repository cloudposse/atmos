package version

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/toolchain"
	"github.com/cloudposse/atmos/pkg/toolchain/lockfile"
	"github.com/cloudposse/atmos/pkg/xdg"
)

func TestBootstrapConfiguration(t *testing.T) {
	t.Setenv("ATMOS_XDG_CACHE_HOME", t.TempDir())
	t.Chdir(t.TempDir())
	project := &schema.AtmosConfiguration{CliConfigPath: t.TempDir(), Toolchain: schema.Toolchain{InstallPath: ".tools", LockFile: "project.lock.yaml", FrozenLockFile: true}}
	got, err := bootstrapConfiguration(project)
	require.NoError(t, err)
	require.Equal(t, project, got)
	got.Toolchain.LockFile = "changed"
	require.Equal(t, "project.lock.yaml", project.Toolchain.LockFile)

	for _, config := range []*schema.AtmosConfiguration{nil, {Toolchain: schema.Toolchain{InstallPath: ".tools", LockFile: "local.lock.yaml"}}} {
		got, err := bootstrapConfiguration(config)
		require.NoError(t, err)
		cache, err := xdg.GetXDGCacheDir("toolchain", 0o755)
		require.NoError(t, err)
		require.Equal(t, cache, got.Toolchain.InstallPath)
		require.Equal(t, filepath.Join(cache, "toolchain.lock.yaml"), got.Toolchain.LockFile)
		require.True(t, got.Toolchain.UseLockFile)
	}
	cwd, err := os.Getwd()
	require.NoError(t, err)
	entries, err := os.ReadDir(cwd)
	require.NoError(t, err)
	require.Empty(t, entries)
}

func TestBootstrapConfigurationDoesNotFallBackToCWD(t *testing.T) {
	blocked := filepath.Join(t.TempDir(), "not-a-directory")
	require.NoError(t, os.WriteFile(blocked, []byte("blocked"), 0o644))
	t.Setenv("ATMOS_XDG_CACHE_HOME", blocked)
	_, err := bootstrapConfiguration(nil)
	require.Error(t, err)
}

func TestFrozenBootstrapRejectsDevelopmentArtifacts(t *testing.T) {
	for _, version := range []string{"pr:123", "sha:abcdef1", "ref:main"} {
		_, err := findOrInstallVersionWithConfig(version, &ReexecConfig{FrozenLockFile: true})
		require.ErrorIs(t, err, errUtils.ErrFrozenLockfile)
	}
}

func TestBootstrapInstallUsesLockWithoutCreatingManifest(t *testing.T) {
	for _, project := range []bool{false, true} {
		t.Run(map[bool]string{false: "XDG", true: "project"}[project], func(t *testing.T) {
			t.Chdir(t.TempDir())
			t.Setenv("ATMOS_XDG_CACHE_HOME", t.TempDir())
			config := &schema.AtmosConfiguration{Toolchain: schema.Toolchain{UseLockFile: true}}
			if project {
				config.CliConfigPath = t.TempDir()
				config.BasePathAbsolute = config.CliConfigPath
				config.Toolchain.InstallPath = ".tools"
				config.Toolchain.LockFile = "toolchain.lock.yaml"
			}
			bootstrap, err := bootstrapConfiguration(config)
			require.NoError(t, err)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, _ = w.Write([]byte("test Atmos executable"))
			}))
			defer server.Close()
			bootstrap.Toolchain.Registries = []schema.ToolchainRegistry{{Name: "test", Type: "atmos", Tools: map[string]any{
				"cloudposse/atmos": map[string]any{"type": "http", "url": server.URL + "/atmos", "format": "raw"},
			}}}
			previous := toolchain.GetAtmosConfig()
			t.Cleanup(func() { toolchain.SetAtmosConfig(previous) })
			toolchain.SetAtmosConfig(bootstrap)
			require.NoError(t, (&defaultInstaller{}).Install("atmos@1.2.3", false, false))
			lockPath := bootstrap.Toolchain.LockFile
			if project {
				lockPath = filepath.Join(config.BasePathAbsolute, lockPath)
				require.NoFileExists(t, filepath.Join(config.BasePathAbsolute, ".tool-versions"))
			}
			lf, err := lockfile.Load(lockPath)
			require.NoError(t, err)
			require.Contains(t, lf.Tools["cloudposse/atmos"].Versions, "1.2.3")
			cwd, err := os.Getwd()
			require.NoError(t, err)
			files, err := os.ReadDir(cwd)
			require.NoError(t, err)
			require.Empty(t, files)
		})
	}
}
