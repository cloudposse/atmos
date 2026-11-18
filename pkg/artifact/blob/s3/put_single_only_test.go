package s3

import (
	"bytes"
	"context"
	"net/http"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/artifact/blob"
	"github.com/cloudposse/atmos/pkg/artifact/blob/s3/s3test"
)

// isolateTempDir points the process temp directory at an empty directory and returns it, so a test
// can prove that the upload spool is removed.
func isolateTempDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	for _, name := range []string{"TMPDIR", "TMP", "TEMP"} {
		t.Setenv(name, dir)
	}
	return dir
}

func requireEmptyDir(t *testing.T, dir string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	assert.Empty(t, entries, "the upload spool must be removed")
}

// With multipart off, every body goes out as one PutObject, whatever its size and whether or not
// its length is declared. No multipart or read-back request is ever issued, so a write-only role
// is enough.
func TestSinglePutOnlyNeverUsesMultipartOrReadsBack(t *testing.T) {
	tests := []struct {
		name string
		size int
		// declared reports whether the caller declares the length.
		declared bool
	}{
		{name: "empty", size: 0, declared: true},
		{name: "below one part", size: smallPart - 1, declared: true},
		{name: "exactly one part", size: smallPart, declared: true},
		{name: "one byte past one part", size: smallPart + 1, declared: true},
		{name: "many parts", size: smallPart*5 + 17, declared: true},
		{name: "many parts of unknown length", size: smallPart*5 + 17},
		{name: "one byte past one part of unknown length", size: smallPart + 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmp := isolateTempDir(t)
			fake := newFake(t)
			for _, op := range []string{
				s3test.OpGetObject, s3test.OpHeadObject, s3test.OpGetObjectTagging, s3test.OpCreateMultipartUpload,
				s3test.OpUploadPart, s3test.OpCompleteMultipartUpload, s3test.OpAbortMultipartUpload,
			} {
				fake.Inject(s3test.Fault{Op: op, Times: -1, Action: s3test.FailStatus, Status: http.StatusForbidden, Code: "AccessDenied"})
			}
			s := newTestStore(t, fake, WithSinglePutOnly())
			data := patterned(tt.size)
			opts := blob.PutOptions{Size: blob.UnknownSize}
			if tt.declared {
				opts.Size = int64(len(data))
			}

			info, err := s.Put(context.Background(), "obj", bytes.NewReader(data), opts)

			require.NoError(t, err)
			assert.Equal(t, int64(len(data)), info.Size)
			stored, ok := fake.Object("obj")
			require.True(t, ok)
			assert.True(t, bytes.Equal(data, stored.Data), "stored %d bytes, want %d", len(stored.Data), len(data))
			requests := fake.Requests()
			require.Len(t, requests, 1)
			assert.Equal(t, s3test.OpPutObject, requests[0].Op)
			requireEmptyDir(t, tmp)
		})
	}
}

// A declared size that cannot fit one request is refused before the body is read or any request
// is sent, with an error that names the cause.
func TestSinglePutOnlyRejectsDeclaredSizeAboveTheLimit(t *testing.T) {
	fake := newFake(t)
	s := newTestStore(t, fake, WithSinglePutOnly())
	reader := &countingReader{size: MaxSinglePutSize + 1}

	_, err := s.Put(context.Background(), "obj", reader, blob.PutOptions{Size: MaxSinglePutSize + 1})

	require.ErrorIs(t, err, errUtils.ErrBlobInvalid)
	assert.Contains(t, err.Error(), "multipart uploads are disabled")
	assert.Zero(t, reader.read.Load(), "the body is not consumed")
	assert.Empty(t, fake.Requests())
}

// A stream of unknown length that outgrows one request stops at the limit, sends nothing, and
// removes its spool.
func TestSinglePutOnlyStopsAnUnknownLengthStreamAtTheLimit(t *testing.T) {
	tmp := isolateTempDir(t)
	fake := newFake(t)
	s := newTestStore(t, fake, WithSinglePutOnly())
	s.cfg.maxSinglePut = smallPart * 3
	data := patterned(smallPart*3 + 1)

	_, err := s.Put(context.Background(), "obj", bytes.NewReader(data), blob.PutOptions{Size: blob.UnknownSize})

	require.ErrorIs(t, err, errUtils.ErrBlobInvalid)
	assert.Empty(t, fake.Requests())
	requireEmptyDir(t, tmp)

	// A body exactly at the limit is still accepted.
	exact := patterned(smallPart * 3)
	_, err = s.Put(context.Background(), "obj", bytes.NewReader(exact), blob.PutOptions{Size: blob.UnknownSize})
	require.NoError(t, err)
	stored, ok := fake.Object("obj")
	require.True(t, ok)
	assert.True(t, bytes.Equal(exact, stored.Data))
	requireEmptyDir(t, tmp)
}

// A refused PutObject is reported as the upload's failure; no fallback hides it, and the spool is
// removed.
func TestSinglePutOnlyReportsPutObjectDenied(t *testing.T) {
	tmp := isolateTempDir(t)
	fake := newFake(t)
	fake.Inject(s3test.Fault{Op: s3test.OpPutObject, Times: -1, Action: s3test.FailStatus, Status: http.StatusForbidden, Code: "AccessDenied"})
	s := newTestStore(t, fake, WithSinglePutOnly())
	data := patterned(smallPart*2 + 1)

	_, err := s.Put(context.Background(), "obj", bytes.NewReader(data), blob.PutOptions{Size: int64(len(data))})

	require.Error(t, err)
	assert.Empty(t, fake.Keys())
	assert.Empty(t, fake.RequestsFor(s3test.OpCreateMultipartUpload))
	requireEmptyDir(t, tmp)
}

// The fallback option still turns an unimplemented multipart upload into one PutObject, and it
// refuses to spool without bound.
func TestSinglePutFallbackWhenMultipartUnsupported(t *testing.T) {
	fake := newFake(t)
	fake.Inject(s3test.Fault{Op: s3test.OpCreateMultipartUpload, Status: http.StatusNotImplemented, Code: "NotImplemented"})
	s := newTestStore(t, fake, WithSinglePutFallback())
	data := patterned(smallPart*2 + 3)

	_, err := s.Put(context.Background(), "obj", bytes.NewReader(data), blob.PutOptions{Size: int64(len(data))})

	require.NoError(t, err)
	stored, ok := fake.Object("obj")
	require.True(t, ok)
	assert.True(t, bytes.Equal(data, stored.Data))
	assert.Len(t, fake.RequestsFor(s3test.OpCreateMultipartUpload), 1)
	assert.Len(t, fake.RequestsFor(s3test.OpPutObject), 1)
}

func TestSinglePutFallbackDoesNotMaskOtherMultipartErrors(t *testing.T) {
	fake := newFake(t)
	fake.Inject(s3test.Fault{Op: s3test.OpCreateMultipartUpload, Status: http.StatusForbidden, Code: "AccessDenied"})
	s := newTestStore(t, fake, WithSinglePutFallback())
	data := patterned(smallPart*2 + 3)

	_, err := s.Put(context.Background(), "obj", bytes.NewReader(data), blob.PutOptions{Size: int64(len(data))})

	require.Error(t, err)
	assert.Empty(t, fake.RequestsFor(s3test.OpPutObject))
}

func TestSinglePutFallbackStopsAtTheLimit(t *testing.T) {
	tmp := isolateTempDir(t)
	fake := newFake(t)
	fake.Inject(s3test.Fault{Op: s3test.OpCreateMultipartUpload, Status: http.StatusNotImplemented, Code: "NotImplemented"})
	s := newTestStore(t, fake, WithSinglePutFallback())
	s.cfg.maxSinglePut = smallPart * 2
	data := patterned(smallPart*2 + 1)

	_, err := s.Put(context.Background(), "obj", bytes.NewReader(data), blob.PutOptions{Size: blob.UnknownSize})

	require.ErrorIs(t, err, errUtils.ErrBlobInvalid)
	assert.Empty(t, fake.RequestsFor(s3test.OpPutObject))
	requireEmptyDir(t, tmp)
}
