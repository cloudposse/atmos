package oci

import (
	"context"
	"io"
	"path/filepath"
	"testing"

	"github.com/opencontainers/go-digest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/artifact"
	"github.com/cloudposse/atmos/pkg/artifact/materialize"
)

// backends runs a test against the registry and the layout repository kinds.
func backends(t *testing.T, run func(t *testing.T, repo *Repository)) {
	t.Helper()
	t.Run("registry", func(t *testing.T) { run(t, newRegistryRepo(t, newRegistry(t, nil))) })
	t.Run("layout", func(t *testing.T) { run(t, newLayoutRepo(t)) })
}

func TestAtmosPackageRoundTrip(t *testing.T) {
	backends(t, func(t *testing.T, repo *Repository) {
		ctx := context.Background()
		result := publish(t, repo, publishSpec{content: "round trip"})
		assert.Equal(t, artifact.OutcomeCreated, result.Outcome)
		assert.NotEmpty(t, result.Location)

		pinned, err := repo.Resolve(ctx, artifact.Ref{Name: appName, Version: appVersion})
		require.NoError(t, err)
		assert.Equal(t, result.Pinned.ResolvedDigest, pinned.ResolvedDigest)
		assert.Equal(t, result.Pinned.RecordDigest, pinned.RecordDigest)

		inspection, err := repo.Inspect(ctx, pinned)
		require.NoError(t, err)
		require.NotNil(t, inspection.Record)
		assert.Nil(t, inspection.Representation.OriginalRoot, "an Atmos package is signed through its Record, not as a native root")
		assert.Equal(t, artifact.PackagingTgz, inspection.Representation.Packaging)
		assert.Equal(t, result.Pinned.ContentDigest, inspection.Pinned.ContentDigest)
		assert.Len(t, inspection.Representation.Documents, 2)

		content, err := repo.Read(ctx, pinned)
		require.NoError(t, err)
		defer content.Close()
		out := filepath.Join(t.TempDir(), "out")
		require.NoError(t, materialize.Materialize(ctx, content, out))
		state, err := materialize.InspectDestination(out, content.Record)
		require.NoError(t, err)
		assert.Equal(t, materialize.DestinationMatches, state)
	})
}

func TestPublishIsIdempotentAndGuardsTheReference(t *testing.T) {
	backends(t, func(t *testing.T, repo *Repository) {
		ctx := context.Background()
		first := publish(t, repo, publishSpec{content: "base"})

		again, err := repo.Publish(ctx, prepare(t, publishSpec{content: "base"}), artifact.PublishOptions{ExpectedPrior: &first.Pinned})
		require.NoError(t, err)
		assert.Equal(t, artifact.OutcomeUnchanged, again.Outcome)
		assert.Equal(t, first.Pinned.ResolvedDigest, again.Pinned.ResolvedDigest)

		changed := prepare(t, publishSpec{content: "changed"})
		_, err = repo.Publish(ctx, changed, artifact.PublishOptions{})
		require.ErrorIs(t, err, errUtils.ErrArtifactConflict, "an existing reference expected to be absent")
		_, err = repo.Publish(ctx, changed, artifact.PublishOptions{ExpectedPrior: &first.Pinned})
		require.ErrorIs(t, err, errUtils.ErrArtifactConflict, "replacement needs force")
		stale := first.Pinned
		stale.ResolvedDigest = digest.FromString("stale")
		_, err = repo.Publish(ctx, changed, artifact.PublishOptions{ExpectedPrior: &stale, Force: true})
		require.ErrorIs(t, err, errUtils.ErrArtifactConflict, "a stale expected prior is a conflict even with force")

		replaced, err := repo.Publish(ctx, changed, artifact.PublishOptions{ExpectedPrior: &first.Pinned, Force: true})
		require.NoError(t, err)
		assert.Equal(t, artifact.OutcomeReplaced, replaced.Outcome)
		assert.NotEqual(t, first.Pinned.ResolvedDigest, replaced.Pinned.ResolvedDigest)

		missing := artifact.PinnedReference{ResolvedDigest: digest.FromString("gone")}
		_, err = repo.Publish(ctx, prepare(t, publishSpec{name: "other/name", content: "x"}), artifact.PublishOptions{ExpectedPrior: &missing})
		require.ErrorIs(t, err, errUtils.ErrArtifactConflict, "an expected prior that no longer exists")
	})
}

func TestMovedTagDoesNotChangeAPinnedRead(t *testing.T) {
	backends(t, func(t *testing.T, repo *Repository) {
		ctx := context.Background()
		first := publish(t, repo, publishSpec{content: "first"})
		pinned, err := repo.Resolve(ctx, artifact.Ref{Name: appName, Version: appVersion})
		require.NoError(t, err)

		_, err = repo.Publish(ctx, prepare(t, publishSpec{content: "second"}), artifact.PublishOptions{ExpectedPrior: &first.Pinned, Force: true})
		require.NoError(t, err)

		content, err := repo.Read(ctx, pinned)
		require.NoError(t, err)
		defer content.Close()
		digestOfRecord, err := content.Record.RecordDigest()
		require.NoError(t, err)
		assert.Equal(t, pinned.RecordDigest, digestOfRecord, "the read follows the pinned subject, not the moved tag")
	})
}

func TestReadVerifiesPayloadAgainstItsDescriptor(t *testing.T) {
	backends(t, func(t *testing.T, repo *Repository) {
		result := publish(t, repo, publishSpec{content: "tamper"})
		pinned, err := repo.Resolve(context.Background(), artifact.Ref{Name: appName, Version: appVersion})
		require.NoError(t, err)
		corruptBlob(t, repo, payloadDigest(t, result))

		content, err := repo.Read(context.Background(), pinned)
		require.NoError(t, err)
		defer content.Close()
		var payload *artifact.ContentStream
		for index := range content.Streams {
			if content.Streams[index].Role == artifact.BlobPayload {
				payload = &content.Streams[index]
			}
		}
		require.NotNil(t, payload)
		_, err = io.ReadAll(payload.Reader)
		require.ErrorIs(t, err, errUtils.ErrArtifactIntegrityFailed)
	})
}

func TestCancellationLeavesNoReference(t *testing.T) {
	backends(t, func(t *testing.T, repo *Repository) {
		canceled, cancel := context.WithCancel(context.Background())
		cancel()
		_, err := repo.Publish(canceled, prepare(t, publishSpec{content: "never"}), artifact.PublishOptions{})
		require.ErrorIs(t, err, context.Canceled)
		_, err = repo.Resolve(canceled, artifact.Ref{Name: appName, Version: appVersion})
		require.ErrorIs(t, err, context.Canceled)

		_, err = repo.Resolve(context.Background(), artifact.Ref{Name: appName, Version: appVersion})
		require.ErrorIs(t, err, errUtils.ErrArtifactNotFound, "a canceled publish created nothing")
	})
}

func TestRejectsMismatchedRecordForItsTag(t *testing.T) {
	repo := newLayoutRepo(t)
	result := publish(t, repo, publishSpec{content: "mismatch"})
	// Serve the 1.0.0 package under another version's tag, as a malicious writer could.
	st, err := repo.be.open(context.Background(), appName, true)
	require.NoError(t, err)
	desc, ok := repo.descriptorFor(appName, result.Pinned.ResolvedDigest)
	require.True(t, ok)
	require.NoError(t, st.Tag(context.Background(), desc, repo.be.reference(appName, "9.9.9")))

	pinned, err := repo.Resolve(context.Background(), artifact.Ref{Name: appName, Version: "9.9.9"})
	require.NoError(t, err)
	_, err = repo.Inspect(context.Background(), pinned)
	require.ErrorIs(t, err, errUtils.ErrArtifactIntegrityFailed, "a tag serving another version's Record is tampering")
}
