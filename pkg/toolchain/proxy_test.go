package toolchain

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

func TestPrepareProxyEnvironmentCreatesLinkAndContext(t *testing.T) {
	tempDir := t.TempDir()
	target := filepath.Join(tempDir, proxyFilename("atmos"))
	require.NoError(t, os.WriteFile(target, []byte("atmos"), 0o755))
	originalProxyExecutable := proxyExecutable
	proxyExecutable = func() (string, error) { return target, nil }
	t.Cleanup(func() { proxyExecutable = originalProxyExecutable })

	versionsFile := filepath.Join(tempDir, ".tool-versions")
	require.NoError(t, os.WriteFile(versionsFile, []byte("coreutils 0.9.0\n"), 0o644))
	config := &schema.AtmosConfiguration{
		CliConfigPath: tempDir,
		Toolchain: schema.Toolchain{
			InstallPath:  filepath.Join(tempDir, "tools"),
			VersionsFile: versionsFile,
			Proxies: map[string]schema.ToolchainProxy{
				"ls": {Tool: "coreutils", Args: []string{"ls"}},
			},
		},
	}

	env, err := PrepareProxyEnvironment(config)
	require.NoError(t, err)
	require.Equal(t, filepath.Join(tempDir, "tools", "bin", "proxy"), env.Path)
	require.Equal(t, tempDir, env.ConfigPath)
	require.Equal(t, versionsFile, env.VersionsFile)

	link := filepath.Join(env.Path, proxyFilename("ls"))
	info, err := os.Lstat(link)
	require.NoError(t, err)
	if runtime.GOOS == "windows" {
		targetInfo, err := os.Stat(target)
		require.NoError(t, err)
		require.True(t, os.SameFile(info, targetInfo))
	} else {
		require.NotZero(t, info.Mode()&os.ModeSymlink)
	}
	require.Equal(t, versionsFile, env.Variables()[ProxyVersionsFileEnv])
}

func TestSyncProxiesRejectsUnsafeAndUnmanagedNames(t *testing.T) {
	tempDir := t.TempDir()
	config := &schema.AtmosConfiguration{Toolchain: schema.Toolchain{
		InstallPath: filepath.Join(tempDir, "tools"),
		Proxies: map[string]schema.ToolchainProxy{
			"../ls": {Tool: "coreutils"},
		},
	}}
	require.Error(t, SyncProxies(config))

	config.Toolchain.Proxies = map[string]schema.ToolchainProxy{"ls": {Tool: "coreutils"}}
	proxyPath := filepath.Join(ProxyDir(config), proxyFilename("ls"))
	require.NoError(t, os.MkdirAll(filepath.Dir(proxyPath), 0o755))
	require.NoError(t, os.WriteFile(proxyPath, []byte("user-managed"), 0o644))
	require.Error(t, SyncProxies(config))
}

func TestSyncProxiesRefreshesManagedLinkAfterExecutableReplacement(t *testing.T) {
	tempDir := t.TempDir()
	firstTarget := filepath.Join(tempDir, "atmos-first")
	secondTarget := filepath.Join(tempDir, "atmos-second")
	require.NoError(t, os.WriteFile(firstTarget, []byte("first"), 0o755))
	require.NoError(t, os.WriteFile(secondTarget, []byte("second"), 0o755))

	currentTarget := firstTarget
	originalProxyExecutable := proxyExecutable
	proxyExecutable = func() (string, error) { return currentTarget, nil }
	t.Cleanup(func() { proxyExecutable = originalProxyExecutable })

	config := &schema.AtmosConfiguration{Toolchain: schema.Toolchain{
		InstallPath: filepath.Join(tempDir, "tools"),
		Proxies: map[string]schema.ToolchainProxy{
			"ls": {Tool: "coreutils"},
		},
	}}
	require.NoError(t, SyncProxies(config))

	currentTarget = secondTarget
	require.NoError(t, SyncProxies(config))

	link := filepath.Join(ProxyDir(config), proxyFilename("ls"))
	info, err := os.Lstat(link)
	require.NoError(t, err)
	pointsToCurrent, err := proxyTargetsExecutable(info, link, secondTarget)
	require.NoError(t, err)
	require.True(t, pointsToCurrent)
	_, err = os.Stat(proxyMetadataPath(ProxyDir(config)))
	require.NoError(t, err)
}

func TestSyncProxiesRejectsCaseInsensitiveReservedName(t *testing.T) {
	config := &schema.AtmosConfiguration{Toolchain: schema.Toolchain{
		InstallPath: t.TempDir(),
		Proxies: map[string]schema.ToolchainProxy{
			"Atmos": {Tool: "coreutils"},
		},
	}}
	require.ErrorContains(t, SyncProxies(config), "reserved")
}

func TestRunProxyRejectsNilConfig(t *testing.T) {
	require.ErrorContains(t, RunProxy(nil, "ls", nil), "requires configuration")
}

func TestProxyExportsAreStableAndShellSafe(t *testing.T) {
	env := ProxyEnvironment{
		Path:         "/tools/bin/proxy",
		ConfigPath:   "/project",
		VersionsFile: "/project/.tool-versions",
		InstallPath:  "/tools",
	}
	exports := formatProxyExports("bash", env)
	require.Equal(t, "export ATMOS_TOOLCHAIN_PROXY_CONFIG_PATH='/project'\n"+
		"export ATMOS_TOOLCHAIN_PROXY_VERSIONS_FILE='/project/.tool-versions'\n"+
		"export ATMOS_TOOLCHAIN_PROXY_INSTALL_PATH='/tools'\n", exports)
}
