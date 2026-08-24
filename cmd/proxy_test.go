package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/toolchain"
)

func TestProxyNameFromArgv(t *testing.T) {
	tests := []struct {
		name     string
		argv     []string
		wantName string
		wantOK   bool
	}{
		{
			name:     "empty argv",
			argv:     nil,
			wantName: "",
			wantOK:   false,
		},
		{
			name:     "atmos executable itself",
			argv:     []string{"/usr/local/bin/atmos"},
			wantName: "",
			wantOK:   false,
		},
		{
			name:     "proxy invocation",
			argv:     []string{"/home/user/.atmos/proxies/terraform"},
			wantName: "terraform",
			wantOK:   true,
		},
		{
			name:     "bare name with no path separators",
			argv:     []string{"kubectl"},
			wantName: "kubectl",
			wantOK:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			name, ok := proxyNameFromArgv(tt.argv)
			assert.Equal(t, tt.wantName, name)
			assert.Equal(t, tt.wantOK, ok)
		})
	}
}

func TestProxyConfigContextDetected(t *testing.T) {
	t.Run("unset env var", func(t *testing.T) {
		t.Setenv(toolchain.ProxyConfigPathEnv, "")
		assert.False(t, proxyConfigContextDetected())
	})

	t.Run("set to a path", func(t *testing.T) {
		t.Setenv(toolchain.ProxyConfigPathEnv, "/tmp/atmos.yaml")
		assert.True(t, proxyConfigContextDetected())
	})
}

func TestApplyProxyEnvOverrides(t *testing.T) {
	t.Run("no overrides set leaves config untouched", func(t *testing.T) {
		t.Setenv(toolchain.ProxyVersionsFileEnv, "")
		t.Setenv(toolchain.ProxyInstallPathEnv, "")

		atmosConfig := &schema.AtmosConfiguration{}
		atmosConfig.Toolchain.VersionsFile = "original-versions"
		atmosConfig.Toolchain.InstallPath = "original-install"

		applyProxyEnvOverrides(atmosConfig)

		assert.Equal(t, "original-versions", atmosConfig.Toolchain.VersionsFile)
		assert.Equal(t, "original-install", atmosConfig.Toolchain.InstallPath)
	})

	t.Run("overrides applied from environment", func(t *testing.T) {
		t.Setenv(toolchain.ProxyVersionsFileEnv, "/parent/.tool-versions")
		t.Setenv(toolchain.ProxyInstallPathEnv, "/parent/.atmos/toolchain")

		atmosConfig := &schema.AtmosConfiguration{}
		applyProxyEnvOverrides(atmosConfig)

		assert.Equal(t, "/parent/.tool-versions", atmosConfig.Toolchain.VersionsFile)
		assert.Equal(t, "/parent/.atmos/toolchain", atmosConfig.Toolchain.InstallPath)
	})
}

func TestProxyConfigSelection(t *testing.T) {
	t.Run("uses ProxyConfigPathEnv when set", func(t *testing.T) {
		t.Setenv(toolchain.ProxyConfigPathEnv, "/parent/atmos.yaml")

		info := proxyConfigSelection()
		assert.Equal(t, []string{"/parent/atmos.yaml"}, info.AtmosConfigDirsFromArg)
	})

	t.Run("falls back to early config discovery when unset", func(t *testing.T) {
		t.Setenv(toolchain.ProxyConfigPathEnv, "")

		info := proxyConfigSelection()
		// Falls through to the same early config discovery cmd/root.go uses,
		// which never sets AtmosConfigDirsFromArg from a bare proxy env var.
		assert.Equal(t, cfg.EarlyConfigAndStacksInfoFromArgs(nil).AtmosConfigDirsFromArg, info.AtmosConfigDirsFromArg)
	})
}

// TestTryRunToolchainProxy_NotAProxyInvocation covers the early-return path:
// argv[0] resolving to the atmos binary itself means this isn't a proxy
// invocation, so TryRunToolchainProxy must return false without attempting
// to load any configuration.
func TestTryRunToolchainProxy_NotAProxyInvocation(t *testing.T) {
	ran, err := TryRunToolchainProxy([]string{"/usr/local/bin/atmos", "terraform", "plan"})
	assert.False(t, ran)
	assert.NoError(t, err)
}

// TestTryRunToolchainProxy_EmptyArgv covers the len(argv) == 0 branch inside
// proxyNameFromArgv as reached through the public entrypoint.
func TestTryRunToolchainProxy_EmptyArgv(t *testing.T) {
	ran, err := TryRunToolchainProxy(nil)
	assert.False(t, ran)
	assert.NoError(t, err)
}

// TestTryRunToolchainProxy_NoMatchingProxyConfigured covers the path where a
// config loads successfully but the invoked name has no registered proxy:
// TryRunToolchainProxy must return false rather than attempting to run it.
func TestTryRunToolchainProxy_NoMatchingProxyConfigured(t *testing.T) {
	tempDir := t.TempDir()
	atmosYAML := `toolchain:
  proxies:
    terraform:
      tool: terraform
`
	require.NoError(t, os.WriteFile(filepath.Join(tempDir, "atmos.yaml"), []byte(atmosYAML), 0o644))
	t.Setenv(toolchain.ProxyConfigPathEnv, tempDir)

	ran, err := TryRunToolchainProxy([]string{"/home/user/.atmos/proxies/some-unconfigured-tool"})
	assert.False(t, ran)
	assert.NoError(t, err)
}

// TestTryRunToolchainProxy_ConfigErrorInProxyContext covers the branch where
// config loading fails while ProxyConfigPathEnv is set: since that env var is
// only ever set by ApplyProxyEnvironment for a genuine proxy child process,
// TryRunToolchainProxy must report the invocation as a proxy call (true) and
// surface the underlying config error, rather than silently falling through
// to an unrelated Atmos subcommand error.
func TestTryRunToolchainProxy_ConfigErrorInProxyContext(t *testing.T) {
	tempDir := t.TempDir() // No atmos.yaml here, so config loading fails.
	t.Setenv(toolchain.ProxyConfigPathEnv, tempDir)

	ran, err := TryRunToolchainProxy([]string{"/home/user/.atmos/proxies/terraform"})
	assert.True(t, ran)
	require.Error(t, err)
}
