package imports

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/cache"
	"github.com/cloudposse/atmos/pkg/downloader"
	"github.com/cloudposse/atmos/pkg/schema"
)

const (
	testSourceURI = "git::https://example.com/acme/infrastructure.git?ref=main"
	testDirPerm   = 0o755
	testFilePerm  = 0o644
)

// newDirFetchMock returns a mock downloader whose directory Fetch populates a small
// stack catalog under stacks/ and increments the returned counter on each call.
func newDirFetchMock(t *testing.T, count *atomic.Int32) *downloader.MockFileDownloader {
	t.Helper()
	ctrl := gomock.NewController(t)
	md := downloader.NewMockFileDownloader(ctrl)
	md.EXPECT().
		Fetch(testSourceURI, gomock.Any(), downloader.ClientModeDir, gomock.Any()).
		DoAndReturn(func(_ string, dest string, _ downloader.ClientMode, _ time.Duration) error {
			count.Add(1)
			stacksDir := filepath.Join(dest, "stacks")
			if err := os.MkdirAll(stacksDir, testDirPerm); err != nil {
				return err
			}
			if err := os.WriteFile(filepath.Join(stacksDir, "a.yaml"), []byte("vars:\n  a: true\n"), testFilePerm); err != nil {
				return err
			}
			return os.WriteFile(filepath.Join(stacksDir, "b.yaml"), []byte("vars:\n  b: true\n"), testFilePerm)
		}).
		AnyTimes()
	return md
}

func newImporterWithCache(t *testing.T, baseDir string, count *atomic.Int32) *RemoteImporter {
	t.Helper()
	testCache, err := cache.NewFileCache("test", cache.WithBaseDir(baseDir))
	require.NoError(t, err)
	importer, err := NewRemoteImporter(&schema.AtmosConfiguration{}, WithCache(testCache), WithDownloader(newDirFetchMock(t, count)))
	require.NoError(t, err)
	return importer
}

// TestRemoteImporter_GitSubdir_DedupesSourceCloneWithinRun verifies that two distinct
// subdir imports of the same repo trigger only a single source clone per invocation.
func TestRemoteImporter_GitSubdir_DedupesSourceCloneWithinRun(t *testing.T) {
	var count atomic.Int32
	importer := newImporterWithCache(t, t.TempDir(), &count)

	aMatches, err := importer.Resolve("git::https://example.com/acme/infrastructure.git//stacks/a.yaml?ref=main")
	require.NoError(t, err)
	require.Len(t, aMatches, 1)
	aData, err := os.ReadFile(aMatches[0].Path)
	require.NoError(t, err)
	assert.Contains(t, string(aData), "a: true")

	bMatches, err := importer.Resolve("git::https://example.com/acme/infrastructure.git//stacks/b.yaml?ref=main")
	require.NoError(t, err)
	require.Len(t, bMatches, 1)
	bData, err := os.ReadFile(bMatches[0].Path)
	require.NoError(t, err)
	assert.Contains(t, string(bData), "b: true")

	assert.Equal(t, int32(1), count.Load(), "the shared source repo should be cloned exactly once per invocation")
}

// TestRemoteImporter_GitSubdir_DistinctSourcesFetchedSeparately is the negative case for
// dedup: imports from different source repos must each be fetched.
func TestRemoteImporter_GitSubdir_DistinctSourcesFetchedSeparately(t *testing.T) {
	testCache, err := cache.NewFileCache("test", cache.WithBaseDir(t.TempDir()))
	require.NoError(t, err)

	ctrl := gomock.NewController(t)
	md := downloader.NewMockFileDownloader(ctrl)
	var count atomic.Int32
	populate := func(_ string, dest string, _ downloader.ClientMode, _ time.Duration) error {
		count.Add(1)
		stacksDir := filepath.Join(dest, "stacks")
		if err := os.MkdirAll(stacksDir, testDirPerm); err != nil {
			return err
		}
		return os.WriteFile(filepath.Join(stacksDir, "a.yaml"), []byte("vars:\n  a: true\n"), testFilePerm)
	}
	md.EXPECT().Fetch("git::https://example.com/acme/one.git?ref=main", gomock.Any(), downloader.ClientModeDir, gomock.Any()).DoAndReturn(populate).Times(1)
	md.EXPECT().Fetch("git::https://example.com/acme/two.git?ref=main", gomock.Any(), downloader.ClientModeDir, gomock.Any()).DoAndReturn(populate).Times(1)

	importer, err := NewRemoteImporter(&schema.AtmosConfiguration{}, WithCache(testCache), WithDownloader(md))
	require.NoError(t, err)

	_, err = importer.Resolve("git::https://example.com/acme/one.git//stacks/a.yaml?ref=main")
	require.NoError(t, err)
	_, err = importer.Resolve("git::https://example.com/acme/two.git//stacks/a.yaml?ref=main")
	require.NoError(t, err)

	assert.Equal(t, int32(2), count.Load(), "distinct source repos must each be cloned")
}

// TestRemoteImporter_GitSubdir_TTLCrossRunReuse verifies cross-run cache behavior:
// a fresh persisted clone is reused within TTL, but is refreshed when no TTL is set or
// when the clone has expired. Each importer has its own session, simulating separate runs.
func TestRemoteImporter_GitSubdir_TTLCrossRunReuse(t *testing.T) {
	baseDir := t.TempDir()
	const importURI = "git::https://example.com/acme/infrastructure.git//stacks/a.yaml?ref=main"

	// Run 1: cold cache -> fetch once.
	var c1 atomic.Int32
	_, err := newImporterWithCache(t, baseDir, &c1).resolveNested(importURI, "", "1h")
	require.NoError(t, err)
	assert.Equal(t, int32(1), c1.Load(), "cold cache must fetch")

	// Run 2: warm cache, fresh TTL -> reuse, no fetch.
	var c2 atomic.Int32
	_, err = newImporterWithCache(t, baseDir, &c2).resolveNested(importURI, "", "1h")
	require.NoError(t, err)
	assert.Equal(t, int32(0), c2.Load(), "fresh persisted clone should be reused across runs within TTL")

	// Run 3: no TTL -> always refresh.
	var c3 atomic.Int32
	_, err = newImporterWithCache(t, baseDir, &c3).resolveNested(importURI, "", "")
	require.NoError(t, err)
	assert.Equal(t, int32(1), c3.Load(), "no TTL should refresh the clone each run")

	// Run 4: backdate the persisted clone past the TTL -> refresh.
	destDir := filepath.Join(baseDir, uriToTempName(testSourceURI)+".source")
	backdateSourceMetadata(t, destDir, 2*time.Hour)
	var c4 atomic.Int32
	_, err = newImporterWithCache(t, baseDir, &c4).resolveNested(importURI, "", "1h")
	require.NoError(t, err)
	assert.Equal(t, int32(1), c4.Load(), "expired clone should be refreshed")
}

// backdateSourceMetadata rewrites the cached source freshness marker with an UpdatedAt
// set `age` in the past, simulating an aged cross-run cache entry.
func backdateSourceMetadata(t *testing.T, destDir string, age time.Duration) {
	t.Helper()
	meta := sourceMetadata{SourceURI: testSourceURI, UpdatedAt: time.Now().Add(-age)}
	data, err := json.Marshal(meta)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(destDir, sourceReadyFileName), data, sourceReadyFilePerm))
}

// setGlobalImporter installs imp as the package global importer for the duration of a
// test. It consumes globalImporterOnce so getGlobalImporter returns imp unchanged (a
// fresh sync.Once would otherwise run and overwrite imp with a real, default-downloader
// importer). The global state is reset on cleanup.
func setGlobalImporter(t *testing.T, imp *RemoteImporter) {
	t.Helper()
	globalImporterOnce = sync.Once{}
	globalImporterOnce.Do(func() {})
	globalImporter = imp
	globalImporterErr = nil
	t.Cleanup(func() {
		globalImporterOnce = sync.Once{}
		globalImporter = nil
		globalImporterErr = nil
	})
}

// TestResolveRemoteImportNested_GlobalImporter covers the public ResolveRemoteImportNested
// convenience wrapper end-to-end through the global importer, including within-run dedup of
// the shared source clone across two subdir imports of the same repo.
func TestResolveRemoteImportNested_GlobalImporter(t *testing.T) {
	var count atomic.Int32
	setGlobalImporter(t, newImporterWithCache(t, t.TempDir(), &count))

	cfg := &schema.AtmosConfiguration{}
	aMatches, err := ResolveRemoteImportNested(cfg, "git::https://example.com/acme/infrastructure.git//stacks/a.yaml?ref=main", "", "1h")
	require.NoError(t, err)
	require.Len(t, aMatches, 1)
	aData, err := os.ReadFile(aMatches[0].Path)
	require.NoError(t, err)
	assert.Contains(t, string(aData), "a: true")

	// A second subdir import of the same source repo reuses the within-run clone.
	bMatches, err := ResolveRemoteImportNested(cfg, "git::https://example.com/acme/infrastructure.git//stacks/b.yaml?ref=main", "", "1h")
	require.NoError(t, err)
	require.Len(t, bMatches, 1)
	bData, err := os.ReadFile(bMatches[0].Path)
	require.NoError(t, err)
	assert.Contains(t, string(bData), "b: true")

	assert.Equal(t, int32(1), count.Load(), "the global importer should clone the shared source once per run")
}

// TestReadSourceMetadata covers the parse/validation paths of the cross-run freshness
// marker reader: a missing marker, malformed JSON, a legacy marker with no timestamp, and
// a valid marker.
func TestReadSourceMetadata(t *testing.T) {
	t.Run("missing marker", func(t *testing.T) {
		_, err := readSourceMetadata(t.TempDir())
		require.Error(t, err)
	})

	t.Run("malformed JSON", func(t *testing.T) {
		dir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(dir, sourceReadyFileName), []byte("not-json"), sourceReadyFilePerm))
		_, err := readSourceMetadata(dir)
		require.Error(t, err)
	})

	t.Run("legacy marker without timestamp", func(t *testing.T) {
		dir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(dir, sourceReadyFileName), []byte(`{"source_uri":"x"}`), sourceReadyFilePerm))
		_, err := readSourceMetadata(dir)
		require.ErrorIs(t, err, errUtils.ErrInvalidRemoteImport)
	})

	t.Run("valid marker", func(t *testing.T) {
		dir := t.TempDir()
		backdateSourceMetadata(t, dir, 0)
		meta, err := readSourceMetadata(dir)
		require.NoError(t, err)
		assert.Equal(t, testSourceURI, meta.SourceURI)
		assert.False(t, meta.UpdatedAt.IsZero())
	})
}

// TestSourceCacheFresh covers the freshness predicate directly.
func TestSourceCacheFresh(t *testing.T) {
	destDir := t.TempDir()

	// No metadata yet -> not fresh.
	assert.False(t, sourceCacheFresh(destDir, "1h"))

	// Empty TTL -> never reuse across runs, regardless of metadata.
	backdateSourceMetadata(t, destDir, 0)
	assert.False(t, sourceCacheFresh(destDir, ""))

	// Recent metadata within TTL -> fresh.
	assert.True(t, sourceCacheFresh(destDir, "1h"))

	// Aged metadata beyond TTL -> not fresh.
	backdateSourceMetadata(t, destDir, 2*time.Hour)
	assert.False(t, sourceCacheFresh(destDir, "1h"))

	// Zero TTL is always expired.
	backdateSourceMetadata(t, destDir, 0)
	assert.False(t, sourceCacheFresh(destDir, "0s"))

	// Invalid TTL fails safe to not-fresh (forces refresh).
	assert.False(t, sourceCacheFresh(destDir, "not-a-duration"))
}
