package s3

import (
	"context"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/artifact"
	"github.com/cloudposse/atmos/pkg/artifact/blob/s3/s3test"
	"github.com/cloudposse/atmos/pkg/artifact/layout"
)

// An interruption before the commit never advertises partial success: the reference is unavailable,
// invisible to listing, and a same-intent retry repairs it completely.
func TestInterruptedPublication_RetryRepairs(t *testing.T) {
	points := []faultPoint{faultBeforePayload, faultDuringPayload, faultAfterMetadata}
	for _, spec := range allSpecs {
		for _, point := range points {
			t.Run(specName(spec)+"/"+string(point), func(t *testing.T) {
				spec = spec.fresh(t).withOpts(withFaults(crashAt(point)))
				repo := newRepo(t, spec)
				prepared := prepare(t, repo, &prepareSpec{files: bigFiles()})

				_, err := repo.Publish(context.Background(), prepared, artifact.PublishOptions{})
				require.ErrorIs(t, err, errCrash)

				_, err = repo.Resolve(context.Background(), defaultRef())
				require.ErrorIs(t, err, errUtils.ErrArtifactPublicationUnavailable, "a pending reference is unavailable to managed readers")
				require.ErrorContains(t, err, prepared.OperationID, "the diagnostic names the owning operation")
				assert.Empty(t, listNames(t, repo), "a pending reference is absent from search results")
				_, err = repo.Pin(context.Background(), defaultRef())
				require.ErrorIs(t, err, errUtils.ErrArtifactPublicationUnavailable)
				recordDigest, err := prepared.Record.RecordDigest()
				require.NoError(t, err)
				_, err = repo.Resolve(context.Background(), artifact.Ref{Name: testName, Digest: recordDigest})
				require.Error(t, err, "even the digest does not expose a pending record")

				retry, err := repo.Publish(context.Background(), prepared, artifact.PublishOptions{})
				require.NoError(t, err)
				assert.Equal(t, artifact.OutcomeCreated, retry.Outcome)
				assertIntact(t, repo, defaultRef())
				assert.Equal(t, []string{testName + ":" + testVersion}, listNames(t, repo))
				assert.Empty(t, managedKeys(spec.fake, "", layout.ManagedRoot+"/operations"), "the retry removes the journal")
			})
		}
	}
}

// A crash after the commit is already complete; only cleanup is left behind.
func TestInterruptedPublication_AfterCommitIsComplete(t *testing.T) {
	spec := noneSpec().fresh(t).withOpts(withFaults(crashAt(faultAfterCommit)))
	repo := newRepo(t, spec)
	_, err := repo.Publish(context.Background(), prepare(t, repo, &prepareSpec{}), artifact.PublishOptions{})
	require.ErrorIs(t, err, errCrash)
	assertIntact(t, repo, defaultRef())
	assert.Equal(t, []string{testName + ":" + testVersion}, listNames(t, repo))
}

// A pending operation of a different intent is never stolen, even under force.
func TestPendingOperation_IsNeverStolen(t *testing.T) {
	spec := noneSpec().fresh(t).withOpts(withFaults(crashAt(faultDuringPayload)))
	repo := newRepo(t, spec)
	first := prepare(t, repo, &prepareSpec{files: map[string]string{"data.txt": "first"}, opID: "op-first"})
	_, err := repo.Publish(context.Background(), first, artifact.PublishOptions{})
	require.ErrorIs(t, err, errCrash)

	other := prepare(t, repo, &prepareSpec{files: map[string]string{"data.txt": "second"}, opID: "op-second"})
	for _, force := range []bool{false, true} {
		_, err = repo.Publish(context.Background(), other, artifact.PublishOptions{Force: force})
		require.ErrorIs(t, err, errUtils.ErrArtifactPublicationUnavailable)
		require.ErrorContains(t, err, "op-first", "the owning operation ID is surfaced for recovery")
	}
	_, err = repo.Publish(context.Background(), first, artifact.PublishOptions{})
	require.NoError(t, err, "the original owner repairs it with the same intent")
}

// An interrupted force replacement leaves the reference unavailable until the same-intent retry
// repairs it, and never serves the replacement's bytes under the old digest.
func TestInterruptedForceReplacement(t *testing.T) {
	var fault armedFault
	for _, spec := range []*rspec{noneSpec(), archiveSpec(artifact.PackagingTgz)} {
		t.Run(specName(spec), func(t *testing.T) {
			spec = spec.fresh(t)
			repo := newRepo(t, spec.withOpts(fault.option()))
			base := publish(t, repo, &prepareSpec{files: map[string]string{"data.txt": "base", "gone.txt": "old"}}, artifact.PublishOptions{})
			replacement := prepare(t, repo, &prepareSpec{files: map[string]string{"data.txt": "replacement"}, opID: "op-replace"})

			fault.hook = crashAt(faultAfterMetadata)
			_, err := repo.Publish(context.Background(), replacement, artifact.PublishOptions{Force: true, ExpectedPrior: &base.Pinned})
			require.ErrorIs(t, err, errCrash)
			_, err = repo.Resolve(context.Background(), defaultRef())
			require.ErrorIs(t, err, errUtils.ErrArtifactPublicationUnavailable)
			_, err = repo.Read(context.Background(), base.Pinned)
			require.Error(t, err, "the old digest is not served while the replacement is unfinished")

			fault.hook = nil
			retry, err := repo.Publish(context.Background(), replacement, artifact.PublishOptions{Force: true, ExpectedPrior: &base.Pinned})
			require.NoError(t, err)
			assert.Equal(t, artifact.OutcomeReplaced, retry.Outcome)
			assertIntact(t, repo, defaultRef())
			_, err = repo.Read(context.Background(), base.Pinned)
			require.ErrorIs(t, err, errUtils.ErrArtifactNotFound, "the retired digest reports not found")
			if spec.packaging == artifact.PackagingNone {
				assert.NotContains(t, payloadKeys(spec.fake, ""), "team/app/1.0.0/gone.txt", "the replaced payload's extra file was retired")
			}
		})
	}
}

// A response lost after the request was sent is reconciled by reading the reference back; the
// publication succeeds exactly once and nothing is written twice with different intent.
func TestPublish_ReconcilesLostResponses(t *testing.T) {
	reference := refKey("")
	tests := []struct {
		name  string
		fault s3test.Fault
	}{
		{name: "pending write applied, response lost", fault: s3test.Fault{Op: s3test.OpPutObject, KeyPrefix: reference, Action: s3test.DropAfterCommit}},
		{name: "completed write applied, response lost", fault: s3test.Fault{Op: s3test.OpPutObject, KeyPrefix: reference, Skip: 1, Action: s3test.DropAfterCommit}},
		{name: "completed write never arrived", fault: s3test.Fault{Op: s3test.OpPutObject, KeyPrefix: reference, Skip: 1, Action: s3test.DropBeforeCommit}},
		{name: "pending write never arrived", fault: s3test.Fault{Op: s3test.OpPutObject, KeyPrefix: reference, Action: s3test.DropBeforeCommit}},
		{name: "payload write applied, response lost", fault: s3test.Fault{Op: s3test.OpPutObject, KeyPrefix: "team/app/1.0.0", Action: s3test.DropAfterCommit}},
	}
	for _, spec := range []*rspec{noneSpec(), archiveSpec(artifact.PackagingTgz)} {
		for _, tt := range tests {
			t.Run(specName(spec)+"/"+tt.name, func(t *testing.T) {
				spec = spec.fresh(t)
				repo := newRepo(t, spec)
				spec.fake.Inject(tt.fault)

				result, err := repo.Publish(context.Background(), prepare(t, repo, &prepareSpec{}), artifact.PublishOptions{})
				require.NoError(t, err)
				assert.Equal(t, artifact.OutcomeCreated, result.Outcome)
				assertIntact(t, repo, defaultRef())
				faulted := 0
				for _, request := range spec.fake.Requests() {
					if request.Faulted {
						faulted++
					}
				}
				assert.Positive(t, faulted, "the fault actually fired")
				assert.Equal(t, []string{testName + ":" + testVersion}, listNames(t, repo))
			})
		}
	}
}

// A conditional write whose response is lost and whose outcome was taken over by another writer is a
// conflict, not a success.
func TestPublish_LostResponseOverwrittenByAnotherWriterIsAConflict(t *testing.T) {
	spec := noneSpec().fresh(t)
	repo := newRepo(t, spec)
	spec.fake.Inject(s3test.Fault{Op: s3test.OpPutObject, KeyPrefix: refKey(""), Skip: 1, Action: s3test.DropBeforeCommit, Times: -1})
	// The request never arrives, every retry is dropped too, and the reference stays pending.
	_, err := repo.Publish(context.Background(), prepare(t, repo, &prepareSpec{}), artifact.PublishOptions{})
	require.Error(t, err)
	_, err = repo.Resolve(context.Background(), defaultRef())
	require.ErrorIs(t, err, errUtils.ErrArtifactPublicationUnavailable, "an unreconciled write leaves the reference pending, never half-published")
}

// Canceling an upload aborts the multipart upload the operation owns, and the same intent resumes.
func TestPublish_CancellationAbortsMultipartAndResumes(t *testing.T) {
	spec := noneSpec().fresh(t)
	repo := newRepo(t, spec)
	prepared := prepare(t, repo, &prepareSpec{files: bigFiles()})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	for index := range prepared.Blobs {
		original := prepared.Blobs[index].Open
		if prepared.Blobs[index].Path != "nested/big.bin" {
			continue
		}
		prepared.Blobs[index].Open = func(ctx context.Context) (io.ReadCloser, error) {
			reader, err := original(ctx)
			if err != nil {
				return nil, err
			}
			return &cancelingReader{ReadCloser: reader, after: 20 << 10, cancel: cancel}, nil
		}
	}

	_, err := repo.Publish(ctx, prepared, artifact.PublishOptions{})
	require.ErrorIs(t, err, context.Canceled)
	assert.Empty(t, spec.fake.OpenUploads(), "no multipart upload is left open")
	assert.GreaterOrEqual(t, spec.fake.AbortedUploads(), 1)
	_, err = repo.Resolve(context.Background(), defaultRef())
	require.ErrorIs(t, err, errUtils.ErrArtifactPublicationUnavailable)

	prepared = prepare(t, repo, &prepareSpec{files: bigFiles()})
	_, err = repo.Publish(context.Background(), prepared, artifact.PublishOptions{})
	require.NoError(t, err, "the same intent resumes after cancellation")
	assertIntact(t, repo, defaultRef())
	assert.Empty(t, managedKeys(spec.fake, "", layout.ManagedRoot+"/locks"), "no lock claim leaked")
}

// cancelingReader cancels a context once it has delivered a number of bytes.
type cancelingReader struct {
	io.ReadCloser
	after  int
	read   int
	cancel context.CancelFunc
}

func (c *cancelingReader) Read(p []byte) (int, error) {
	n, err := c.ReadCloser.Read(p)
	c.read += n
	if c.read >= c.after {
		c.cancel()
	}
	return n, err
}

// An identical republication is a no-op: no payload object is written again.
func TestRepublishIsUnchanged(t *testing.T) {
	for _, spec := range []*rspec{noneSpec(), archiveSpec(artifact.PackagingTgz)} {
		t.Run(specName(spec), func(t *testing.T) {
			spec = spec.fresh(t)
			repo := newRepo(t, spec)
			first := publish(t, repo, &prepareSpec{}, artifact.PublishOptions{})
			spec.fake.ResetRequests()

			again, err := repo.Publish(context.Background(), prepare(t, repo, &prepareSpec{opID: "op-again"}), artifact.PublishOptions{ExpectedPrior: &first.Pinned})
			require.NoError(t, err)
			assert.Equal(t, artifact.OutcomeUnchanged, again.Outcome)
			assert.Equal(t, first.Pinned.RecordDigest, again.Pinned.RecordDigest)
			for _, request := range mutations(spec.fake) {
				assert.Contains(t, request.Key, layout.ManagedRoot, "only managed lock claims and journals may change: %s %s", request.Op, request.Key)
				assert.NotContains(t, request.Key, "/refs/")
			}
		})
	}
}

// A publication rejected before any state change writes nothing.
func TestRejectedPublicationWritesNothing(t *testing.T) {
	spec := noneSpec().fresh(t)
	repo := newRepo(t, spec)
	prepared := prepare(t, repo, &prepareSpec{})
	prepared.Blobs = prepared.Blobs[:len(prepared.Blobs)-1]
	_, err := repo.Publish(context.Background(), prepared, artifact.PublishOptions{})
	require.Error(t, err)
	assert.Empty(t, spec.fake.Keys())
	assert.Empty(t, spec.fake.Requests())
}
