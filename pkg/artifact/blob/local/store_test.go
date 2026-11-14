package local

import (
	"bytes"
	"context"
	"crypto/sha256"
	"io"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

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
	store := newStore(t, WithPlainFileCompatibility())
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
	for _, key := range []string{"../escape", "a/../../b", "/abs", "a//b", "a/", ".", "d/" + tempPrefix + "x", "d/x" + attrsSuffix, tempPrefix + "dir/child", "a/" + tempPrefix + "dir/child", "x" + attrsSuffix + "/child", lockName, "a\\b"} {
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

func TestLargeObjectContentETag(t *testing.T) {
	store := newStore(t)
	big := bytes.Repeat([]byte("x"), (4<<20)+1)
	info, err := store.Put(context.Background(), "big", bytes.NewReader(big), blob.PutOptions{Size: int64(len(big))})
	require.NoError(t, err)
	stamp := time.Unix(1700000000, 0)
	require.NoError(t, os.Chtimes(filepath.Join(store.Root(), "big"), stamp, stamp))
	stat, err := store.Stat(context.Background(), "big", blob.StatOptions{})
	require.NoError(t, err)
	assert.Equal(t, info.ETag, stat.ETag)
	big[len(big)-1] = 'y'
	_, err = store.Put(context.Background(), "big", bytes.NewReader(big), blob.PutOptions{Size: int64(len(big))})
	require.NoError(t, err)
	require.NoError(t, os.Chtimes(filepath.Join(store.Root(), "big"), stamp, stamp))
	updated, err := store.Stat(context.Background(), "big", blob.StatOptions{})
	require.NoError(t, err)
	assert.NotEqual(t, stat.ETag, updated.ETag)
	_, err = store.Put(context.Background(), "big", bytes.NewReader(big), blob.PutOptions{Size: int64(len(big)), Condition: blob.IfMatch(stat.ETag)})
	assert.ErrorIs(t, err, errUtils.ErrBlobPreconditionFailed)
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
	oldPath := filepath.Join(store.Root(), "old")
	require.NoError(t, os.WriteFile(oldPath, []byte("old"), 0o644))
	file, err := openFile(oldPath)
	require.NoError(t, err)
	defer file.Close()
	require.NoError(t, os.WriteFile(path, []byte("new"), 0o644))
	root, err := store.openRoot(false)
	require.NoError(t, err)
	defer root.Close()
	info, err := store.describe(context.Background(), root, describeTarget{op: blob.OpGet, key: "k", file: file}, describeOptions{})
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

func TestSymlinksCannotEscapeRoot(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("creating symlinks requires extra privileges on Windows")
	}
	store := newStore(t)
	require.NoError(t, os.MkdirAll(store.Root(), 0o755))
	outside := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(outside, "object"), []byte("keep"), 0o644))
	require.NoError(t, os.Symlink(outside, filepath.Join(store.Root(), "team")))
	ctx := context.Background()
	_, err := store.Put(ctx, "team/object", bytes.NewReader([]byte("overwrite")), blob.PutOptions{Size: 9})
	require.Error(t, err)
	_, err = store.Get(ctx, "team/object", blob.GetOptions{})
	require.Error(t, err)
	_, err = store.Stat(ctx, "team/object", blob.StatOptions{})
	require.Error(t, err)
	require.Error(t, store.Delete(ctx, "team/object", blob.DeleteOptions{}))
	page, listErr := store.List(ctx, blob.ListOptions{Prefix: "team/"})
	if listErr == nil {
		assert.Empty(t, page.Objects)
	}
	data, err := os.ReadFile(filepath.Join(outside, "object"))
	require.NoError(t, err)
	assert.Equal(t, "keep", string(data))
}

func TestListingPagesUseSnapshotAndFreshListingWalksAgain(t *testing.T) {
	store := newStore(t)
	ctx := context.Background()
	for _, key := range []string{"a", "b"} {
		_, err := store.Put(ctx, key, bytes.NewReader([]byte(key)), blob.PutOptions{Size: 1})
		require.NoError(t, err)
	}
	first, err := store.List(ctx, blob.ListOptions{Limit: 1})
	require.NoError(t, err)
	require.Len(t, first.Objects, 1)
	assert.Equal(t, "a", first.Objects[0].Key)
	require.NotEmpty(t, first.NextCursor)
	require.NoError(t, store.Delete(ctx, "b", blob.DeleteOptions{}))
	_, err = store.Put(ctx, "c", bytes.NewReader([]byte("c")), blob.PutOptions{Size: 1})
	require.NoError(t, err)
	second, err := store.List(ctx, blob.ListOptions{Limit: 1, Cursor: first.NextCursor})
	require.NoError(t, err)
	require.Len(t, second.Objects, 1)
	assert.Equal(t, "b", second.Objects[0].Key, "continuation uses the original walk")
	fresh, err := store.List(ctx, blob.ListOptions{Limit: 2})
	require.NoError(t, err)
	require.Len(t, fresh.Objects, 2)
	assert.Equal(t, "c", fresh.Objects[1].Key, "a fresh listing sees filesystem changes")
	other, err := New(store.Root())
	require.NoError(t, err)
	resumed, err := other.List(ctx, blob.ListOptions{Limit: 1, Cursor: first.NextCursor})
	require.NoError(t, err)
	require.Len(t, resumed.Objects, 1)
	assert.Equal(t, "c", resumed.Objects[0].Key, "an evicted snapshot resumes from its ordering token")
}

func TestPutDirectoryConflictPreservesExistingSidecar(t *testing.T) {
	store := newStore(t)
	ctx := context.Background()
	_, err := store.Put(ctx, "prefix/child", bytes.NewReader([]byte("x")), blob.PutOptions{Size: 1})
	require.NoError(t, err)
	sidecar := filepath.Join(store.Root(), "prefix"+attrsSuffix)
	original := []byte(`{"content_type":"original"}`)
	require.NoError(t, os.WriteFile(sidecar, original, 0o644))
	_, err = store.Put(ctx, "prefix", bytes.NewReader([]byte("x")), blob.PutOptions{Size: 1, ContentType: "new"})
	require.ErrorIs(t, err, errUtils.ErrBlobInvalid)
	got, err := os.ReadFile(sidecar)
	require.NoError(t, err)
	assert.Equal(t, original, got)
}

func TestFailedPutPrunesEmptyStagingDirectories(t *testing.T) {
	for name, opts := range map[string][]Option{"managed": nil, "plain": {WithPlainFileCompatibility()}} {
		t.Run(name, func(t *testing.T) {
			store := newStore(t, opts...)
			_, err := store.Put(context.Background(), "keep/sibling", bytes.NewReader([]byte("ok")), blob.PutOptions{Size: 2})
			require.NoError(t, err)

			_, err = store.Put(context.Background(), "keep/new/deep/object", bytes.NewReader([]byte("abc")), blob.PutOptions{Size: 1})
			require.ErrorIs(t, err, errUtils.ErrBlobInvalid, "a body longer than the declared size fails")

			assert.NoDirExists(t, filepath.Join(store.Root(), "keep", "new"), "staging directories created by the failed put are pruned")
			assert.DirExists(t, filepath.Join(store.Root(), "keep"), "a non-empty parent is left intact")
			assert.FileExists(t, filepath.Join(store.Root(), "keep", "sibling"))
		})
	}
}

func TestListPrefixHonorsLiteralBackslashes(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("backslash is a path separator on Windows")
	}
	strict := newStore(t)
	_, err := strict.List(context.Background(), blob.ListOptions{Prefix: `dir\`})
	require.ErrorIs(t, err, errUtils.ErrBlobInvalid, "backslashes stay rejected unless the store opts in")

	literal := newStore(t, WithLiteralBackslashes())
	_, err = literal.Put(context.Background(), `dir\file`, bytes.NewReader([]byte("x")), blob.PutOptions{Size: 1})
	require.NoError(t, err)
	page, err := literal.List(context.Background(), blob.ListOptions{Prefix: `dir\`})
	require.NoError(t, err)
	require.Len(t, page.Objects, 1)
	assert.Equal(t, `dir\file`, page.Objects[0].Key)

	for _, prefix := range []string{"/abs", "a\x00b", "../x"} {
		_, err = literal.List(context.Background(), blob.ListOptions{Prefix: prefix})
		require.ErrorIs(t, err, errUtils.ErrBlobInvalid, "%q stays rejected", prefix)
	}
}

func TestReservedNamesFoldCase(t *testing.T) {
	store := newStore(t)
	for _, key := range []string{
		".ATMOS-BLOB-TMP-TRANSACTION",
		"d/.Atmos-Blob-Tmp-x",
		"d/x.ATMOS-BLOB.JSON",
		".ATMOS-BLOB.LOCK",
		"Mixed/.atmos-blob-tmp-dir/child",
	} {
		_, err := store.Put(context.Background(), key, bytes.NewReader([]byte("x")), blob.PutOptions{Size: 1})
		require.Error(t, err, key)
		assert.ErrorIs(t, err, errUtils.ErrBlobInvalid, key)
	}
	assert.NoDirExists(t, filepath.Join(store.Root(), "d"), "a rejected key stages nothing")
}

func TestDeletePrefixOnlyDirectoryIsMissingObject(t *testing.T) {
	for _, tc := range []struct {
		name string
		opts []Option
	}{
		{name: "managed"},
		{name: "plain", opts: []Option{WithPlainFileCompatibility()}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store := newStore(t, tc.opts...)
			ctx := context.Background()
			_, err := store.Put(ctx, "prefix/child", bytes.NewReader([]byte("child")), blob.PutOptions{Size: 5})
			require.NoError(t, err)

			require.NoError(t, store.Delete(ctx, "prefix", blob.DeleteOptions{}), "an unconditional delete of a missing object succeeds")

			info, err := store.Stat(ctx, "prefix/child", blob.StatOptions{})
			require.NoError(t, err, "the children stay in place")
			assert.Equal(t, int64(5), info.Size)
			assert.DirExists(t, filepath.Join(store.Root(), "prefix"))

			err = store.Delete(ctx, "prefix", blob.DeleteOptions{IfMatch: `"any"`})
			require.ErrorIs(t, err, errUtils.ErrBlobNotFound, "a conditional delete of a prefix reports the object as missing")
			assert.FileExists(t, filepath.Join(store.Root(), "prefix", "child"))
		})
	}
}

func TestETagHashingObservesCancellation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "big")
	content := bytes.Repeat([]byte("x"), 1<<20)
	require.NoError(t, os.WriteFile(path, content, 0o644))
	file, err := openFile(path)
	require.NoError(t, err)
	defer file.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = etagOpen(ctx, file)
	require.ErrorIs(t, err, context.Canceled, "hashing stops instead of scanning the whole file")

	etag, err := etagOpen(context.Background(), file)
	require.NoError(t, err)
	sum := sha256.Sum256(content)
	assert.Equal(t, contentETag(sum[:]), etag, "an active context hashes the full content")
}

func TestPlainFilesTreatAttributeLookingNamesAsObjects(t *testing.T) {
	store := newStore(t, WithPlainFileCompatibility())
	ctx := context.Background()
	const object, lookalike = "a.tar", "a.tar" + attrsSuffix
	put := func(key, body string) {
		t.Helper()
		_, err := store.Put(ctx, key, bytes.NewReader([]byte(body)), blob.PutOptions{Size: int64(len(body))})
		require.NoError(t, err, key)
	}
	read := func(key string) string {
		t.Helper()
		obj, err := store.Get(ctx, key, blob.GetOptions{})
		require.NoError(t, err, key)
		defer obj.Body.Close()
		data, err := io.ReadAll(obj.Body)
		require.NoError(t, err)
		return string(data)
	}

	put(object, "tar-v1")
	put(lookalike, "not json at all")
	assert.Equal(t, "tar-v1", read(object), "an unrelated object beside the data is not parsed as attributes")

	put(object, "tar-v2")
	assert.Equal(t, "tar-v2", read(object))
	assert.Equal(t, "not json at all", read(lookalike), "updating a.tar leaves the independent artifact untouched")

	page, err := store.List(ctx, blob.ListOptions{})
	require.NoError(t, err)
	require.Len(t, page.Objects, 2, "both names are listed as objects")
	assert.Equal(t, object, page.Objects[0].Key)
	assert.Equal(t, lookalike, page.Objects[1].Key)

	require.NoError(t, store.Delete(ctx, object, blob.DeleteOptions{}))
	assert.Equal(t, "not json at all", read(lookalike), "deleting a.tar leaves the independent artifact untouched")

	_, err = store.Put(ctx, "tagged", bytes.NewReader([]byte("x")), blob.PutOptions{Size: 1, Tags: map[string]string{"a": "b"}})
	require.ErrorIs(t, err, errUtils.ErrBlobInvalid, "attributes cannot be stored without a sidecar")
	assert.False(t, store.Capabilities().Tags)
	assert.False(t, store.Capabilities().Metadata)
}

func TestSkipETagAvoidsHashingTheObject(t *testing.T) {
	store := newStore(t)
	ctx := context.Background()
	_, err := store.Put(ctx, "k", bytes.NewReader([]byte("body")), blob.PutOptions{Size: 4})
	require.NoError(t, err)

	tests := []struct {
		name     string
		skip     bool
		wantETag bool
	}{
		{name: "default computes the content ETag", wantETag: true},
		{name: "skip leaves the ETag empty", skip: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			stat, err := store.Stat(ctx, "k", blob.StatOptions{SkipETag: tc.skip})
			require.NoError(t, err)
			assert.Equal(t, int64(4), stat.Size)
			assert.Equal(t, tc.wantETag, stat.ETag != "")

			obj, err := store.Get(ctx, "k", blob.GetOptions{SkipETag: tc.skip})
			require.NoError(t, err)
			defer obj.Body.Close()
			data, err := io.ReadAll(obj.Body)
			require.NoError(t, err)
			assert.Equal(t, "body", string(data), "the body is complete either way")
			assert.Equal(t, tc.wantETag, obj.ETag != "")
		})
	}
}
