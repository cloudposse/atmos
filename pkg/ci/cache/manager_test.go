package cache

import (
	"bytes"
	"context"
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
}

func newFakeBackend() *fakeBackend {
	return &fakeBackend{blobs: map[string][]byte{}}
}

func (f *fakeBackend) Name() string { return "fake" }

func (f *fakeBackend) Save(_ context.Context, key string, data io.Reader, _ int64) error {
	f.saveCalls++
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
	entries := make([]Entry, 0, len(f.blobs))
	for k := range f.blobs {
		entries = append(entries, Entry{Key: k})
	}
	return entries, nil
}

func (f *fakeBackend) Delete(_ context.Context, key string) error {
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
