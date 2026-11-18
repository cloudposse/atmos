package s3

import (
	"bytes"
	"context"
	"io"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/artifact/blob"
	"github.com/cloudposse/atmos/pkg/artifact/blob/s3/s3test"
	"github.com/cloudposse/atmos/pkg/store"
)

// smallPart is a part size that lets tests exercise multipart with kilobytes.
const smallPart = 16 << 10

// newFake starts a fake S3 endpoint.
func newFake(t *testing.T, opts ...s3test.Option) *s3test.Server {
	t.Helper()
	return s3test.NewServer(t, opts...)
}

// newTestStore returns a store over the fake with small parts unless overridden.
func newTestStore(t *testing.T, fake *s3test.Server, opts ...Option) *Store {
	t.Helper()
	all := append([]Option{WithPartSize(smallPart)}, opts...)
	s, err := NewFromClient(fake.Client(), fake.Bucket(), all...)
	require.NoError(t, err)
	return s
}

// putBytes stores data and fails the test on error.
func putBytes(t *testing.T, s *Store, key string, data []byte, opts blob.PutOptions) *blob.ObjectInfo {
	t.Helper()
	if opts.Size == 0 && len(data) > 0 {
		opts.Size = int64(len(data))
	}
	info, err := s.Put(context.Background(), key, bytes.NewReader(data), opts)
	require.NoError(t, err)
	return info
}

// patterned returns n deterministic bytes.
func patterned(n int) []byte {
	data := make([]byte, n)
	for i := range data {
		data[i] = byte(i*7 + i/251)
	}
	return data
}

// countingReader generates size bytes and counts what the consumer has read.
type countingReader struct {
	size     int64
	read     atomic.Int64
	onRead   func(read int64)
	exhaust  func()
	finished atomic.Bool
}

func (r *countingReader) Read(p []byte) (int, error) {
	remaining := r.size - r.read.Load()
	if remaining <= 0 {
		if r.finished.CompareAndSwap(false, true) && r.exhaust != nil {
			r.exhaust()
		}
		return 0, io.EOF
	}
	n := int64(len(p))
	if n > remaining {
		n = remaining
	}
	for i := int64(0); i < n; i++ {
		p[i] = byte(i)
	}
	total := r.read.Add(n)
	if r.onRead != nil {
		r.onRead(total)
	}
	return int(n), nil
}

// fakeResolver is an AuthResolver double.
type fakeResolver struct {
	result *store.AWSAuthConfig
	err    error
	calls  atomic.Int32
}

func (f *fakeResolver) ResolveAWSAuthContext(_ context.Context, _ string) (*store.AWSAuthConfig, error) {
	f.calls.Add(1)
	return f.result, f.err
}
