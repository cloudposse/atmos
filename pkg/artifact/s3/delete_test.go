package s3

import (
	"context"
	"testing"

	"github.com/opencontainers/go-digest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/artifact"
	"github.com/cloudposse/atmos/pkg/artifact/artifacttest"
	"github.com/cloudposse/atmos/pkg/artifact/blob"
	"github.com/cloudposse/atmos/pkg/artifact/blob/s3/s3test"
	"github.com/cloudposse/atmos/pkg/artifact/layout"
)

// Deletion is scoped to the selected artifact: its reference, Record, representation, evidence, and
// owned payload objects. Sibling versions and unmanaged objects stay.
func TestDelete_IsScopedToTheSelectedArtifact(t *testing.T) {
	for _, spec := range allSpecs {
		t.Run(specName(spec), func(t *testing.T) {
			spec = spec.fresh(t).withOpts(WithVerifier(&artifacttest.FakeVerifier{}))
			repo := newRepo(t, spec)
			publish(t, repo, &prepareSpec{version: "1.0.0", files: map[string]string{"data.txt": "keep"}}, artifact.PublishOptions{})
			drop := publish(t, repo, &prepareSpec{version: "2.0.0", files: map[string]string{"data.txt": "drop"}}, artifact.PublishOptions{})
			signer := newSigner()
			_, err := repo.Sign(context.Background(), recordSubject(t, repo, &drop.Pinned), signer, artifact.SignOptions{})
			require.NoError(t, err)
			spec.fake.Seed("unrelated/user-object.txt", []byte("mine"), nil)
			if spec.packaging == artifact.PackagingNone {
				spec.fake.Seed("team/app/2.0.0/extra-user-file.txt", []byte("also mine"), nil)
			}
			spec.fake.ResetRequests()

			result, err := repo.Delete(context.Background(), drop.Pinned, artifact.DeleteOptions{ExpectedCurrent: &drop.Pinned, Scope: artifact.DeleteObject})
			require.NoError(t, err)
			assert.NotEmpty(t, result.Deleted)
			assert.Equal(t, drop.Pinned.RecordDigest, result.Digest)

			_, err = repo.Resolve(context.Background(), ref(testName, "2.0.0"))
			require.ErrorIs(t, err, errUtils.ErrArtifactNotFound)
			assertIntact(t, repo, ref(testName, "1.0.0"))
			assert.Equal(t, []string{testName + ":1.0.0"}, listNames(t, repo))
			_, ok := spec.fake.Object("unrelated/user-object.txt")
			assert.True(t, ok, "objects outside the artifact are untouched")
			for _, key := range bucketKeys(spec.fake) {
				assert.NotContains(t, key, drop.Pinned.RecordDigest.Encoded(), "the Record and its evidence are gone")
			}
			if spec.packaging == artifact.PackagingNone {
				_, ok = spec.fake.Object("team/app/2.0.0/extra-user-file.txt")
				assert.True(t, ok, "an unmanaged object inside the payload prefix stays")
			}

			deletes := spec.fake.RequestsFor(s3test.OpDeleteObject)
			require.NotEmpty(t, deletes)
			assert.Equal(t, layout.ReferencePath(layout.ReferenceKey(testName, "2.0.0")), deletes[0].Key, "the reference is removed first")
			assert.NotEmpty(t, deletes[0].Header.Get("If-Match"), "and conditionally on the observed ETag")
		})
	}
}

func TestDelete_RejectsStaleTargetsScopesAndPendingPublications(t *testing.T) {
	spec := noneSpec().fresh(t)
	var fault armedFault
	repo := newRepo(t, spec.withOpts(fault.option()))
	done := publish(t, repo, &prepareSpec{}, artifact.PublishOptions{})
	before := bucketKeys(spec.fake)

	stale := done.Pinned
	stale.ResolvedDigest = digest.FromString("stale")
	_, err := repo.Delete(context.Background(), done.Pinned, artifact.DeleteOptions{ExpectedCurrent: &stale})
	require.ErrorIs(t, err, errUtils.ErrArtifactConflict)

	_, err = repo.Delete(context.Background(), done.Pinned, artifact.DeleteOptions{Scope: artifact.DeleteReference})
	require.ErrorIs(t, err, errUtils.ErrArtifactUnsupportedCapability)
	assert.Equal(t, before, bucketKeys(spec.fake), "rejected deletes changed nothing")

	fault.hook = crashAt(faultAfterMetadata)
	_, err = repo.Publish(context.Background(), prepare(t, repo, &prepareSpec{files: map[string]string{"data.txt": "changed"}, opID: "op-pending"}),
		artifact.PublishOptions{Force: true, ExpectedPrior: &done.Pinned})
	require.ErrorIs(t, err, errCrash)
	_, err = repo.Delete(context.Background(), done.Pinned, artifact.DeleteOptions{})
	require.ErrorIs(t, err, errUtils.ErrArtifactPublicationUnavailable, "a pending publication is never deleted; its owner repairs it")
}

// The reference removal is conditional: a reference that moved on is a failed delete, not a removal
// of someone else's state.
func TestDelete_ReferenceRemovalIsConditional(t *testing.T) {
	spec := noneSpec().fresh(t)
	repo := newRepo(t, spec)
	publish(t, repo, &prepareSpec{}, artifact.PublishOptions{})
	state, err := repo.loadReference(context.Background(), layout.ReferenceKey(testName, testVersion))
	require.NoError(t, err)
	require.NotNil(t, state)

	err = repo.removeReference(context.Background(), &refState{ref: state.ref, etag: `"not-the-current-etag"`})
	require.ErrorIs(t, err, errUtils.ErrArtifactDeleteFailed)
	require.True(t, blob.IsPreconditionFailed(err))
	assertIntact(t, repo, defaultRef())

	require.NoError(t, repo.removeReference(context.Background(), state))
	_, err = repo.Resolve(context.Background(), defaultRef())
	require.ErrorIs(t, err, errUtils.ErrArtifactNotFound)
}

// After a delete the reference can be published again, even if cleanup left leftovers behind.
func TestDelete_ThenRepublish(t *testing.T) {
	spec := noneSpec().fresh(t)
	repo := newRepo(t, spec)
	first := publish(t, repo, &prepareSpec{}, artifact.PublishOptions{})
	_, err := repo.Delete(context.Background(), first.Pinned, artifact.DeleteOptions{})
	require.NoError(t, err)
	assert.Empty(t, payloadKeys(spec.fake, ""))
	assert.Empty(t, managedKeys(spec.fake, "", layout.ManagedRoot+"/refs"))

	again := publish(t, repo, &prepareSpec{opID: "op-again"}, artifact.PublishOptions{})
	assert.Equal(t, artifact.OutcomeCreated, again.Outcome)
	assertIntact(t, repo, defaultRef())
}
