package toolchain

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/toolchain/lockfile"
)

func TestAutomaticInstallPreservesDeclarations(t *testing.T) {
	setupTestIO(t)
	for _, existing := range []bool{false, true} {
		t.Run(map[bool]string{false: "no manifest", true: "existing manifest"}[existing], func(t *testing.T) {
			t.Chdir(t.TempDir())
			t.Setenv("ATMOS_XDG_CACHE_HOME", t.TempDir())
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, _ = w.Write([]byte("test executable"))
			}))
			defer server.Close()
			project := t.TempDir()
			manifest := filepath.Join(project, ".tool-versions")
			content := []byte("# Keep this comment and pin unchanged.\nowner/tool 0.9.0\n")
			if existing {
				require.NoError(t, os.WriteFile(manifest, content, 0o644))
			}
			previous := GetAtmosConfig()
			t.Cleanup(func() { SetAtmosConfig(previous) })
			config := &schema.AtmosConfiguration{
				BasePathAbsolute: project,
				Toolchain: schema.Toolchain{
					VersionsFile: ".tool-versions", InstallPath: ".tools", LockFile: "toolchain.lock.yaml", UseLockFile: true,
					Registries: []schema.ToolchainRegistry{{Name: "local", Type: "atmos", Tools: map[string]any{
						"owner/tool": map[string]any{"type": "http", "url": server.URL + "/tool", "format": "raw"},
					}}},
				},
			}
			SetAtmosConfig(config)
			specs := []string{"owner/tool@1.0.0"}
			if existing {
				specs = append(specs, "owner/tool@2.0.0")
			}
			require.NoError(t, RunAutomaticInstallBatch(specs, false))
			binary, err := findBinaryPath("owner/tool@1.0.0")
			require.NoError(t, err)
			require.FileExists(t, binary)
			lf, err := lockfile.Load(filepath.Join(project, "toolchain.lock.yaml"))
			require.NoError(t, err)
			require.NotEmpty(t, lf.Tools["owner/tool"].Versions["1.0.0"].Platforms[runtime.GOOS+"_"+runtime.GOARCH].Checksum)
			if existing {
				require.NotEmpty(t, lf.Tools["owner/tool"].Versions["2.0.0"].Platforms[runtime.GOOS+"_"+runtime.GOARCH].Checksum)
				after, err := os.ReadFile(manifest)
				require.NoError(t, err)
				require.Equal(t, string(content), string(after))
			} else {
				require.NoFileExists(t, manifest)
			}
			require.NoFileExists(t, manifest+".lock")
			cwd, err := os.Getwd()
			require.NoError(t, err)
			entries, err := os.ReadDir(cwd)
			require.NoError(t, err)
			require.Empty(t, entries, "automatic installs must not write into the invoking directory")
			config.Toolchain.FrozenLockFile = true
			require.ErrorIs(t, RunLock(nil, LockOptions{MaxConcurrency: 1}), errUtils.ErrFrozenLockfile)
		})
	}
}

func TestReadMissingToolVersionsDoesNotCreateFiles(t *testing.T) {
	root := t.TempDir()
	_, err := LoadToolVersions(filepath.Join(root, "missing", ".tool-versions"))
	require.ErrorIs(t, err, os.ErrNotExist)
	entries, err := os.ReadDir(root)
	require.NoError(t, err)
	require.Empty(t, entries)
}

func TestDefaultToolVersionsFromProjectSubdirectory(t *testing.T) {
	project := t.TempDir()
	component := filepath.Join(project, "components", "example")
	require.NoError(t, os.MkdirAll(component, 0o755))
	t.Chdir(component)
	manifest := filepath.Join(project, ".tool-versions")
	require.NoError(t, os.WriteFile(manifest, []byte("owner/tool 1.2.3\n"), 0o644))
	previous := GetAtmosConfig()
	t.Cleanup(func() { SetAtmosConfig(previous) })
	config := &schema.AtmosConfiguration{BasePathAbsolute: project}
	SetAtmosConfig(config)

	// Omitting versions_file must resolve the same project declaration for
	// direct toolchain commands and the environment inherited by proxies.
	require.Equal(t, manifest, GetToolVersionsFilePath())
	require.Equal(t, manifest, resolveVersionsFilePath(config))
	versions, err := LoadToolVersions(GetToolVersionsFilePath())
	require.NoError(t, err)
	require.Equal(t, []string{"1.2.3"}, versions.Tools["owner/tool"])
	entries, err := os.ReadDir(component)
	require.NoError(t, err)
	require.Empty(t, entries)

	// Without a project base, preserve the standalone CWD-relative default.
	SetAtmosConfig(nil)
	require.Equal(t, DefaultToolVersionsFilePath, GetToolVersionsFilePath())
}
