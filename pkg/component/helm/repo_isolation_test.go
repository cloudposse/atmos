package helm

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// Issue #1: when the user has not set HELM_REPOSITORY_CONFIG / HELM_REPOSITORY_CACHE, atmos must
// isolate Helm's repository config and cache to an atmos-managed location instead of inheriting the
// user's GLOBAL Helm config. Otherwise Helm's chart resolution (scanReposForURL) iterates every repo
// in the user's global repositories.yaml and fails on the first one whose index is not cached, and
// atmos also mutates the user's global Helm config by adding the components' declared repositories.
func TestNewSettings_IsolatesRepositoryConfigWhenEnvUnset(t *testing.T) {
	t.Setenv("HELM_REPOSITORY_CONFIG", "")
	t.Setenv("HELM_REPOSITORY_CACHE", "")
	t.Setenv("ATMOS_XDG_CONFIG_HOME", "")
	t.Setenv("ATMOS_XDG_CACHE_HOME", "")
	cfgHome := t.TempDir()
	cacheHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", cfgHome)
	t.Setenv("XDG_CACHE_HOME", cacheHome)

	s := newSettings()

	require.Equal(t, filepath.Join(cfgHome, "atmos", "helm", "repositories.yaml"), s.RepositoryConfig,
		"repository config must be isolated to the atmos-managed XDG config dir")
	require.Equal(t, filepath.Join(cacheHome, "atmos", "helm", "repository"), s.RepositoryCache,
		"repository cache must be isolated to the atmos-managed XDG cache dir")
}

// When the user explicitly sets the HELM_REPOSITORY_* env vars, atmos must not override them.
func TestNewSettings_RespectsExplicitRepositoryEnv(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "repositories.yaml")
	cachePath := filepath.Join(t.TempDir(), "repository")
	t.Setenv("HELM_REPOSITORY_CONFIG", configPath)
	t.Setenv("HELM_REPOSITORY_CACHE", cachePath)

	s := newSettings()

	require.Equal(t, configPath, s.RepositoryConfig)
	require.Equal(t, cachePath, s.RepositoryCache)
}
