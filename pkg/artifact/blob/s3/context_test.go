package s3

import (
	"bytes"
	"context"
	"errors"
	"io"
	"sync"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/artifact/blob"
)

func TestProbeWaiterStopsWhenItsContextEnds(t *testing.T) {
	api := NewMockAPI(gomock.NewController(t))
	started := make(chan struct{})
	release := make(chan struct{})
	// The mocked PutObject parks on release. Free it on every exit so a failed assertion never
	// strands the probe goroutine.
	var freeOnce sync.Once
	free := func() { freeOnce.Do(func() { close(release) }) }
	t.Cleanup(free)
	errEndpointDown := errors.New("endpoint down")
	api.EXPECT().
		PutObject(gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(context.Context, *s3.PutObjectInput, ...func(*s3.Options)) (*s3.PutObjectOutput, error) {
			close(started)
			<-release
			return nil, errEndpointDown
		}).
		Times(1)
	api.EXPECT().DeleteObject(gomock.Any(), gomock.Any(), gomock.Any()).Return(&s3.DeleteObjectOutput{}, nil).Times(1)
	store, err := NewFromClient(api, "bucket", WithConditionalMode(ConditionalProbe))
	require.NoError(t, err)

	first := make(chan error, 1)
	go func() { first <- store.ProbeConditionalWrites(context.Background()) }()
	select {
	case <-started:
	case err := <-first:
		t.Fatalf("the first probe ended before it reached the endpoint: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("the first probe never reached the endpoint")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	waited := make(chan error, 1)
	go func() { waited <- store.ProbeConditionalWrites(ctx) }()
	select {
	case err := <-waited:
		require.ErrorIs(t, err, context.DeadlineExceeded, "the waiter gives up at its own deadline while the first probe is in flight")
		require.ErrorIs(t, err, errUtils.ErrBlobOperationFailed)
	case <-time.After(5 * time.Second):
		t.Fatal("the waiting probe stayed blocked after its context ended")
	}

	free()
	select {
	case err := <-first:
		require.ErrorIs(t, err, errEndpointDown, "the first probe still finishes and reports its own failure")
	case <-time.After(5 * time.Second):
		t.Fatal("the first probe never finished after the endpoint was released")
	}
}

func TestPutChecksContextBeforeReadingTheBody(t *testing.T) {
	api := NewMockAPI(gomock.NewController(t)) // Any SDK call fails the test: nothing may be sent.
	store, err := NewFromClient(api, "bucket", WithPartSize(smallPart))
	require.NoError(t, err)
	reader, writer := io.Pipe() // Nothing is ever written, so reading would block forever.
	defer writer.Close()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	done := make(chan error, 1)
	go func() {
		_, putErr := store.Put(ctx, "k", reader, blob.PutOptions{Size: blob.UnknownSize})
		done <- putErr
	}()
	select {
	case err := <-done:
		require.ErrorIs(t, err, context.Canceled)
		assert.ErrorIs(t, err, errUtils.ErrBlobOperationFailed)
	case <-time.After(5 * time.Second):
		t.Fatal("put blocked reading the body although its context was already canceled")
	}
}

func TestContextReaderStopsConsumingAfterCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	reader := &ctxReader{ctx: ctx, reader: bytes.NewReader([]byte("abcdef"))}
	buf := make([]byte, 3)
	n, err := reader.Read(buf)
	require.NoError(t, err)
	assert.Equal(t, "abc", string(buf[:n]))

	cancel()
	n, err = reader.Read(buf)
	require.ErrorIs(t, err, context.Canceled)
	assert.Zero(t, n)
}

func TestMergeUploadErrorsKeepsTheWorkerFailure(t *testing.T) {
	errService := errors.New("service rejected the part")
	errRead := errors.New("body read failed")
	live := context.Background()
	canceled, cancel := context.WithCancel(context.Background())
	cancel()

	tests := []struct {
		name       string
		parent     context.Context
		produceErr error
		waitErr    error
		want       error
	}{
		{name: "no failure", parent: live},
		{name: "worker failure surfaces when the producer was canceled by the group", parent: live, produceErr: context.Canceled, waitErr: errService, want: errService},
		{name: "caller cancellation stays the reported error", parent: canceled, produceErr: context.Canceled, waitErr: errService, want: context.Canceled},
		{name: "unrelated producer error is kept", parent: live, produceErr: errRead, waitErr: context.Canceled, want: errRead},
		{name: "worker failure alone is reported", parent: live, waitErr: errService, want: errService},
		{name: "producer failure alone is reported", parent: live, produceErr: errRead, want: errRead},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := mergeUploadErrors(tc.parent, tc.produceErr, tc.waitErr)
			if tc.want == nil {
				assert.NoError(t, got)
				return
			}
			assert.ErrorIs(t, got, tc.want)
		})
	}
}

func TestUnclassifiedFailuresCarryTheSentinel(t *testing.T) {
	cause := errors.New("disk full")
	err := asBlobError("put", "k", cause)
	require.ErrorIs(t, err, errUtils.ErrBlobOperationFailed)
	assert.ErrorIs(t, err, cause, "the original cause stays visible")
}
