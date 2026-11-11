package local

import (
	"bytes"
	"context"
	"crypto/sha256"
	"io"
	"math"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/artifact/blob"
	"github.com/cloudposse/atmos/pkg/artifact/blob/blobtest"
)

func newStore(t *testing.T, opts ...Option) *Store {
	t.Helper()
	store, err := New(filepath.Join(t.TempDir(), "blobs"), opts...)
	require.NoError(t, err)
	return store
}

func TestConformance(t *testing.T) {
	blobtest.RunStoreConformance(t, func(t *testing.T) blob.Store { return newStore(t) })
}

func TestConformanceLockAllWrites(t *testing.T) {
	blobtest.RunStoreConformance(t, func(t *testing.T) blob.Store { return newStore(t, WithLockAllWrites()) })
}

func TestNewRequiresRoot(t *testing.T) {
	_, err := New("")
	require.ErrorIs(t, err, errUtils.ErrBlobInvalid)
}

func TestWritesPlainFilesWithoutExtraArtifacts(t *testing.T) {
	store := newStore(t)
	_, err := store.Put(context.Background(), "stack/component/sha.tar", bytes.NewReader([]byte("tar")), blob.PutOptions{Size: 3})
	require.NoError(t, err)

	data, err := os.ReadFile(filepath.Join(store.Root(), "stack", "component", "sha.tar"))
	require.NoError(t, err)
	assert.Equal(t, []byte("tar"), data)

	entries, err := os.ReadDir(filepath.Join(store.Root(), "stack", "component"))
	require.NoError(t, err)
	require.Len(t, entries, 1, "no temp, attribute, or lock files remain for a plain unconditional put")
	assert.NoFileExists(t, filepath.Join(store.Root(), lockName))
}

func TestConditionalPutCreatesLockFileOnly(t *testing.T) {
	store := newStore(t)
	_, err := store.Put(context.Background(), "k", bytes.NewReader([]byte("a")), blob.PutOptions{Size: 1, Condition: blob.IfAbsent()})
	require.NoError(t, err)
	assert.FileExists(t, filepath.Join(store.Root(), lockName))

	objects, _, err := blob.ListAll(context.Background(), store, blob.ListOptions{})
	require.NoError(t, err)
	require.Len(t, objects, 1, "bookkeeping files are not listed")
	assert.Equal(t, "k", objects[0].Key)
}

func TestAttributeFileHiddenAndRemoved(t *testing.T) {
	store := newStore(t)
	ctx := context.Background()
	_, err := store.Put(ctx, "d/k", bytes.NewReader([]byte("a")), blob.PutOptions{Size: 1, Tags: map[string]string{"a": "b"}})
	require.NoError(t, err)
	assert.FileExists(t, filepath.Join(store.Root(), "d", "k"+attrsSuffix))

	objects, _, err := blob.ListAll(ctx, store, blob.ListOptions{})
	require.NoError(t, err)
	assert.Len(t, objects, 1)

	require.NoError(t, store.Delete(ctx, "d/k", blob.DeleteOptions{}))
	assert.NoDirExists(t, filepath.Join(store.Root(), "d"), "empty parent directories are cleaned up")
}

func TestReservedAndUnsafeKeysRejected(t *testing.T) {
	store := newStore(t)
	for _, key := range []string{"../escape", "a/../../b", "/abs", "a//b", "a/", ".", "d/" + tempPrefix + "x", "d/x" + attrsSuffix, lockName, "a\\b"} {
		_, err := store.Put(context.Background(), key, bytes.NewReader(nil), blob.PutOptions{Size: 0})
		require.Error(t, err, key)
		assert.ErrorIs(t, err, errUtils.ErrBlobInvalid, key)
	}
	assert.NoDirExists(t, filepath.Join(filepath.Dir(store.Root()), "escape"))
}

func TestObjectAndPrefixCannotShareAPath(t *testing.T) {
	store := newStore(t)
	ctx := context.Background()
	_, err := store.Put(ctx, "a", bytes.NewReader([]byte("x")), blob.PutOptions{Size: 1})
	require.NoError(t, err)

	_, err = store.Put(ctx, "a/b", bytes.NewReader([]byte("x")), blob.PutOptions{Size: 1})
	require.ErrorIs(t, err, errUtils.ErrBlobInvalid, "a parent path that is an object")

	_, err = store.Put(ctx, "c/d", bytes.NewReader([]byte("x")), blob.PutOptions{Size: 1})
	require.NoError(t, err)
	_, err = store.Put(ctx, "c", bytes.NewReader([]byte("x")), blob.PutOptions{Size: 1})
	require.ErrorIs(t, err, errUtils.ErrBlobInvalid, "a key that is a prefix of other objects")

	_, err = store.Stat(ctx, "c", blob.StatOptions{})
	assert.True(t, blob.IsNotFound(err), "a prefix is not an object")
}

func TestLargeObjectGetsWeakETag(t *testing.T) {
	store := newStore(t)
	big := bytes.Repeat([]byte("x"), contentETagLimit+1)
	info, err := store.Put(context.Background(), "big", bytes.NewReader(big), blob.PutOptions{Size: int64(len(big))})
	require.NoError(t, err)
	assert.Contains(t, info.ETag, `W/"`)
	stat, err := store.Stat(context.Background(), "big", blob.StatOptions{})
	require.NoError(t, err)
	assert.Equal(t, info.ETag, stat.ETag)
}

func TestListRejectsEscapingPrefix(t *testing.T) {
	store := newStore(t)
	_, err := store.List(context.Background(), blob.ListOptions{Prefix: "../"})
	require.ErrorIs(t, err, errUtils.ErrBlobInvalid)
}

func TestGetRejectsNegativeOffsetAndPastEnd(t *testing.T) {
	store := newStore(t)
	_, err := store.Put(context.Background(), "k", bytes.NewReader([]byte("abc")), blob.PutOptions{Size: 3})
	require.NoError(t, err)
	_, err = store.Get(context.Background(), "k", blob.GetOptions{Offset: -1})
	require.ErrorIs(t, err, errUtils.ErrBlobInvalid)
	_, err = store.Get(context.Background(), "k", blob.GetOptions{Offset: 9, Length: 1})
	require.ErrorIs(t, err, errUtils.ErrBlobInvalid)
}

func TestCanceledContextFailsFast(t *testing.T) {
	store := newStore(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := store.Put(ctx, "k", bytes.NewReader([]byte("x")), blob.PutOptions{Size: 1})
	require.ErrorIs(t, err, context.Canceled)
	_, err = store.Stat(ctx, "k", blob.StatOptions{})
	require.ErrorIs(t, err, context.Canceled)
	assert.NoDirExists(t, store.Root(), "nothing is created for a canceled operation")
}

func TestInvalidSizesAndNULPrefix(t *testing.T) {
	store := newStore(t)
	for _, size := range []int64{-2, math.MaxInt64} {
		_, err := store.Put(context.Background(), "k", bytes.NewReader(nil), blob.PutOptions{Size: size})
		require.ErrorIs(t, err, errUtils.ErrBlobInvalid)
	}
	_, err := store.List(context.Background(), blob.ListOptions{Prefix: "x\x00y"})
	require.ErrorIs(t, err, errUtils.ErrBlobInvalid)
}

func TestDescribeHashesOpenedFile(t *testing.T) {
	store := newStore(t)
	require.NoError(t, os.MkdirAll(store.Root(), 0o755))
	path := filepath.Join(store.Root(), "k")
	require.NoError(t, os.WriteFile(path, []byte("old"), 0o644))
	file, err := openFile(path)
	require.NoError(t, err)
	defer file.Close()
	require.NoError(t, os.Rename(path, path+".old"))
	require.NoError(t, os.WriteFile(path, []byte("new"), 0o644))
	info, err := store.describe(blob.OpGet, "k", path, file, false)
	require.NoError(t, err)
	sum := sha256.Sum256([]byte("old"))
	assert.Equal(t, contentETag(sum[:]), info.ETag)
	body, err := io.ReadAll(file)
	require.NoError(t, err)
	assert.Equal(t, "old", string(body))
}

func TestRootLockWaitRespectsCancellation(t *testing.T) {
	store := newStore(t)
	release, err := store.lockRoot(context.Background(), blob.OpPut, "k")
	require.NoError(t, err)
	defer release()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = store.lockRoot(ctx, blob.OpPut, "k")
	require.ErrorIs(t, err, context.Canceled)
}

func TestPutRejectsUnknownCondition(t *testing.T) {
	store := newStore(t)
	_, err := store.Put(context.Background(), "k", bytes.NewReader(nil), blob.PutOptions{Condition: blob.Condition{Kind: blob.ConditionKind(999)}})
	require.ErrorIs(t, err, errUtils.ErrBlobInvalid)
}
