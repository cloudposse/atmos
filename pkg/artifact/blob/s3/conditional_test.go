package s3

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/artifact/blob"
	"github.com/cloudposse/atmos/pkg/artifact/blob/s3/s3test"
)

func putIfAbsent(s *Store, key, body string) (*blob.ObjectInfo, error) {
	return s.Put(context.Background(), key, bytes.NewReader([]byte(body)), blob.PutOptions{Size: int64(len(body)), Condition: blob.IfAbsent()})
}

func TestConditionalHeadersAreSent(t *testing.T) {
	fake := newFake(t)
	s := newTestStore(t, fake)

	first, err := putIfAbsent(s, "lock", "a")
	require.NoError(t, err)
	_, err = s.Put(context.Background(), "lock", bytes.NewReader([]byte("b")), blob.PutOptions{Size: 1, Condition: blob.IfMatch(first.ETag)})
	require.NoError(t, err)

	puts := fake.RequestsFor(s3test.OpPutObject)
	require.Len(t, puts, 2)
	assert.Equal(t, "*", puts[0].Header.Get("If-None-Match"))
	assert.Empty(t, puts[0].Header.Get("If-Match"))
	assert.Equal(t, first.ETag, puts[1].Header.Get("If-Match"))
	assert.Empty(t, puts[1].Header.Get("If-None-Match"))
}

func TestConditionalDeleteSendsIfMatch(t *testing.T) {
	fake := newFake(t)
	s := newTestStore(t, fake)
	info := putBytes(t, s, "k", []byte("x"), blob.PutOptions{})

	err := s.Delete(context.Background(), "k", blob.DeleteOptions{IfMatch: `"stale"`})
	assert.True(t, blob.IsPreconditionFailed(err), "%v", err)
	require.NoError(t, s.Delete(context.Background(), "k", blob.DeleteOptions{IfMatch: info.ETag}))

	deletes := fake.RequestsFor(s3test.OpDeleteObject)
	require.Len(t, deletes, 2)
	assert.Equal(t, `"stale"`, deletes[0].Header.Get("If-Match"))
	assert.Equal(t, info.ETag, deletes[1].Header.Get("If-Match"))
}

func TestEndpointThatCannotEnforceConditionsFailsClearly(t *testing.T) {
	fake := newFake(t, s3test.WithConditionalBehavior(s3test.ConditionalNotImplemented))
	s := newTestStore(t, fake)
	putBytes(t, s, "existing", []byte("keep"), blob.PutOptions{})

	_, err := putIfAbsent(s, "lock", "a")
	require.Error(t, err)
	assert.True(t, blob.IsConditionalUnsupported(err), "501 maps to a clear unsupported-condition error: %v", err)
	assert.False(t, blob.IsAmbiguous(err))
	_, ok := fake.Object("lock")
	assert.False(t, ok, "nothing was written")

	_, err = s.Put(context.Background(), "existing", bytes.NewReader([]byte("new")), blob.PutOptions{Size: 3, Condition: blob.IfMatch(`"x"`)})
	assert.True(t, blob.IsConditionalUnsupported(err), "%v", err)
	object, _ := fake.Object("existing")
	assert.Equal(t, []byte("keep"), object.Data, "the existing object was not overwritten")
}

func TestEndpointRejectingTheHeaderWithBadRequestIsUnsupported(t *testing.T) {
	fake := newFake(t)
	fake.Inject(s3test.Fault{Op: s3test.OpPutObject, Status: http.StatusBadRequest, Code: "InvalidArgument", Times: -1})
	s := newTestStore(t, fake)
	_, err := putIfAbsent(s, "lock", "a")
	assert.True(t, blob.IsConditionalUnsupported(err), "%v", err)

	// The same rejection of an unconditional put is a plain failure, not a capability problem.
	_, err = s.Put(context.Background(), "plain", bytes.NewReader([]byte("x")), blob.PutOptions{Size: 1})
	require.Error(t, err)
	assert.False(t, blob.IsConditionalUnsupported(err))
}

func TestDisabledModeNeverSendsConditions(t *testing.T) {
	fake := newFake(t)
	s := newTestStore(t, fake, WithConditionalMode(ConditionalDisabled))

	_, err := putIfAbsent(s, "lock", "a")
	assert.True(t, blob.IsConditionalUnsupported(err), "%v", err)
	err = s.Delete(context.Background(), "lock", blob.DeleteOptions{IfMatch: `"x"`})
	assert.True(t, blob.IsConditionalUnsupported(err), "%v", err)
	assert.Empty(t, fake.Requests(), "an endpoint known not to enforce conditions is never asked to")

	putBytes(t, s, "plain", []byte("x"), blob.PutOptions{})
	assert.Len(t, fake.Requests(), 1, "unconditional writes still work")
}

func TestProbeDetectsEndpointThatIgnoresConditions(t *testing.T) {
	fake := newFake(t, s3test.WithConditionalBehavior(s3test.ConditionalIgnore))
	s := newTestStore(t, fake, WithConditionalMode(ConditionalProbe))
	fake.Seed("lock", []byte("owner-a"), nil)

	_, err := putIfAbsent(s, "lock", "owner-b")
	require.Error(t, err)
	assert.True(t, blob.IsConditionalUnsupported(err), "a silently-ignoring endpoint must not be trusted: %v", err)
	object, _ := fake.Object("lock")
	assert.Equal(t, []byte("owner-a"), object.Data, "the caller's object was never touched")
	assert.Len(t, fake.Keys(), 1, "probe objects are removed")

	before := len(fake.Requests())
	_, err = putIfAbsent(s, "lock", "owner-c")
	assert.True(t, blob.IsConditionalUnsupported(err))
	assert.Len(t, fake.Requests(), before, "a definite probe verdict is cached")
}

func TestProbeAcceptsEnforcingEndpointOnce(t *testing.T) {
	fake := newFake(t)
	s := newTestStore(t, fake, WithConditionalMode(ConditionalProbe))

	_, err := putIfAbsent(s, "lock", "a")
	require.NoError(t, err)
	_, err = putIfAbsent(s, "lock2", "a")
	require.NoError(t, err)

	puts := fake.RequestsFor(s3test.OpPutObject)
	assert.Len(t, puts, 4, "two probe creates plus the two caller puts; the probe runs once")
	assert.Equal(t, []string{"lock", "lock2"}, fake.Keys(), "the probe object was cleaned up")
	assert.Equal(t, 1, countPrefix(puts, probePrefix)/2, "exactly one probe key was created twice")

	require.NoError(t, s.ProbeConditionalWrites(context.Background()))
	assert.Len(t, fake.RequestsFor(s3test.OpPutObject), 4, "explicit probe reuses the cached verdict")
}

func countPrefix(requests []s3test.Request, prefix string) int {
	count := 0
	for _, req := range requests {
		if len(req.Key) >= len(prefix) && req.Key[:len(prefix)] == prefix {
			count++
		}
	}
	return count
}

func TestResponseDroppedAfterCommitIsAmbiguousAndNotRetried(t *testing.T) {
	fake := newFake(t)
	fake.Inject(s3test.Fault{Op: s3test.OpPutObject, Action: s3test.DropAfterCommit})
	s := newTestStore(t, fake)

	_, err := putIfAbsent(s, "lock", "mine")
	require.Error(t, err)
	assert.True(t, blob.IsAmbiguous(err), "%v", err)
	var blobErr *blob.Error
	require.True(t, errors.As(err, &blobErr))
	assert.True(t, blobErr.Temporary(), "ambiguity is signaled through Temporary()")
	assert.False(t, blob.IsPreconditionFailed(err), "an SDK retry must not turn our own commit into a fake conflict")

	assert.Len(t, fake.RequestsFor(s3test.OpPutObject), 1, "conditional requests are never retried by the SDK")
	object, ok := fake.Object("lock")
	require.True(t, ok, "the write did commit")
	assert.Equal(t, []byte("mine"), object.Data)

	// Reconciliation: reading the object back proves ownership.
	stat, statErr := s.Stat(context.Background(), "lock", blob.StatOptions{})
	require.NoError(t, statErr)
	assert.Equal(t, object.ETag, stat.ETag)
}

func TestResponseDroppedBeforeCommitIsAlsoAmbiguous(t *testing.T) {
	fake := newFake(t)
	fake.Inject(s3test.Fault{Op: s3test.OpPutObject, Action: s3test.DropBeforeCommit})
	s := newTestStore(t, fake)
	_, err := putIfAbsent(s, "lock", "mine")
	assert.True(t, blob.IsAmbiguous(err), "the client cannot tell the two apart: %v", err)
	_, ok := fake.Object("lock")
	assert.False(t, ok)
}

func TestServerErrorOnConditionalMutationIsAmbiguous(t *testing.T) {
	fake := newFake(t)
	fake.Inject(s3test.Fault{Op: s3test.OpPutObject, Status: http.StatusInternalServerError, Code: "InternalError"})
	s := newTestStore(t, fake)
	_, err := putIfAbsent(s, "lock", "a")
	assert.True(t, blob.IsAmbiguous(err), "%v", err)
	assert.Len(t, fake.RequestsFor(s3test.OpPutObject), 1)
}

func TestThrottlingIsNotAmbiguous(t *testing.T) {
	fake := newFake(t)
	fake.Inject(s3test.Fault{Op: s3test.OpPutObject, Status: http.StatusServiceUnavailable, Code: "SlowDown", Times: -1})
	s := newTestStore(t, fake)
	_, err := putIfAbsent(s, "lock", "a")
	require.Error(t, err)
	assert.False(t, blob.IsAmbiguous(err), "503 means the request was rejected, not half applied")
}

func TestUnconditionalPutRetriesTransientServerError(t *testing.T) {
	fake := newFake(t)
	fake.Inject(s3test.Fault{Op: s3test.OpPutObject, Status: http.StatusInternalServerError, Code: "InternalError"})
	s := newTestStore(t, fake)
	putBytes(t, s, "k", []byte("x"), blob.PutOptions{})
	assert.Len(t, fake.RequestsFor(s3test.OpPutObject), 2, "the SDK retried once and succeeded")
}

func TestConnectionFailureBeforeSendingIsNotAmbiguous(t *testing.T) {
	fake := newFake(t)
	s := newTestStore(t, fake)
	fake.Close()
	_, err := putIfAbsent(s, "lock", "a")
	require.Error(t, err)
	assert.False(t, blob.IsAmbiguous(err), "the request never reached a server: %v", err)
}

func TestAmbiguousCompletionLeavesTheUploadForReconciliation(t *testing.T) {
	fake := newFake(t)
	fake.Inject(s3test.Fault{Op: s3test.OpCompleteMultipartUpload, Action: s3test.DropAfterCommit})
	s := newTestStore(t, fake)
	data := patterned(smallPart*2 + 9)

	_, err := s.Put(context.Background(), "big", bytes.NewReader(data), blob.PutOptions{Size: int64(len(data)), Condition: blob.IfAbsent()})
	require.Error(t, err)
	assert.True(t, blob.IsAmbiguous(err), "%v", err)
	assert.Empty(t, fake.RequestsFor(s3test.OpAbortMultipartUpload), "an upload that may have completed is never aborted")
	object, ok := fake.Object("big")
	require.True(t, ok)
	assert.Equal(t, data, object.Data)
}

func TestServiceCodesMapToKinds(t *testing.T) {
	cases := []struct {
		name   string
		op     string
		status int32
		code   string
		check  func(error) bool
	}{
		{"missing key", s3test.OpGetObject, 404, "NoSuchKey", blob.IsNotFound},
		{"conflicting concurrent write", s3test.OpPutObject, 409, "ConditionalRequestConflict", blob.IsPreconditionFailed},
		{"explicit precondition", s3test.OpPutObject, 412, "PreconditionFailed", blob.IsPreconditionFailed},
		{"access denied is plain", s3test.OpGetObject, 403, "AccessDenied", func(err error) bool {
			return !blob.IsNotFound(err) && !blob.IsPreconditionFailed(err) && !blob.IsAmbiguous(err)
		}},
		{"missing bucket is not a missing object", s3test.OpGetObject, 404, "NoSuchBucket", func(err error) bool { return !blob.IsNotFound(err) }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fake := newFake(t)
			fake.Inject(s3test.Fault{Op: tc.op, Status: tc.status, Code: tc.code, Times: -1})
			s := newTestStore(t, fake)
			var err error
			if tc.op == s3test.OpGetObject {
				_, err = s.Get(context.Background(), "k", blob.GetOptions{})
			} else {
				_, err = putIfAbsent(s, "k", "x")
			}
			require.Error(t, err)
			assert.True(t, tc.check(err), "%v", err)
		})
	}
}
