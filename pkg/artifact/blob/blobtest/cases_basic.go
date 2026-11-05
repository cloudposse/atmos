package blobtest

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/artifact/blob"
)

func (c *conformance) putGetRoundTrip(t *testing.T) {
	store := c.newStore(t)
	data := []byte("hello blob")

	info := put(t, store, keyNested, data)
	assert.Equal(t, keyNested, info.Key)
	assert.Equal(t, int64(len(data)), info.Size)
	assert.NotEmpty(t, info.ETag)

	assert.Equal(t, data, read(t, store, keyNested))
	stat, err := store.Stat(context.Background(), keyNested, blob.StatOptions{})
	require.NoError(t, err)
	assert.Equal(t, int64(len(data)), stat.Size)
	assert.Equal(t, info.ETag, stat.ETag, "Put and Stat must report the same concurrency token")
}

func (c *conformance) zeroLength(t *testing.T) {
	store := c.newStore(t)
	info := put(t, store, "empty", nil)
	assert.Zero(t, info.Size)
	assert.NotEmpty(t, info.ETag)

	assert.Empty(t, read(t, store, "empty"))
	stat, err := store.Stat(context.Background(), "empty", blob.StatOptions{})
	require.NoError(t, err)
	assert.Zero(t, stat.Size)

	// A zero-length body of unknown size is also a valid object.
	_, err = store.Put(context.Background(), "empty-unknown", bytes.NewReader(nil), blob.PutOptions{Size: blob.UnknownSize})
	require.NoError(t, err)
	assert.Empty(t, read(t, store, "empty-unknown"))
}

func (c *conformance) missing(t *testing.T) {
	store := c.newStore(t)
	ctx := context.Background()

	_, err := store.Get(ctx, "nope", blob.GetOptions{})
	assert.True(t, blob.IsNotFound(err), "Get: %v", err)
	_, err = store.Stat(ctx, "nope", blob.StatOptions{})
	assert.True(t, blob.IsNotFound(err), "Stat: %v", err)
	assert.NoError(t, store.Delete(ctx, "nope", blob.DeleteOptions{}), "unconditional delete of a missing object succeeds")
}

func (c *conformance) overwrite(t *testing.T) {
	store := c.newStore(t)
	first := put(t, store, keyObject, []byte("one"))
	second := put(t, store, keyObject, []byte("two!"))
	assert.NotEqual(t, first.ETag, second.ETag, "different content must yield a different token")
	assert.Equal(t, []byte("two!"), read(t, store, keyObject))

	require.NoError(t, store.Delete(context.Background(), keyObject, blob.DeleteOptions{}))
	requireAbsent(t, store, keyObject)
}

func (c *conformance) invalidKeys(t *testing.T) {
	store := c.newStore(t)
	_, err := store.Put(context.Background(), "", bytes.NewReader([]byte(bodyX)), blob.PutOptions{Size: 1})
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrBlobInvalid)
	_, err = store.Get(context.Background(), "", blob.GetOptions{})
	require.Error(t, err)
	_, err = store.Stat(context.Background(), "", blob.StatOptions{})
	require.Error(t, err)
}

func (c *conformance) attributes(t *testing.T) {
	store := c.newStore(t)
	caps := store.Capabilities()
	ctx := context.Background()
	opts := blob.PutOptions{
		Size:        3,
		ContentType: "application/json",
		Metadata:    map[string]string{"owner": "atmos"},
		Tags:        map[string]string{"team": "platform", "env": "dev"},
	}
	_, err := store.Put(ctx, keyAttrs, bytes.NewReader([]byte("{ }")), opts)
	require.NoError(t, err)

	stat, err := store.Stat(ctx, keyAttrs, blob.StatOptions{IncludeTags: true})
	require.NoError(t, err)
	if caps.Metadata {
		assert.Equal(t, "application/json", stat.ContentType)
		assert.Equal(t, "atmos", stat.Metadata["owner"])
	}
	if caps.Tags {
		assert.Equal(t, opts.Tags, stat.Tags)
	}

	// Rewriting without attributes must not keep the old ones.
	put(t, store, keyAttrs, []byte("{ }"))
	stat, err = store.Stat(ctx, keyAttrs, blob.StatOptions{IncludeTags: true})
	require.NoError(t, err)
	assert.Empty(t, stat.Tags)
	assert.Empty(t, stat.Metadata)

	tooMany := map[string]string{}
	for _, key := range []string{"a", "b", "c", "d", "e", "f", "g", "h", "i", "j", "k"} {
		tooMany[key] = "v"
	}
	_, err = store.Put(ctx, "bad-tags", bytes.NewReader(nil), blob.PutOptions{Size: 0, Tags: tooMany})
	require.Error(t, err, "more than the S3 limit of 10 tags must be rejected on every backend")
	requireAbsent(t, store, "bad-tags")
}

func (c *conformance) ranged(t *testing.T) {
	store := c.newStore(t)
	if !store.Capabilities().Range {
		t.Skipf("store does not advertise range reads")
	}
	put(t, store, "r", []byte("0123456789"))
	for _, tc := range []struct {
		name string
		opts blob.GetOptions
		want string
	}{
		{"middle", blob.GetOptions{Offset: 2, Length: 4}, "2345"},
		{"tail", blob.GetOptions{Offset: tailOffset}, "789"},
		{"clamped", blob.GetOptions{Offset: clampedOffset, Length: listPageGuard}, "89"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			object, err := store.Get(context.Background(), "r", tc.opts)
			require.NoError(t, err)
			defer object.Body.Close()
			data, err := io.ReadAll(object.Body)
			require.NoError(t, err)
			assert.Equal(t, tc.want, string(data))
			assert.Equal(t, int64(10), object.Size, "Size is the full object size")
		})
	}
}

func (c *conformance) sizeMismatch(t *testing.T) {
	store := c.newStore(t)
	for name, body := range map[string]string{"short": "ab", "long": "abcdef"} {
		_, err := store.Put(context.Background(), name, bytes.NewReader([]byte(body)), blob.PutOptions{Size: 4})
		require.Error(t, err, name)
		requireAbsent(t, store, name)
	}
}

// errSourceBroke is the failure the broken-source case injects.
var errSourceBroke = errors.New("source broke")

// mockFailingReader yields data and then fails.
type mockFailingReader struct {
	data []byte
	err  error
}

func (f *mockFailingReader) Read(p []byte) (int, error) {
	if len(f.data) == 0 {
		return 0, f.err
	}
	n := copy(p, f.data)
	f.data = f.data[n:]
	return n, nil
}

func (c *conformance) failedBody(t *testing.T) {
	store := c.newStore(t)
	boom := errSourceBroke
	_, err := store.Put(context.Background(), "partial", &mockFailingReader{data: bytes.Repeat([]byte(bodyX), sourceChunk), err: boom}, blob.PutOptions{Size: blob.UnknownSize})
	require.ErrorIs(t, err, boom)
	requireAbsent(t, store, "partial")
}

func (c *conformance) canceled(t *testing.T) {
	store := c.newStore(t)
	ctx, cancel := context.WithCancel(context.Background())
	body := io.MultiReader(bytes.NewReader(bytes.Repeat([]byte(bodyX), sourceChunk)), &mockCancelingReader{cancel: cancel})
	_, err := store.Put(ctx, "canceled", body, blob.PutOptions{Size: blob.UnknownSize})
	require.Error(t, err)
	assert.ErrorIs(t, err, context.Canceled)
	requireAbsent(t, store, "canceled")
}

// mockCancelingReader cancels its context on first read and then reports the cancellation.
type mockCancelingReader struct{ cancel context.CancelFunc }

func (r *mockCancelingReader) Read([]byte) (int, error) {
	r.cancel()
	return 0, context.Canceled
}

func (c *conformance) largeStream(t *testing.T) {
	store := c.newStore(t)
	const size = 3<<20 + 123
	unit := []byte("0123456789abcdef")
	build := func() io.Reader {
		return io.LimitReader(&mockRepeatReader{unit: unit}, size)
	}
	want := sha256.New()
	_, err := io.Copy(want, build())
	require.NoError(t, err)

	for name, declared := range map[string]int64{"known-size": size, "unknown-size": blob.UnknownSize} {
		info, err := store.Put(context.Background(), name, build(), blob.PutOptions{Size: declared})
		require.NoError(t, err, name)
		assert.Equal(t, int64(size), info.Size, name)

		object, err := store.Get(context.Background(), name, blob.GetOptions{})
		require.NoError(t, err, name)
		got := sha256.New()
		n, err := io.Copy(got, object.Body)
		require.NoError(t, err, name)
		require.NoError(t, object.Body.Close())
		assert.Equal(t, int64(size), n, name)
		assert.Equal(t, want.Sum(nil), got.Sum(nil), name)
	}
}

// mockRepeatReader yields unit forever.
type mockRepeatReader struct {
	unit []byte
	off  int
}

func (r *mockRepeatReader) Read(p []byte) (int, error) {
	for i := range p {
		p[i] = r.unit[(r.off+i)%len(r.unit)]
	}
	r.off = (r.off + len(p)) % len(r.unit)
	return len(p), nil
}
