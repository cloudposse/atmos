package cache

import (
	"bytes"
	stdio "io"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	cachepkg "github.com/cloudposse/atmos/pkg/ci/cache"
	"github.com/cloudposse/atmos/pkg/data"
	iolib "github.com/cloudposse/atmos/pkg/io"
)

// pathsTestStreams is a minimal io.Streams implementation for capturing
// data output in tests, mirroring cmd/stack's stackConfigTestStreams.
type pathsTestStreams struct {
	stdin  stdio.Reader
	stdout *bytes.Buffer
	stderr *bytes.Buffer
}

func (ts *pathsTestStreams) Input() stdio.Reader     { return ts.stdin }
func (ts *pathsTestStreams) Output() stdio.Writer    { return ts.stdout }
func (ts *pathsTestStreams) Error() stdio.Writer     { return ts.stderr }
func (ts *pathsTestStreams) RawOutput() stdio.Writer { return ts.stdout }
func (ts *pathsTestStreams) RawError() stdio.Writer  { return ts.stderr }

// capturePathsTestWriter wires a fresh data writer that captures stdout, and
// returns a func reading back everything written so far.
func capturePathsTestWriter(t *testing.T) func() string {
	t.Helper()

	streams := &pathsTestStreams{stdin: &bytes.Buffer{}, stdout: &bytes.Buffer{}, stderr: &bytes.Buffer{}}
	ioCtx, err := iolib.NewContext(iolib.WithStreams(streams))
	require.NoError(t, err)
	data.InitWriter(ioCtx)
	t.Cleanup(data.Reset)
	return streams.stdout.String
}

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

func TestCacheResolvedExcludes(t *testing.T) {
	root := filepath.FromSlash("/cache/root")

	t.Run("default excludes are resolved against root", func(t *testing.T) {
		got := cacheResolvedExcludes(&cachepkg.Config{Root: root})
		require.Len(t, got, len(cachepkg.DefaultExcludedPaths()))
		for i, ex := range cachepkg.DefaultExcludedPaths() {
			assert.Equal(t, filepath.Join(root, ex), got[i])
		}
	})

	t.Run("allow unsafe auth cache disables excludes", func(t *testing.T) {
		got := cacheResolvedExcludes(&cachepkg.Config{Root: root, AllowUnsafeAuthCache: true})
		assert.Empty(t, got)
	})
}

func TestGithubGlobLines_PairsIncludeExcludeAtSameDepth(t *testing.T) {
	got := githubGlobLines([]string{filepath.FromSlash("/r")}, []string{filepath.FromSlash("/r/aws-sso")})
	assert.Equal(t, []string{"/r/**", "!/r/aws-sso/**"}, got)
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
	// The default excludes make "path" multiline (root + 4 exclusion lines),
	// so it's emitted as a heredoc rather than a bare "path=" assignment.
	assert.Contains(t, s, "path<<")
}

func TestRunCachePaths_GitHubOutput_ExcludesAuthCaches(t *testing.T) {
	initTestIO(t)
	root := filepath.FromSlash("/c/root")
	out := filepath.Join(t.TempDir(), "gh_output")
	t.Setenv("GITHUB_OUTPUT", out)
	stubResolveCacheConfig(t, &cachepkg.Config{
		Root: root, Key: "atmos-k", RestoreKeys: []string{"atmos-"},
	})
	setPathsFormat(t, "github")

	require.NoError(t, runCachePaths(cachePathsCmd, nil))

	b, err := os.ReadFile(out)
	require.NoError(t, err)
	s := string(b)
	assert.Contains(t, s, filepath.ToSlash(filepath.Join(root, "**")))
	for _, ex := range cachepkg.DefaultExcludedPaths() {
		assert.Contains(t, s, "!"+filepath.ToSlash(filepath.Join(root, ex, "**")))
	}
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

func TestRunCachePaths_JSON_ExcludePathsField(t *testing.T) {
	root := filepath.FromSlash("/r")
	out := capturePathsTestWriter(t)
	stubResolveCacheConfig(t, &cachepkg.Config{Root: root, Key: "k", RestoreKeys: []string{"k-"}})
	setPathsFormat(t, "json")
	require.NoError(t, runCachePaths(cachePathsCmd, nil))

	s := out()
	assert.Contains(t, s, "exclude_paths")
	for _, ex := range cachepkg.DefaultExcludedPaths() {
		assert.Contains(t, s, filepath.ToSlash(filepath.Join(root, ex)))
	}
}

func TestRunCachePaths_YAML_ExcludePathsField(t *testing.T) {
	root := filepath.FromSlash("/r")
	out := capturePathsTestWriter(t)
	stubResolveCacheConfig(t, &cachepkg.Config{Root: root, Key: "k", RestoreKeys: []string{"k-"}})
	setPathsFormat(t, "yaml")
	require.NoError(t, runCachePaths(cachePathsCmd, nil))

	s := out()
	assert.Contains(t, s, "exclude_paths")
	for _, ex := range cachepkg.DefaultExcludedPaths() {
		assert.Contains(t, s, filepath.ToSlash(filepath.Join(root, ex)))
	}
}

// TestRunCachePaths_Env_ExcludePaths captures real os.Stdout via a pipe
// because the "env" format writes with fmt.Print directly rather than
// through the pkg/data writer context (see env.Output).
func TestRunCachePaths_Env_ExcludePaths(t *testing.T) {
	initTestIO(t)
	root := filepath.FromSlash("/r")
	stubResolveCacheConfig(t, &cachepkg.Config{Root: root, Key: "k", RestoreKeys: []string{"k-"}})
	setPathsFormat(t, "env")

	oldStdout := os.Stdout
	r, w, err := os.Pipe()
	require.NoError(t, err)
	os.Stdout = w
	t.Cleanup(func() { os.Stdout = oldStdout })

	require.NoError(t, runCachePaths(cachePathsCmd, nil))
	require.NoError(t, w.Close())
	os.Stdout = oldStdout

	var buf bytes.Buffer
	_, err = stdio.Copy(&buf, r)
	require.NoError(t, err)
	s := buf.String()

	assert.Contains(t, s, "ATMOS_CI_CACHE_EXCLUDE_PATHS=")
	for _, ex := range cachepkg.DefaultExcludedPaths() {
		assert.Contains(t, s, filepath.Join(root, ex))
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
	err := emitCachePaths("nope", &cachepkg.Config{Root: "/r", Key: "k"}, []string{"/r"}, nil)
	require.ErrorIs(t, err, errUtils.ErrInvalidFormat)
}
