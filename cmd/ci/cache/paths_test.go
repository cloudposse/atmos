package cache

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	cachepkg "github.com/cloudposse/atmos/pkg/ci/cache"
)

// stubResolveCacheConfig stubs the package-level resolveCacheConfig seam so the
// paths command can be exercised without loading real Atmos config.
func stubResolveCacheConfig(t *testing.T, cfg *cachepkg.Config) {
	t.Helper()
	orig := resolveCacheConfig
	t.Cleanup(func() { resolveCacheConfig = orig })
	resolveCacheConfig = func(_ *cobra.Command, _ cacheOverrides) (*cachepkg.Config, error) {
		return cfg, nil
	}
}

func TestCacheResolvedPaths(t *testing.T) {
	root := filepath.FromSlash("/cache/root")

	t.Run("no includes -> whole root", func(t *testing.T) {
		assert.Equal(t, []string{root}, cacheResolvedPaths(&cachepkg.Config{Root: root}))
	})

	t.Run("includes resolved against root", func(t *testing.T) {
		got := cacheResolvedPaths(&cachepkg.Config{Root: root, Includes: []string{"toolchain", filepath.Join("a", "b")}})
		require.Len(t, got, 2)
		assert.Equal(t, filepath.Join(root, "toolchain"), got[0])
		assert.Equal(t, filepath.Join(root, "a", "b"), got[1])
	})
}

func setPathsFormat(t *testing.T, format string) {
	t.Helper()
	require.NoError(t, cachePathsCmd.Flags().Set("format", format))
	t.Cleanup(func() { _ = cachePathsCmd.Flags().Set("format", "github") })
}

func TestRunCachePaths_GitHubWritesOutput(t *testing.T) {
	initTestIO(t)
	out := filepath.Join(t.TempDir(), "gh_output")
	t.Setenv("GITHUB_OUTPUT", out)
	stubResolveCacheConfig(t, &cachepkg.Config{
		Root: filepath.FromSlash("/c/root"), Key: "atmos-k", RestoreKeys: []string{"atmos-"},
	})
	setPathsFormat(t, "github")

	require.NoError(t, runCachePaths(cachePathsCmd, nil))

	b, err := os.ReadFile(out)
	require.NoError(t, err)
	s := string(b)
	assert.Contains(t, s, "key=atmos-k")
	assert.Contains(t, s, "restore-keys=atmos-")
	assert.Contains(t, s, "path=")
}

func TestRunCachePaths_GitHubMultilinePathsHeredoc(t *testing.T) {
	initTestIO(t)
	out := filepath.Join(t.TempDir(), "gh_output")
	t.Setenv("GITHUB_OUTPUT", out)
	stubResolveCacheConfig(t, &cachepkg.Config{
		Root: filepath.FromSlash("/c/root"), Key: "k", Includes: []string{"a", "b"},
	})
	setPathsFormat(t, "github")

	require.NoError(t, runCachePaths(cachePathsCmd, nil))

	b, err := os.ReadFile(out)
	require.NoError(t, err)
	// Two paths → multiline value → heredoc syntax.
	assert.Contains(t, string(b), "path<<")
}

func TestRunCachePaths_OtherFormats(t *testing.T) {
	for _, format := range []string{"json", "yaml", "env"} {
		t.Run(format, func(t *testing.T) {
			initTestIO(t)
			stubResolveCacheConfig(t, &cachepkg.Config{Root: filepath.FromSlash("/r"), Key: "k", RestoreKeys: []string{"k-"}})
			setPathsFormat(t, format)
			require.NoError(t, runCachePaths(cachePathsCmd, nil))
		})
	}
}

func TestRunCachePaths_InvalidFormat(t *testing.T) {
	initTestIO(t)
	stubResolveCacheConfig(t, &cachepkg.Config{Root: filepath.FromSlash("/r"), Key: "k"})
	setPathsFormat(t, "bogus")
	require.ErrorIs(t, runCachePaths(cachePathsCmd, nil), errUtils.ErrInvalidFormat)
}

func TestRunCachePaths_ResolveError(t *testing.T) {
	orig := resolveCacheConfig
	t.Cleanup(func() { resolveCacheConfig = orig })
	resolveCacheConfig = func(_ *cobra.Command, _ cacheOverrides) (*cachepkg.Config, error) {
		return nil, errUtils.ErrCacheUnavailable
	}
	require.ErrorIs(t, runCachePaths(cachePathsCmd, nil), errUtils.ErrCacheUnavailable)
}

// emitCachePaths default branch is covered via TestRunCachePaths_InvalidFormat;
// assert the pure helper directly too for an unknown format.
func TestEmitCachePaths_UnknownFormat(t *testing.T) {
	err := emitCachePaths("nope", &cachepkg.Config{Root: "/r", Key: "k"}, []string{"/r"})
	require.ErrorIs(t, err, errUtils.ErrInvalidFormat)
}
