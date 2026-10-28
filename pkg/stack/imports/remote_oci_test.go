package imports

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/cache"
	"github.com/cloudposse/atmos/pkg/downloader"
	"github.com/cloudposse/atmos/pkg/oci/ocitest"
	"github.com/cloudposse/atmos/pkg/schema"
)

// ociStackFiles is the content of the test OCI artifact, keyed by path inside the image.
var ociStackFiles = map[string]string{
	"stacks/catalog/vpc.yaml":          "vars:\n  cidr: 10.0.0.0/16\n",
	"stacks/catalog/nested/dns.yaml":   "vars:\n  zone: example.com\n",
	"stacks/catalog/template.yml.tmpl": "vars:\n  template: true\n",
	"stacks/orgs/acme/_defaults.yaml":  "import:\n  - catalog/_defaults\nvars:\n  org: acme\n",
	"stacks/catalog/_defaults.yaml":    "vars:\n  catalog: true\n",
	"stacks/catalog/ignored.txt":       "ignored\n",
}

// readMatch returns the content of the cached file a match points at.
func readMatch(t *testing.T, match RemoteImportMatch) string {
	t.Helper()

	content, err := os.ReadFile(match.Path)
	require.NoError(t, err)
	return string(content)
}

// TestRemoteImporter_Resolve_OCI_File reproduces the bug where an oci:// import failed: the
// remote importer fetched non-Git sources in single-file mode, which the OCI getter rejects
// (an OCI artifact is only ever extracted as a directory).
func TestRemoteImporter_Resolve_OCI_File(t *testing.T) {
	ref := ocitest.NewRegistry(t, "stacks:v1", ociStackFiles)
	importer := newTestRemoteImporter(t, &schema.AtmosConfiguration{})

	uri := "oci://" + ref + "//stacks/catalog/vpc.yaml"
	matches, err := importer.Resolve(uri)
	require.NoError(t, err)
	require.Len(t, matches, 1)

	assert.Equal(t, uri+"#stacks/catalog/vpc.yaml", matches[0].Key)
	assert.Equal(t, "vars:\n  cidr: 10.0.0.0/16\n", readMatch(t, matches[0]))
}

func TestRemoteImporter_Resolve_OCI_Selection(t *testing.T) {
	ref := ocitest.NewRegistry(t, "stacks:v1", ociStackFiles)

	tests := []struct {
		name         string
		subdir       string
		expectedKeys []string
	}{
		{
			name:         "extensionless file resolves the yaml extension",
			subdir:       "stacks/catalog/vpc",
			expectedKeys: []string{"stacks/catalog/vpc.yaml"},
		},
		{
			name:   "directory resolves recursively in sorted order and skips non-stack files",
			subdir: "stacks/catalog",
			expectedKeys: []string{
				"stacks/catalog/_defaults.yaml",
				"stacks/catalog/nested/dns.yaml",
				"stacks/catalog/template.yml.tmpl",
				"stacks/catalog/vpc.yaml",
			},
		},
		{
			name:   "glob selects matching files",
			subdir: "stacks/catalog/**/*.yaml",
			expectedKeys: []string{
				"stacks/catalog/_defaults.yaml",
				"stacks/catalog/nested/dns.yaml",
				"stacks/catalog/vpc.yaml",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			importer := newTestRemoteImporter(t, &schema.AtmosConfiguration{})
			uri := "oci://" + ref + "//" + tt.subdir

			matches, err := importer.Resolve(uri)
			require.NoError(t, err)
			require.Len(t, matches, len(tt.expectedKeys))

			for i, key := range tt.expectedKeys {
				assert.Equal(t, uri+"#"+key, matches[i].Key)
				assert.Equal(t, ociStackFiles[key], readMatch(t, matches[i]))
			}
		})
	}
}

func TestRemoteImporter_Resolve_OCI_Errors(t *testing.T) {
	ref := ocitest.NewRegistry(t, "stacks:v1", ociStackFiles)

	t.Run("missing path to the file", func(t *testing.T) {
		importer := newTestRemoteImporter(t, &schema.AtmosConfiguration{})

		matches, err := importer.Resolve("oci://" + ref)
		require.Error(t, err)
		assert.Nil(t, matches)
		assert.ErrorIs(t, err, errUtils.ErrOCIImportMissingPath)
	})

	t.Run("path traversal out of the artifact", func(t *testing.T) {
		importer := newTestRemoteImporter(t, &schema.AtmosConfiguration{})

		matches, err := importer.Resolve("oci://" + ref + "//../outside.yaml")
		require.Error(t, err)
		assert.Nil(t, matches)
		assert.ErrorIs(t, err, errRemoteSubdirTraversal)
	})

	t.Run("path not present in the artifact", func(t *testing.T) {
		importer := newTestRemoteImporter(t, &schema.AtmosConfiguration{})

		matches, err := importer.Resolve("oci://" + ref + "//stacks/missing.yaml")
		require.Error(t, err)
		assert.Nil(t, matches)
		assert.ErrorIs(t, err, errUtils.ErrFailedToFindImport)
	})

	t.Run("registry fetch failure is returned", func(t *testing.T) {
		importer := newTestRemoteImporter(t, &schema.AtmosConfiguration{})

		// Nothing listens on port 1, so the pull fails.
		matches, err := importer.Resolve("oci://127.0.0.1:1/stacks:v1//stacks/catalog/vpc.yaml")
		require.Error(t, err)
		assert.Nil(t, matches)
	})
}

// TestRemoteImporter_Resolve_OCI_DotDirectory verifies that a hidden directory (such as an
// atmos.d config directory) can be imported whole, as `oci://host/repo:tag//.atmos.d`.
func TestRemoteImporter_Resolve_OCI_DotDirectory(t *testing.T) {
	files := map[string]string{
		".atmos.d/ai.yaml":        "ai:\n  enabled: true\n",
		".atmos.d/nested/mcp.yml": "mcp:\n  enabled: true\n",
		".atmos.d/notes.txt":      "ignored\n",
		"unrelated/other.yaml":    "other: true\n",
	}
	ref := ocitest.NewRegistry(t, "shared-config:v1.2.3", files)
	importer := newTestRemoteImporter(t, &schema.AtmosConfiguration{})

	uri := "oci://" + ref + "//.atmos.d"
	matches, err := importer.Resolve(uri)
	require.NoError(t, err)
	require.Len(t, matches, 2)

	assert.Equal(t, uri+"#.atmos.d/ai.yaml", matches[0].Key)
	assert.Equal(t, files[".atmos.d/ai.yaml"], readMatch(t, matches[0]))
	assert.Equal(t, uri+"#.atmos.d/nested/mcp.yml", matches[1].Key)
	assert.Equal(t, files[".atmos.d/nested/mcp.yml"], readMatch(t, matches[1]))

	// The cached copies keep their own extensions, which config loading relies on to detect the file type.
	assert.Equal(t, ".yaml", filepath.Ext(matches[0].Path))
	assert.Equal(t, ".yml", filepath.Ext(matches[1].Path))
}

// TestRemoteImporter_Resolve_OCI_UsesXDGCache verifies that, with no cache override, an OCI import is
// pulled into and served from the XDG cache directory like every other remote import.
func TestRemoteImporter_Resolve_OCI_UsesXDGCache(t *testing.T) {
	xdgCache := t.TempDir()
	t.Setenv("ATMOS_XDG_CACHE_HOME", xdgCache)
	t.Setenv("XDG_CACHE_HOME", xdgCache)

	ref := ocitest.NewRegistry(t, "stacks:v1", ociStackFiles)
	importer, err := NewRemoteImporter(&schema.AtmosConfiguration{})
	require.NoError(t, err)

	matches, err := importer.Resolve("oci://" + ref + "//stacks/catalog/vpc.yaml")
	require.NoError(t, err)
	require.Len(t, matches, 1)

	cacheDir := importer.cache.BaseDir()
	assert.True(t, strings.HasPrefix(cacheDir, xdgCache), "cache dir %q must be under the XDG cache %q", cacheDir, xdgCache)
	assert.True(t, strings.HasPrefix(matches[0].Path, cacheDir), "file %q must be cached under %q", matches[0].Path, cacheDir)

	// The extracted artifact is persisted in the cache next to the per-file entries.
	sources, err := filepath.Glob(filepath.Join(cacheDir, "*.source"))
	require.NoError(t, err)
	assert.Len(t, sources, 1)
}

// ociMockFetch returns a downloader mock whose Fetch of the artifact writes files into the
// destination directory, counting calls in fetches.
func ociMockFetch(t *testing.T, sourceURI string, files map[string]string, fetches *int) *downloader.MockFileDownloader {
	t.Helper()

	ctrl := gomock.NewController(t)
	mock := downloader.NewMockFileDownloader(ctrl)
	mock.EXPECT().
		Fetch(sourceURI, gomock.Any(), downloader.ClientModeDir, gomock.Any()).
		DoAndReturn(func(_, dest string, _ downloader.ClientMode, _ time.Duration) error {
			*fetches++
			for name, content := range files {
				path := filepath.Join(dest, filepath.FromSlash(name))
				if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
					return err
				}
				if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
					return err
				}
			}
			return nil
		}).
		AnyTimes()
	return mock
}

// TestRemoteImporter_Resolve_OCI_PullsArtifactOncePerRun verifies that several imports from one
// artifact share a single registry pull within an invocation.
func TestRemoteImporter_Resolve_OCI_PullsArtifactOncePerRun(t *testing.T) {
	const sourceURI = "oci://registry.example.com/org/stacks:v1"
	testCache, err := cache.NewFileCache("test", cache.WithBaseDir(t.TempDir()))
	require.NoError(t, err)

	fetches := 0
	importer, err := NewRemoteImporter(&schema.AtmosConfiguration{},
		WithCache(testCache), WithDownloader(ociMockFetch(t, sourceURI, ociStackFiles, &fetches)))
	require.NoError(t, err)

	vpc, err := importer.Resolve(sourceURI + "//stacks/catalog/vpc.yaml")
	require.NoError(t, err)
	dns, err := importer.Resolve(sourceURI + "//stacks/catalog/nested/dns.yaml")
	require.NoError(t, err)

	assert.Equal(t, "vars:\n  cidr: 10.0.0.0/16\n", readMatch(t, vpc[0]))
	assert.Equal(t, "vars:\n  zone: example.com\n", readMatch(t, dns[0]))
	assert.Equal(t, 1, fetches)
}

// TestRemoteImporter_Resolve_OCI_TTLReusesCacheAcrossRuns verifies cross-run reuse of the pulled
// artifact: a fresh importer (a new invocation) sharing the cache directory skips the registry
// while the configured imports.ttl is fresh, and pulls again when no ttl is configured.
func TestRemoteImporter_Resolve_OCI_TTLReusesCacheAcrossRuns(t *testing.T) {
	const sourceURI = "oci://registry.example.com/org/stacks:v1"
	uri := sourceURI + "//stacks/catalog/vpc.yaml"
	cacheDir := t.TempDir()

	newImporter := func(ttl string, fetches *int) *RemoteImporter {
		testCache, err := cache.NewFileCache("test", cache.WithBaseDir(cacheDir))
		require.NoError(t, err)
		importer, err := NewRemoteImporter(
			&schema.AtmosConfiguration{Imports: schema.ImportsSettings{TTL: ttl}},
			WithCache(testCache), WithDownloader(ociMockFetch(t, sourceURI, ociStackFiles, fetches)),
		)
		require.NoError(t, err)
		return importer
	}

	var firstRun int
	_, err := newImporter("1h", &firstRun).Resolve(uri)
	require.NoError(t, err)
	assert.Equal(t, 1, firstRun)

	var secondRun int
	matches, err := newImporter("1h", &secondRun).Resolve(uri)
	require.NoError(t, err)
	assert.Equal(t, "vars:\n  cidr: 10.0.0.0/16\n", readMatch(t, matches[0]))
	assert.Equal(t, 0, secondRun, "a fresh ttl must reuse the cached artifact without pulling")

	var noTTLRun int
	_, err = newImporter("", &noTTLRun).Resolve(uri)
	require.NoError(t, err)
	assert.Equal(t, 1, noTTLRun, "without a ttl the artifact is pulled again on each run")
}

// TestRemoteImporter_Resolve_OCI_RefreshesCachedFiles verifies that a mutable tag picks up new artifact
// content on the next run: the cached per-file copy must not outlive a re-pull of the artifact. With a
// fresh ttl the artifact is not re-pulled, so the cached content is correctly kept.
func TestRemoteImporter_Resolve_OCI_RefreshesCachedFiles(t *testing.T) {
	const sourceURI = "oci://registry.example.com/org/stacks:latest"
	uri := sourceURI + "//stacks/catalog/vpc.yaml"

	resolve := func(cacheDir, ttl, content string) string {
		testCache, err := cache.NewFileCache("test", cache.WithBaseDir(cacheDir))
		require.NoError(t, err)
		var fetches int
		importer, err := NewRemoteImporter(
			&schema.AtmosConfiguration{Imports: schema.ImportsSettings{TTL: ttl}},
			WithCache(testCache),
			WithDownloader(ociMockFetch(t, sourceURI, map[string]string{"stacks/catalog/vpc.yaml": content}, &fetches)),
		)
		require.NoError(t, err)

		matches, err := importer.Resolve(uri)
		require.NoError(t, err)
		return readMatch(t, matches[0])
	}

	t.Run("no ttl re-pulls and serves the new content", func(t *testing.T) {
		cacheDir := t.TempDir()
		assert.Equal(t, "v1", resolve(cacheDir, "", "v1"))
		assert.Equal(t, "v2", resolve(cacheDir, "", "v2"))
	})

	t.Run("fresh ttl keeps the cached content without re-pulling", func(t *testing.T) {
		cacheDir := t.TempDir()
		assert.Equal(t, "v1", resolve(cacheDir, "1h", "v1"))
		assert.Equal(t, "v1", resolve(cacheDir, "1h", "v2"))
	})
}

func TestRemoteImporter_Resolve_OCI_DownloaderError(t *testing.T) {
	expectedErr := errors.New("registry unavailable")
	testCache, err := cache.NewFileCache("test", cache.WithBaseDir(t.TempDir()))
	require.NoError(t, err)

	ctrl := gomock.NewController(t)
	mock := downloader.NewMockFileDownloader(ctrl)
	mock.EXPECT().
		Fetch("oci://registry.example.com/org/stacks:v1", gomock.Any(), downloader.ClientModeDir, gomock.Any()).
		Return(expectedErr)

	importer, err := NewRemoteImporter(&schema.AtmosConfiguration{}, WithCache(testCache), WithDownloader(mock))
	require.NoError(t, err)

	matches, err := importer.Resolve("oci://registry.example.com/org/stacks:v1//stacks/catalog/vpc.yaml")
	require.Error(t, err)
	assert.Nil(t, matches)
	assert.ErrorIs(t, err, expectedErr)
}

func TestRemoteImporter_ResolveNested_OCI_RemoteBase(t *testing.T) {
	ref := ocitest.NewRegistry(t, "stacks:v1", ociStackFiles)
	importer := newTestRemoteImporter(t, &schema.AtmosConfiguration{Stacks: schema.Stacks{BasePath: "stacks"}})

	uri := "oci://" + ref + "//stacks/orgs/acme/_defaults.yaml"
	matches, err := importer.ResolveNested(uri, schema.StackImportNestedImportsRemote)
	require.NoError(t, err)
	require.Len(t, matches, 1)

	assert.Equal(t, uri+"#stacks/orgs/acme/_defaults.yaml", matches[0].Key)
	assert.True(t, strings.HasSuffix(filepath.ToSlash(matches[0].BasePath), "/stacks"))

	data, err := os.ReadFile(filepath.Join(matches[0].BasePath, "catalog", "_defaults.yaml"))
	require.NoError(t, err)
	assert.Contains(t, string(data), "catalog: true")
}

func TestRemoteImporter_ResolveNested_OCI_MissingPath(t *testing.T) {
	importer := newTestRemoteImporter(t, &schema.AtmosConfiguration{})

	matches, err := importer.ResolveNested("oci://registry.example.com/org/stacks:v1", schema.StackImportNestedImportsRemote)
	require.Error(t, err)
	assert.Nil(t, matches)
	assert.ErrorIs(t, err, errUtils.ErrInvalidRemoteImport)
}

func TestIsOCIURI(t *testing.T) {
	tests := []struct {
		uri      string
		expected bool
	}{
		{"oci://ghcr.io/org/stacks:v1//catalog/vpc.yaml", true},
		{"oci://github.com/org/stacks:v1//catalog/vpc.yaml", true},
		{"git::https://github.com/org/repo.git//stacks", false},
		{"https://example.com/stack.yaml", false},
		{"catalog/vpc", false},
		{"./oci://local", false},
	}

	for _, tt := range tests {
		t.Run(tt.uri, func(t *testing.T) {
			assert.Equal(t, tt.expected, IsOCIURI(tt.uri))
		})
	}
}
