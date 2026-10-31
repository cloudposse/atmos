package s3

import (
	"bytes"
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/artifact/blob"
	"github.com/cloudposse/atmos/pkg/artifact/blob/s3/s3test"
)

func TestSmallBodyUsesOneSignedRequest(t *testing.T) {
	fake := newFake(t)
	s := newTestStore(t, fake)
	putBytes(t, s, "k", []byte("tiny"), blob.PutOptions{ContentType: "text/plain", Metadata: map[string]string{"Owner": "atmos"}})

	requests := fake.Requests()
	require.Len(t, requests, 1, "no multipart machinery for a small body")
	assert.Equal(t, s3test.OpPutObject, requests[0].Op)
	assert.Equal(t, "text/plain", requests[0].Header.Get("Content-Type"))
	assert.Equal(t, "atmos", requests[0].Header.Get("X-Amz-Meta-Owner"))
	assert.Empty(t, requests[0].Header.Get("If-None-Match"), "unconditional puts carry no condition")
	assert.Empty(t, fake.OpenUploads())
}

func TestZeroLengthObjectIsOnePutWithEmptyBody(t *testing.T) {
	fake := newFake(t)
	s := newTestStore(t, fake)
	putBytes(t, s, "empty", nil, blob.PutOptions{})
	requests := fake.RequestsFor(s3test.OpPutObject)
	require.Len(t, requests, 1)
	assert.Equal(t, int64(0), requests[0].Bytes)
	object, ok := fake.Object("empty")
	require.True(t, ok)
	assert.Zero(t, object.Size)
}

func TestBodyExactlyOnePartIsNotMultipart(t *testing.T) {
	fake := newFake(t)
	s := newTestStore(t, fake)
	putBytes(t, s, "exact", patterned(smallPart), blob.PutOptions{})
	assert.Empty(t, fake.RequestsFor(s3test.OpCreateMultipartUpload))
	assert.Len(t, fake.RequestsFor(s3test.OpPutObject), 1)
}

func TestMultipartRequestOrderAndContent(t *testing.T) {
	fake := newFake(t, s3test.WithMinPartSize(smallPart))
	s := newTestStore(t, fake, WithConcurrency(1))
	data := patterned(smallPart*3 + 17)
	info := putBytes(t, s, "big", data, blob.PutOptions{
		ContentType: "application/octet-stream",
		Tags:        map[string]string{"team": "platform"},
		Condition:   blob.IfAbsent(),
	})

	var ops []string
	for _, req := range fake.Requests() {
		ops = append(ops, req.Op)
	}
	assert.Equal(t, []string{
		s3test.OpCreateMultipartUpload,
		s3test.OpUploadPart, s3test.OpUploadPart, s3test.OpUploadPart, s3test.OpUploadPart,
		s3test.OpCompleteMultipartUpload,
	}, ops, "create, then every part, then complete last")

	create := fake.RequestsFor(s3test.OpCreateMultipartUpload)[0]
	assert.Equal(t, "team=platform", create.Header.Get("X-Amz-Tagging"), "tags are set when the upload is created")
	assert.Empty(t, create.Header.Get("If-None-Match"), "the condition belongs to the completing request")
	complete := fake.RequestsFor(s3test.OpCompleteMultipartUpload)[0]
	assert.Equal(t, "*", complete.Header.Get("If-None-Match"))
	for i, part := range fake.RequestsFor(s3test.OpUploadPart) {
		assert.Equal(t, []string{string(rune('1' + i))}, part.Query["partNumber"], "parts are numbered in order")
	}

	object, ok := fake.Object("big")
	require.True(t, ok)
	assert.Equal(t, data, object.Data)
	assert.Equal(t, info.ETag, object.ETag)
	assert.Contains(t, object.ETag, "-4", "a multipart ETag records its part count and is not a payload checksum")
	assert.Equal(t, map[string]string{"team": "platform"}, object.Tags)
	assert.Equal(t, "application/octet-stream", object.ContentType)
	assert.Equal(t, 1, fake.CompletedUploads())
	assert.Empty(t, fake.OpenUploads())
}

func TestUnknownSizeStreamsAsMultipart(t *testing.T) {
	fake := newFake(t)
	s := newTestStore(t, fake)
	data := patterned(smallPart*2 + 5)
	info, err := s.Put(context.Background(), "stream", bytes.NewReader(data), blob.PutOptions{Size: blob.UnknownSize})
	require.NoError(t, err)
	assert.Equal(t, int64(len(data)), info.Size)
	object, _ := fake.Object("stream")
	assert.Equal(t, data, object.Data)
	assert.Len(t, fake.RequestsFor(s3test.OpUploadPart), 3)
}

func TestKnownSizeGrowsPartsToStayUnderPartLimit(t *testing.T) {
	assert.Equal(t, int64(10), effectivePartSize(10*MaxParts, 1))
	assert.Equal(t, int64(smallPart), effectivePartSize(100, smallPart))
	assert.Equal(t, int64(smallPart), effectivePartSize(blob.UnknownSize, smallPart))
}

func TestTagsAreCopiedToEveryObject(t *testing.T) {
	fake := newFake(t)
	s := newTestStore(t, fake, WithTags(map[string]string{"managed-by": "atmos", "env": "dev"}))

	putBytes(t, s, "single", []byte("x"), blob.PutOptions{})
	putBytes(t, s, "multi", patterned(smallPart*2+1), blob.PutOptions{})
	putBytes(t, s, "override", []byte("x"), blob.PutOptions{Tags: map[string]string{"env": "prod", "extra": "1"}})
	putBytes(t, s, "empty", nil, blob.PutOptions{})

	want := map[string]map[string]string{
		"single":   {"managed-by": "atmos", "env": "dev"},
		"multi":    {"managed-by": "atmos", "env": "dev"},
		"override": {"managed-by": "atmos", "env": "prod", "extra": "1"},
		"empty":    {"managed-by": "atmos", "env": "dev"},
	}
	for key, tags := range want {
		object, ok := fake.Object(key)
		require.True(t, ok, key)
		assert.Equal(t, tags, object.Tags, "tags on %s", key)
	}
}

func TestTooManyTagsRejectedBeforeAnyRequest(t *testing.T) {
	fake := newFake(t)
	tags := map[string]string{}
	for _, key := range []string{"a", "b", "c", "d", "e", "f"} {
		tags[key] = "v"
	}
	s := newTestStore(t, fake, WithTags(tags))
	_, err := s.Put(context.Background(), "k", bytes.NewReader(nil), blob.PutOptions{Tags: map[string]string{"g": "v", "h": "v", "i": "v", "j": "v", "k": "v"}})
	require.ErrorIs(t, err, errUtils.ErrBlobInvalid)
	assert.Empty(t, fake.Requests())
}

func TestSizeMismatchIsRejectedAndMultipartAborted(t *testing.T) {
	fake := newFake(t)
	s := newTestStore(t, fake)
	_, err := s.Put(context.Background(), "short", bytes.NewReader([]byte("ab")), blob.PutOptions{Size: 9})
	require.ErrorIs(t, err, errUtils.ErrBlobInvalid)
	assert.Empty(t, fake.Requests(), "a single-part size mismatch is caught before sending")

	data := patterned(smallPart*2 + 3)
	_, err = s.Put(context.Background(), "long", bytes.NewReader(data), blob.PutOptions{Size: int64(len(data) - 1)})
	require.ErrorIs(t, err, errUtils.ErrBlobInvalid)
	assert.Empty(t, fake.OpenUploads(), "the half-built upload was aborted")
	assert.Equal(t, 1, fake.AbortedUploads())
	assert.Empty(t, fake.Keys())
}

func TestCanceledOperationAbortsItsMultipartUpload(t *testing.T) {
	fake := newFake(t)
	fake.Inject(s3test.Fault{Op: s3test.OpUploadPart, Action: s3test.Delay, Delay: 200 * time.Millisecond, Times: -1})
	s := newTestStore(t, fake, WithConcurrency(2))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	source := &countingReader{size: 64 * smallPart, onRead: func(read int64) {
		if read > 3*smallPart {
			cancel()
		}
	}}
	_, err := s.Put(ctx, "canceled", source, blob.PutOptions{Size: blob.UnknownSize})
	require.Error(t, err)
	assert.ErrorIs(t, err, context.Canceled)
	assert.False(t, blob.IsAmbiguous(err), "a canceled part upload cannot have created the object")

	assert.Empty(t, fake.OpenUploads(), "no multipart upload may outlive its canceled operation")
	assert.Equal(t, 1, fake.AbortedUploads())
	assert.Empty(t, fake.Keys())
}

func TestFailedSourceAbortsItsMultipartUpload(t *testing.T) {
	fake := newFake(t)
	s := newTestStore(t, fake)
	boom := errors.New("disk vanished")
	body := &errAfterReader{data: patterned(smallPart*2 + 10), err: boom}
	_, err := s.Put(context.Background(), "k", body, blob.PutOptions{Size: blob.UnknownSize})
	require.ErrorIs(t, err, boom)
	assert.Empty(t, fake.OpenUploads())
	assert.Equal(t, 1, fake.AbortedUploads())
	assert.Empty(t, fake.Keys())
}

func TestFailedPartAbortsItsMultipartUpload(t *testing.T) {
	fake := newFake(t)
	fake.Inject(s3test.Fault{Op: s3test.OpUploadPart, Skip: 1, Times: -1, Status: 400, Code: "InvalidPart"})
	s := newTestStore(t, fake, WithConcurrency(1))
	_, err := s.Put(context.Background(), "k", bytes.NewReader(patterned(smallPart*4)), blob.PutOptions{Size: blob.UnknownSize})
	require.Error(t, err)
	assert.Empty(t, fake.OpenUploads())
	assert.Equal(t, 1, fake.AbortedUploads())
}

func TestLostConditionAtCompletionAbortsTheUpload(t *testing.T) {
	fake := newFake(t)
	s := newTestStore(t, fake)
	fake.Seed("taken", []byte("existing"), nil)

	_, err := s.Put(context.Background(), "taken", bytes.NewReader(patterned(smallPart*2+1)), blob.PutOptions{Size: blob.UnknownSize, Condition: blob.IfAbsent()})
	require.Error(t, err)
	assert.True(t, blob.IsPreconditionFailed(err), "%v", err)
	assert.Empty(t, fake.OpenUploads(), "the losing writer cleans up its parts")
	object, _ := fake.Object("taken")
	assert.Equal(t, []byte("existing"), object.Data)
}

func TestMemoryStaysBoundedWhileStreaming(t *testing.T) {
	const (
		part        = 1 << 20
		total       = 48 << 20
		concurrency = 3
	)
	fake := newFake(t, s3test.WithDiscardBodies())
	fake.Inject(s3test.Fault{Op: s3test.OpUploadPart, Action: s3test.Delay, Delay: 2 * time.Millisecond, Times: -1})
	s := newTestStore(t, fake, WithPartSize(part), WithConcurrency(concurrency))

	var maxInFlight atomic.Int64
	source := &countingReader{size: total}
	source.onRead = func(read int64) {
		inFlight := read - fake.PartBytesReceived()
		for {
			current := maxInFlight.Load()
			if inFlight <= current || maxInFlight.CompareAndSwap(current, inFlight) {
				return
			}
		}
	}
	info, err := s.Put(context.Background(), "huge", source, blob.PutOptions{Size: blob.UnknownSize})
	require.NoError(t, err)
	assert.Equal(t, int64(total), info.Size)

	bound := int64(concurrency+1) * part
	assert.LessOrEqual(t, maxInFlight.Load(), bound, "bytes read but not yet uploaded must stay within the buffer pool, not the object size")
	assert.LessOrEqual(t, fake.MaxConcurrentParts(), concurrency, "worker concurrency is bounded")
	assert.GreaterOrEqual(t, fake.MaxConcurrentParts(), 2, "parts do upload in parallel")
	assert.Equal(t, int64(total), fake.PartBytesReceived())
}

// errAfterReader yields data and then fails.
type errAfterReader struct {
	data []byte
	err  error
}

func (r *errAfterReader) Read(p []byte) (int, error) {
	if len(r.data) == 0 {
		return 0, r.err
	}
	n := copy(p, r.data)
	r.data = r.data[n:]
	return n, nil
}
