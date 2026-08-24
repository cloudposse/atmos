package cache

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
)

// fakeBackend is an in-memory cache.Backend for manager tests.
type fakeBackend struct {
	blobs        map[string][]byte
	saveCalls    int
	restoreCalls int
	forceExists  bool

	// Optional error injection. When set, the corresponding method returns the
	// error instead of its normal behavior, so the Manager's wrapping branches
	// can be exercised.
	restoreErr error
	saveErr    error
	listErr    error
	deleteErr  error
}

func newFakeBackend() *fakeBackend {
	return &fakeBackend{blobs: map[string][]byte{}}
}

func (f *fakeBackend) Name() string { return "fake" }

func (f *fakeBackend) Save(_ context.Context, key string, data io.Reader, _ int64) error {
	f.saveCalls++
	if f.saveErr != nil {
		return f.saveErr
	}
	if f.forceExists {
		return errUtils.ErrCacheAlreadyExists
	}
	if _, ok := f.blobs[key]; ok {
		return errUtils.ErrCacheAlreadyExists
	}
	b, err := io.ReadAll(data)
	if err != nil {
		return err
	}
	f.blobs[key] = b
	return nil
}

func (f *fakeBackend) Restore(_ context.Context, key string, restoreKeys []string) (string, io.ReadCloser, error) {
	f.restoreCalls++
	if f.restoreErr != nil {
		return "", nil, f.restoreErr
	}
	if b, ok := f.blobs[key]; ok {
		return key, io.NopCloser(bytes.NewReader(b)), nil
	}
	for _, rk := range restoreKeys {
		for k, b := range f.blobs {
			if strings.HasPrefix(k, rk) {
				return k, io.NopCloser(bytes.NewReader(b)), nil
			}
		}
	}
	return "", nil, errUtils.ErrCacheNotFound
}

func (f *fakeBackend) List(_ context.Context, _ ListOptions) ([]Entry, error) {
	if f.listErr != nil {
		return nil, f.listErr
	}
	entries := make([]Entry, 0, len(f.blobs))
	for k := range f.blobs {
		entries = append(entries, Entry{Key: k})
	}
	return entries, nil
}

func (f *fakeBackend) Delete(_ context.Context, key string) error {
	if f.deleteErr != nil {
		return f.deleteErr
	}
	delete(f.blobs, key)
	return nil
}

// newTestConfig builds a minimal valid config rooted at a temp dir with one file.
func newTestConfig(t *testing.T, key string) *Config {
	t.Helper()
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, "toolchain", "bin"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "toolchain", "bin", "tool"), []byte("binary-content"), 0o644))
	return &Config{
		Enabled:     true,
		Auto:        autoBoth,
		Root:        root,
		Key:         key,
		Compression: compressionGzip,
	}
}

func TestManager_SaveThenRestoreRoundTrip(t *testing.T) {
	cfg := newTestConfig(t, "k1")
	fake := newFakeBackend()
	m := NewManager(fake, cfg)

	saveRes, err := m.Save(context.Background())
	require.NoError(t, err)
	require.True(t, saveRes.Saved)
	require.Len(t, fake.blobs, 1)

	// Remove the restored content and confirm Restore puts it back.
	require.NoError(t, os.Remove(filepath.Join(cfg.Root, "toolchain", "bin", "tool")))

	restoreRes, err := m.Restore(context.Background())
	require.NoError(t, err)
	require.True(t, restoreRes.Hit)
	require.True(t, restoreRes.Exact)

	got, err := os.ReadFile(filepath.Join(cfg.Root, "toolchain", "bin", "tool"))
	require.NoError(t, err)
	assert.Equal(t, "binary-content", string(got))
}

func TestManager_SaveSkippedOnExactHit(t *testing.T) {
	cfg := newTestConfig(t, "k1")
	fake := newFakeBackend()
	m := NewManager(fake, cfg)

	// Simulate an exact-key cache hit at restore time.
	recordRestore(cfg.Root, cfg.Key, restoredExact, cfg.Key)

	res, err := m.Save(context.Background())
	require.NoError(t, err)
	assert.True(t, res.Skipped)
	assert.Equal(t, 0, fake.saveCalls, "save must not be attempted on an exact hit")
}

// Negative path for the exact-hit skip: a miss must NOT skip the save.
func TestManager_SaveFiresOnMiss(t *testing.T) {
	cfg := newTestConfig(t, "k1")
	fake := newFakeBackend()
	m := NewManager(fake, cfg)

	recordRestore(cfg.Root, cfg.Key, restoredMiss, "")

	res, err := m.Save(context.Background())
	require.NoError(t, err)
	assert.True(t, res.Saved)
	assert.False(t, res.Skipped)
	assert.Equal(t, 1, fake.saveCalls)
}

func TestManager_RestoreIsIdempotent(t *testing.T) {
	cfg := newTestConfig(t, "k1")
	fake := newFakeBackend()
	m := NewManager(fake, cfg)

	// Seed the backend by saving first.
	_, err := m.Save(context.Background())
	require.NoError(t, err)

	first, err := m.Restore(context.Background())
	require.NoError(t, err)
	require.True(t, first.Hit)
	require.False(t, first.Skipped)

	second, err := m.Restore(context.Background())
	require.NoError(t, err)
	assert.True(t, second.Skipped, "second restore of the same key is a no-op")
	assert.Equal(t, 1, fake.restoreCalls, "backend must not be hit again")
}

func TestManager_RestorePrefixFallback(t *testing.T) {
	cfg := newTestConfig(t, "atmos-cache-linux-amd64-newhash")
	cfg.RestoreKeys = []string{"atmos-cache-linux-amd64-"}
	fake := newFakeBackend()
	// Pre-seed a prefix-matching entry built from the same root.
	m := NewManager(fake, cfg)
	archivePath, size, err := m.buildArchive()
	require.NoError(t, err)
	defer os.Remove(archivePath)
	data, err := os.ReadFile(archivePath)
	require.NoError(t, err)
	_ = size
	fake.blobs["atmos-cache-linux-amd64-oldhash"] = data

	res, err := m.Restore(context.Background())
	require.NoError(t, err)
	assert.True(t, res.Hit)
	assert.False(t, res.Exact)
	assert.Equal(t, "atmos-cache-linux-amd64-oldhash", res.MatchedKey)
}

func TestManager_RestoreMiss(t *testing.T) {
	cfg := newTestConfig(t, "missing")
	fake := newFakeBackend()
	m := NewManager(fake, cfg)

	res, err := m.Restore(context.Background())
	require.NoError(t, err)
	assert.False(t, res.Hit)

	e := lookupEntry(cfg.Root, cfg.Key)
	require.NotNil(t, e)
	assert.Equal(t, restoredMiss, e.RestoredFrom)
}

func TestManager_SaveAlreadyExistsRemotely(t *testing.T) {
	cfg := newTestConfig(t, "k1")
	fake := newFakeBackend()
	fake.forceExists = true
	m := NewManager(fake, cfg)

	res, err := m.Save(context.Background())
	require.NoError(t, err)
	assert.True(t, res.Saved)
	assert.True(t, res.Skipped)

	e := lookupEntry(cfg.Root, cfg.Key)
	require.NotNil(t, e)
	assert.True(t, e.Saved)
}

func TestManager_SaveSkippedWhenAlreadySaved(t *testing.T) {
	cfg := newTestConfig(t, "k1")
	fake := newFakeBackend()
	m := NewManager(fake, cfg)

	recordSaved(cfg.Root, cfg.Key)

	res, err := m.Save(context.Background())
	require.NoError(t, err)
	assert.True(t, res.Skipped)
	assert.Equal(t, 0, fake.saveCalls)
}

func TestManager_DeleteRequiresKey(t *testing.T) {
	cfg := newTestConfig(t, "k1")
	m := NewManager(newFakeBackend(), cfg)

	err := m.Delete(context.Background(), "")
	require.ErrorIs(t, err, errUtils.ErrCacheKeyRequired)
}

func TestManager_Config(t *testing.T) {
	cfg := newTestConfig(t, "k1")
	m := NewManager(newFakeBackend(), cfg)
	assert.Same(t, cfg, m.Config())
}

func TestManager_List(t *testing.T) {
	cfg := newTestConfig(t, "k1")
	fake := newFakeBackend()
	fake.blobs["alpha"] = []byte("a")
	fake.blobs["beta"] = []byte("b")
	m := NewManager(fake, cfg)

	entries, err := m.List(context.Background(), "")
	require.NoError(t, err)
	require.Len(t, entries, 2)
	keys := []string{entries[0].Key, entries[1].Key}
	assert.Contains(t, keys, "alpha")
	assert.Contains(t, keys, "beta")
}

func TestManager_DeleteSuccess(t *testing.T) {
	cfg := newTestConfig(t, "k1")
	fake := newFakeBackend()
	fake.blobs["gone"] = []byte("x")
	m := NewManager(fake, cfg)

	require.NoError(t, m.Delete(context.Background(), "gone"))
	_, ok := fake.blobs["gone"]
	assert.False(t, ok, "entry should be removed from the backend")
}

// errInjected is an arbitrary backend failure used to exercise the Manager's
// error-wrapping branches (distinct from any sentinel the Manager special-cases).
var errInjected = errors.New("injected backend error")

func TestManager_RestoreRejectsInvalidConfig(t *testing.T) {
	// Empty Key fails validate() before the backend is touched.
	m := NewManager(newFakeBackend(), &Config{Root: t.TempDir()})

	_, err := m.Restore(context.Background())
	require.ErrorIs(t, err, errUtils.ErrCacheKeyRequired)
}

func TestManager_SaveRejectsInvalidConfig(t *testing.T) {
	m := NewManager(newFakeBackend(), &Config{Root: t.TempDir()})

	_, err := m.Save(context.Background())
	require.ErrorIs(t, err, errUtils.ErrCacheKeyRequired)
}

func TestManager_RestoreBackendError(t *testing.T) {
	cfg := newTestConfig(t, "k1")
	fake := newFakeBackend()
	// A non-NotFound backend error must be wrapped as a restore failure.
	fake.restoreErr = errInjected
	m := NewManager(fake, cfg)

	_, err := m.Restore(context.Background())
	require.ErrorIs(t, err, errUtils.ErrCacheRestoreFailed)
}

func TestManager_RestoreExtractError(t *testing.T) {
	cfg := newTestConfig(t, "k1")
	fake := newFakeBackend()
	// Seed a corrupt (non-gzip) blob under the exact key so the restore hits the
	// backend, then fails while extracting.
	fake.blobs[cfg.Key] = []byte("not a gzip archive")
	m := NewManager(fake, cfg)

	_, err := m.Restore(context.Background())
	require.ErrorIs(t, err, errUtils.ErrCacheExtractFailed)
}

func TestManager_SaveBackendError(t *testing.T) {
	cfg := newTestConfig(t, "k1")
	fake := newFakeBackend()
	// A non-AlreadyExists backend error must be wrapped as a save failure.
	fake.saveErr = errInjected
	m := NewManager(fake, cfg)

	_, err := m.Save(context.Background())
	require.ErrorIs(t, err, errUtils.ErrCacheSaveFailed)
}

func TestManager_ListBackendError(t *testing.T) {
	cfg := newTestConfig(t, "k1")
	fake := newFakeBackend()
	fake.listErr = errInjected
	m := NewManager(fake, cfg)

	_, err := m.List(context.Background(), "")
	require.ErrorIs(t, err, errUtils.ErrCacheListFailed)
}

func TestManager_DeleteBackendError(t *testing.T) {
	cfg := newTestConfig(t, "k1")
	fake := newFakeBackend()
	fake.deleteErr = errInjected
	m := NewManager(fake, cfg)

	err := m.Delete(context.Background(), "some-key")
	require.ErrorIs(t, err, errUtils.ErrCacheDeleteFailed)
}

func TestManager_BuildArchiveSuccess(t *testing.T) {
	cfg := newTestConfig(t, "k1")
	m := NewManager(newFakeBackend(), cfg)

	path, size, err := m.buildArchive()
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.Remove(path) })

	info, statErr := os.Stat(path)
	require.NoError(t, statErr, "archive temp file should exist")
	assert.Positive(t, size, "archive should be non-empty")
	assert.Equal(t, size, info.Size())
}

func TestManager_BuildArchiveError(t *testing.T) {
	cfg := newTestConfig(t, "k1")
	// Point the root at a path that does not exist so WalkDir fails.
	cfg.Root = filepath.Join(t.TempDir(), "does-not-exist")
	m := NewManager(newFakeBackend(), cfg)

	_, _, err := m.buildArchive()
	require.ErrorIs(t, err, errUtils.ErrCacheArchiveFailed)
}
