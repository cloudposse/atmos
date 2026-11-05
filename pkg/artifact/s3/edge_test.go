package s3

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/opencontainers/go-digest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/artifact"
	"github.com/cloudposse/atmos/pkg/artifact/artifacttest"
	"github.com/cloudposse/atmos/pkg/artifact/blob/s3/s3test"
	"github.com/cloudposse/atmos/pkg/artifact/layout"
)

// A same-intent retry under a different packaging cannot reuse the journaled location (its
// extension would disagree), so it publishes at a freshly rendered location and retires the
// abandoned attempt's partial payload instead of orphaning it.
func TestResumedPublicationRetiresAnAbandonedLocation(t *testing.T) {
	tests := []struct {
		name      string
		crashed   *rspec
		retry     *rspec
		abandoned []string
		wantKeys  []string
	}{
		{
			name: "tgz to zip", crashed: archiveSpec(artifact.PackagingTgz), retry: archiveSpec(artifact.PackagingZip),
			abandoned: []string{"team/app/1.0.0.tgz", "team/app/1.0.0.tgz.metadata.json"},
			wantKeys:  []string{"team/app/1.0.0.zip", "team/app/1.0.0.zip.docs", "team/app/1.0.0.zip.metadata.json"},
		},
		{
			name: "archive to files", crashed: archiveSpec(artifact.PackagingTgz), retry: noneSpec(),
			abandoned: []string{"team/app/1.0.0.tgz", "team/app/1.0.0.tgz.metadata.json"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fake := s3test.NewServer(t)
			crashed := newRepo(t, tt.crashed.with(func(s *rspec) { s.fake = fake }).withOpts(withFaults(crashAt(faultAfterMetadata))))
			_, err := crashed.Publish(context.Background(), prepare(t, crashed, &prepareSpec{opID: "op-first"}), artifact.PublishOptions{})
			require.ErrorIs(t, err, errCrash)
			for _, key := range tt.abandoned {
				_, ok := fake.Object(key)
				require.True(t, ok, "the crashed attempt left %s behind", key)
			}

			retry := newRepo(t, tt.retry.with(func(s *rspec) { s.fake = fake }))
			_, err = retry.Publish(context.Background(), prepare(t, retry, &prepareSpec{opID: "op-retry"}), artifact.PublishOptions{})
			require.NoError(t, err, "the same intent repairs the pending reference whatever the packaging")
			for _, key := range tt.abandoned {
				_, ok := fake.Object(key)
				assert.False(t, ok, "the abandoned object %s was retired, not orphaned", key)
			}
			assertIntact(t, retry, defaultRef())
			assert.Empty(t, managedKeys(fake, "", layout.ManagedRoot+"/operations"))
		})
	}
}

// A Record object that already exists with different bytes is corruption, never replaced.
func TestPublish_ExistingRecordWithDifferentBytesIsRejected(t *testing.T) {
	spec := noneSpec().fresh(t)
	repo := newRepo(t, spec)
	prepared := prepare(t, repo, &prepareSpec{})
	recordDigest, err := prepared.Record.RecordDigest()
	require.NoError(t, err)
	spec.fake.Seed(layout.RecordPath(recordDigest), []byte("squatter"), nil)

	_, err = repo.Publish(context.Background(), prepared, artifact.PublishOptions{})
	require.ErrorIs(t, err, errUtils.ErrArtifactIntegrityFailed)
	object, ok := spec.fake.Object(layout.RecordPath(recordDigest))
	require.True(t, ok)
	assert.Equal(t, []byte("squatter"), object.Data, "the existing object is left as found")
	_, err = repo.Resolve(context.Background(), defaultRef())
	require.ErrorIs(t, err, errUtils.ErrArtifactPublicationUnavailable, "the reference never completed")
}

// Managed documents that disagree with where they live are corruption.
func TestTamperedManagedDocuments(t *testing.T) {
	setup := func(t *testing.T) (*Repository, *s3test.Server, artifact.PinnedReference) {
		repo, spec, pinned := setupTampering(t)
		_ = spec
		return repo, spec.fake, pinned
	}
	otherRecord := func(t *testing.T, repo *Repository) digest.Digest {
		return publish(t, repo, &prepareSpec{name: "other/app"}, artifact.PublishOptions{}).Pinned.RecordDigest
	}
	t.Run("a reference naming another artifact's record", func(t *testing.T) {
		repo, fake, _ := setup(t)
		other := otherRecord(t, repo)
		state, err := repo.loadReference(context.Background(), layout.ReferenceKey(testName, testVersion))
		require.NoError(t, err)
		forged := *state.ref
		forged.RecordDigest = other
		raw, err := forged.Marshal()
		require.NoError(t, err)
		fake.Seed(layout.ReferencePath(forged.Key()), raw, nil)
		_, err = repo.Resolve(context.Background(), defaultRef())
		require.ErrorIs(t, err, errUtils.ErrArtifactIntegrityFailed)
	})
	t.Run("a reference stored under another reference's key", func(t *testing.T) {
		repo, fake, _ := setup(t)
		otherRecord(t, repo)
		state, err := repo.loadReference(context.Background(), layout.ReferenceKey("other/app", testVersion))
		require.NoError(t, err)
		raw, err := state.ref.Marshal()
		require.NoError(t, err)
		fake.Seed(layout.ReferencePath(layout.ReferenceKey(testName, testVersion)), raw, nil)
		_, err = repo.Resolve(context.Background(), defaultRef())
		require.ErrorIs(t, err, errUtils.ErrArtifactIntegrityFailed)
	})
	t.Run("a representation stored under another reference's key", func(t *testing.T) {
		repo, fake, pinned := setup(t)
		otherRecord(t, repo)
		rep, err := repo.loadRepresentation(context.Background(), layout.ReferenceKey("other/app", testVersion))
		require.NoError(t, err)
		raw, err := rep.Marshal()
		require.NoError(t, err)
		fake.Seed(layout.RepresentationPath(layout.ReferenceKey(testName, testVersion)), raw, nil)
		_, err = repo.Inspect(context.Background(), pinned)
		require.ErrorIs(t, err, errUtils.ErrArtifactIntegrityFailed)
	})
	t.Run("an archive representation with two payloads", func(t *testing.T) {
		repo, fake, pinned := setup(t)
		rep, err := repo.loadRepresentation(context.Background(), layout.ReferenceKey(testName, testVersion))
		require.NoError(t, err)
		rep.Payload = append(rep.Payload, rep.Payload[0])
		raw, err := rep.Marshal()
		require.NoError(t, err)
		fake.Seed(layout.RepresentationPath(layout.ReferenceKey(testName, testVersion)), raw, nil)
		_, err = repo.Read(context.Background(), pinned)
		require.ErrorIs(t, err, errUtils.ErrArtifactIntegrityFailed)
	})
	t.Run("an operation journal describing another operation", func(t *testing.T) {
		repo, spec, _ := setupTampering(t)
		fake := spec.fake
		other := publishPending(t, spec, "op-real")
		raw, _, err := repo.readObject(context.Background(), layout.OperationPath(other), layout.MaxDocumentBytes)
		require.NoError(t, err)
		require.NotNil(t, raw)
		fake.Seed(layout.OperationPath("op-claimed"), raw, nil)
		_, err = repo.loadOperation(context.Background(), "op-claimed")
		require.ErrorIs(t, err, errUtils.ErrArtifactIntegrityFailed)
	})
	t.Run("an oversize reference", func(t *testing.T) {
		repo, fake, _ := setup(t)
		fake.Seed(layout.ReferencePath(layout.ReferenceKey(testName, testVersion)), bytes.Repeat([]byte("x"), int(layout.MaxDocumentBytes)+1), nil)
		_, err := repo.Resolve(context.Background(), defaultRef())
		require.ErrorIs(t, err, errUtils.ErrArtifactTooLarge)
	})
}

func setupTampering(t *testing.T) (*Repository, *rspec, artifact.PinnedReference) {
	t.Helper()
	spec := archiveSpec(artifact.PackagingTgz).fresh(t)
	repo := newRepo(t, spec)
	return repo, spec, publish(t, repo, &prepareSpec{}, artifact.PublishOptions{}).Pinned
}

// publishPending leaves a crashed publication behind and returns its operation ID.
func publishPending(t *testing.T, spec *rspec, opID string) string {
	t.Helper()
	crashed := newRepo(t, spec.withOpts(withFaults(crashAt(faultAfterMetadata))))
	_, err := crashed.Publish(context.Background(), prepare(t, crashed, &prepareSpec{name: "pending/app", opID: opID}), artifact.PublishOptions{})
	require.ErrorIs(t, err, errCrash)
	return opID
}

// A digest reference to a Record whose reference was deleted is not found.
func TestResolveByDigest_ReferenceDeletedUnderneath(t *testing.T) {
	spec := noneSpec().fresh(t)
	repo := newRepo(t, spec)
	pinned := publish(t, repo, &prepareSpec{}, artifact.PublishOptions{}).Pinned
	require.NoError(t, repo.deleteObject(context.Background(), layout.ReferencePath(layout.ReferenceKey(testName, testVersion))))
	_, err := repo.Resolve(context.Background(), artifact.Ref{Name: testName, Digest: pinned.RecordDigest})
	require.ErrorIs(t, err, errUtils.ErrArtifactNotFound)
	require.ErrorContains(t, err, "reference was deleted")
}

// The generation recheck reports a reference that vanished while its bytes were read.
func TestRead_GuardReportsAVanishedReference(t *testing.T) {
	spec := noneSpec().fresh(t)
	repo := newRepo(t, spec)
	pinned := publish(t, repo, &prepareSpec{}, artifact.PublishOptions{}).Pinned
	content, err := repo.Read(context.Background(), pinned)
	require.NoError(t, err)
	defer content.Close()
	require.NoError(t, repo.deleteObject(context.Background(), layout.ReferencePath(layout.ReferenceKey(testName, testVersion))))
	require.ErrorIs(t, readAll(t, content), errUtils.ErrArtifactNotFound)
}

// A failure while deleting payload objects leaves the reference gone and garbage behind, never a
// reference that points at missing bytes, and a republish clears the garbage.
func TestDelete_PartialFailureLeavesGarbageNotADanglingReference(t *testing.T) {
	spec := noneSpec().fresh(t)
	repo := newRepo(t, spec)
	pinned := publish(t, repo, &prepareSpec{files: map[string]string{"a.txt": "a", "b.txt": "b"}}, artifact.PublishOptions{}).Pinned
	spec.fake.Inject(s3test.Fault{Op: s3test.OpDeleteObject, KeyPrefix: "team/app/1.0.0/b.txt", Status: 403, Code: "AccessDenied", Times: -1})

	_, err := repo.Delete(context.Background(), pinned, artifact.DeleteOptions{})
	require.ErrorIs(t, err, errUtils.ErrArtifactDeleteFailed)
	_, err = repo.Resolve(context.Background(), defaultRef())
	require.ErrorIs(t, err, errUtils.ErrArtifactNotFound, "the reference was removed first")

	spec.fake.ClearFaults()
	again := publish(t, repo, &prepareSpec{files: map[string]string{"a.txt": "a2", "b.txt": "b2"}, opID: "op-again"}, artifact.PublishOptions{})
	assert.Equal(t, artifact.OutcomeCreated, again.Outcome, "leftover payload objects are still owned by the reference's representation")
	assertIntact(t, repo, defaultRef())
}

// A lost response to the reference delete is settled by reading the reference.
func TestDelete_LostResponseIsReconciled(t *testing.T) {
	spec := noneSpec().fresh(t)
	repo := newRepo(t, spec)
	pinned := publish(t, repo, &prepareSpec{}, artifact.PublishOptions{}).Pinned
	spec.fake.Inject(s3test.Fault{Op: s3test.OpDeleteObject, KeyPrefix: layout.ReferencePath(layout.ReferenceKey(testName, testVersion)), Action: s3test.DropAfterCommit})
	_, err := repo.Delete(context.Background(), pinned, artifact.DeleteOptions{})
	require.NoError(t, err)
	_, err = repo.Resolve(context.Background(), defaultRef())
	require.ErrorIs(t, err, errUtils.ErrArtifactNotFound)
}

// A create that never reached the endpoint is retried transparently.
func TestLock_CreateThatNeverArrivedIsRetried(t *testing.T) {
	spec := noneSpec().fresh(t)
	repo := newRepo(t, spec)
	spec.fake.Inject(s3test.Fault{Op: s3test.OpPutObject, KeyPrefix: layout.LockPath(lockResource), Action: s3test.DropBeforeCommit})
	handle, err := repo.Lock(context.Background(), artifact.LockRequest{Resource: lockResource, Owner: "owner-a"})
	require.NoError(t, err)
	require.NoError(t, handle.Release(context.Background()))
}

// More evidence than the read bound is reported, not silently dropped.
func TestVerify_BoundsTheNumberOfEvidenceObjectsRead(t *testing.T) {
	spec := signedSpec(t, noneSpec())
	repo := newRepo(t, spec)
	pinned := publish(t, repo, &prepareSpec{}, artifact.PublishOptions{}).Pinned
	subject := recordSubject(t, repo, &pinned)
	for index := 0; index < maxEvidenceFiles+3; index++ {
		stored, err := layout.NewEvidence(subject, &artifact.EvidencePayload{Format: "fake", MediaType: "application/json", Data: []byte(strings.Repeat("x", index+1))})
		require.NoError(t, err)
		raw, err := stored.Marshal()
		require.NoError(t, err)
		require.NoError(t, repo.putBytes(context.Background(), layout.SignaturePath(subject.Kind, subject.Digest, stored.Digest()), raw))
	}
	loaded, diagnostics, err := repo.loadEvidence(context.Background(), subject.Digest)
	require.NoError(t, err)
	assert.Len(t, loaded, maxEvidenceFiles)
	require.Len(t, diagnostics, 1)
	assert.Equal(t, "evidence-truncated", diagnostics[0].Code)
}

func TestSign_RejectsOversizeEvidence(t *testing.T) {
	repo := newRepo(t, signedSpec(t, noneSpec()))
	pinned := publish(t, repo, &prepareSpec{}, artifact.PublishOptions{}).Pinned
	huge := &oversizeSigner{}
	_, err := repo.Sign(context.Background(), recordSubject(t, repo, &pinned), huge, artifact.SignOptions{})
	require.ErrorIs(t, err, errUtils.ErrArtifactTooLarge)
}

// oversizeSigner returns evidence past the bounded-read limit.
type oversizeSigner struct{}

func (*oversizeSigner) Formats() []artifact.EvidenceFormat {
	return []artifact.EvidenceFormat{artifacttest.FakeEvidenceFormat}
}

func (*oversizeSigner) Sign(context.Context, artifact.SignatureSubject) (*artifact.EvidencePayload, error) {
	return &artifact.EvidencePayload{Format: artifacttest.FakeEvidenceFormat, MediaType: "application/json", Data: make([]byte, artifact.MaxEvidenceSize+1)}, nil
}

// The configured owner is recorded in claims and journals, and a part size reaches the SDK store.
func TestOptions_OwnerAndPartSize(t *testing.T) {
	isolateAWS(t)
	fake := s3test.NewServer(t)
	cfg := s3Config(map[string]any{"bucket": fake.Bucket(), "endpoint": fake.Endpoint(), "region": "us-east-1"})
	repo, err := New("owned", cfg, WithOwner("ci-runner-7"), WithPartSize(smallPart), WithConcurrency(2), WithLockWait(time.Second))
	require.NoError(t, err)
	handle, err := repo.Lock(context.Background(), artifact.LockRequest{Resource: lockResource})
	require.NoError(t, err)
	assert.Equal(t, "ci-runner-7", handle.Owner(), "an unset request owner falls back to the configured one")
	require.NoError(t, handle.Release(context.Background()))

	publish(t, repo, &prepareSpec{files: bigFiles()}, artifact.PublishOptions{})
	assert.GreaterOrEqual(t, fake.CompletedUploads(), 1, "the configured part size forced multipart uploads")
	assert.NotEmpty(t, defaultOwner())
}
