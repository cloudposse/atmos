package s3

import (
	"context"
	"errors"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/artifact"
	"github.com/cloudposse/atmos/pkg/artifact/blob/s3/s3test"
	"github.com/cloudposse/atmos/pkg/artifact/layout"
)

// payloadKey returns the bucket key of the primary payload object of the default artifact.
func payloadKey(packaging artifact.Packaging, file string) string {
	location := "team/app/1.0.0"
	if isArchive(packaging) {
		return location + layout.ArchiveExtension(packaging)
	}
	return location + "/" + file
}

func metadataKeyFor(packaging artifact.Packaging) string {
	location := "team/app/1.0.0"
	if isArchive(packaging) {
		return layout.MetadataSidecarPath(location + layout.ArchiveExtension(packaging))
	}
	return layout.DistributionPath(location, layout.ManifestName)
}

func documentKeyFor(packaging artifact.Packaging, name string) string {
	location := "team/app/1.0.0"
	if isArchive(packaging) {
		return layout.DocsSidecarDir(location+layout.ArchiveExtension(packaging)) + "/" + name
	}
	return layout.DistributionPath(location, name)
}

// edit rewrites an object in the fake with fn, as a stranger with bucket access could.
func edit(t *testing.T, fake *s3test.Server, key string, fn func([]byte) []byte) {
	t.Helper()
	object, ok := fake.Object(key)
	require.True(t, ok, "object %s exists", key)
	fake.Seed(key, fn(object.Data), object.Tags)
}

// sameLengthEdit changes content without changing its size, so only the digest can catch it.
func sameLengthEdit(data []byte) []byte {
	out := append([]byte(nil), data...)
	out[0] = 'X'
	if data[0] == 'X' {
		out[0] = 'Y'
	}
	return out
}

func truncate(data []byte) []byte { return data[:len(data)-1] }

// Every consumed byte is verified against the digest in the Record; ETags prove nothing.
func TestTamperedPayload_FailsAtTheDeclaredDigest(t *testing.T) {
	for _, spec := range allSpecs {
		t.Run(specName(spec)+"/same length", func(t *testing.T) {
			spec = spec.fresh(t)
			repo := newRepo(t, spec)
			prep, _ := packageSpec(t)
			publish(t, repo, prep, artifact.PublishOptions{})
			pinned := resolve(t, repo, defaultRef())
			edit(t, spec.fake, payloadKey(spec.packaging, "nested/deep/file.txt"), sameLengthEdit)

			content, err := repo.Read(context.Background(), pinned)
			require.NoError(t, err, "opening is cheap; verification happens as the bytes are consumed")
			defer content.Close()
			require.ErrorIs(t, readAll(t, content), errUtils.ErrArtifactIntegrityFailed)
		})
		t.Run(specName(spec)+"/truncated", func(t *testing.T) {
			spec = spec.fresh(t)
			repo := newRepo(t, spec)
			prep, _ := packageSpec(t)
			publish(t, repo, prep, artifact.PublishOptions{})
			pinned := resolve(t, repo, defaultRef())
			edit(t, spec.fake, payloadKey(spec.packaging, "nested/deep/file.txt"), truncate)

			content, err := repo.Read(context.Background(), pinned)
			require.NoError(t, err)
			defer content.Close()
			require.ErrorIs(t, readAll(t, content), errUtils.ErrArtifactIntegrityFailed, "a size mismatch is caught before any byte is served")
		})
	}
}

func TestMissingObjects_FailExplicitly(t *testing.T) {
	for _, spec := range []*rspec{noneSpec(), archiveSpec(artifact.PackagingTgz)} {
		t.Run(specName(spec)+"/payload", func(t *testing.T) {
			spec = spec.fresh(t)
			repo := newRepo(t, spec)
			prep, _ := packageSpec(t)
			publish(t, repo, prep, artifact.PublishOptions{})
			pinned := resolve(t, repo, defaultRef())
			require.NoError(t, repo.deleteObject(context.Background(), payloadKey(spec.packaging, "nested/deep/file.txt")))

			content, err := repo.Read(context.Background(), pinned)
			require.NoError(t, err)
			defer content.Close()
			require.ErrorIs(t, readAll(t, content), errUtils.ErrArtifactIntegrityFailed, "a completed reference promised the object exists")
		})
		t.Run(specName(spec)+"/metadata", func(t *testing.T) {
			spec = spec.fresh(t)
			repo := newRepo(t, spec)
			publish(t, repo, &prepareSpec{}, artifact.PublishOptions{})
			pinned := resolve(t, repo, defaultRef())
			require.NoError(t, repo.deleteObject(context.Background(), metadataKeyFor(spec.packaging)))
			_, err := repo.Read(context.Background(), pinned)
			require.ErrorIs(t, err, errUtils.ErrArtifactIntegrityFailed)
		})
		t.Run(specName(spec)+"/document", func(t *testing.T) {
			spec = spec.fresh(t)
			repo := newRepo(t, spec)
			prep, _ := packageSpec(t)
			publish(t, repo, prep, artifact.PublishOptions{})
			pinned := resolve(t, repo, defaultRef())
			require.NoError(t, repo.deleteObject(context.Background(), documentKeyFor(spec.packaging, "README.md")))

			stream, err := repo.OpenDocument(context.Background(), pinned, artifact.DocumentReadme)
			require.NoError(t, err)
			_, err = io.Copy(io.Discard, stream.Reader)
			require.ErrorIs(t, err, errUtils.ErrArtifactIntegrityFailed)
			assert.NotErrorIs(t, err, errUtils.ErrArtifactDocumentMissing, "a lost object is damage, not an absent optional document")
			other, err := repo.OpenDocument(context.Background(), pinned, artifact.DocumentChangelog)
			require.NoError(t, err, "the other document is unaffected")
			_, err = io.Copy(io.Discard, other.Reader)
			require.NoError(t, err)
		})
	}
}

func TestTamperedMetadataAndDocuments_FailIntegrity(t *testing.T) {
	for _, spec := range []*rspec{noneSpec(), archiveSpec(artifact.PackagingZip)} {
		t.Run(specName(spec)+"/metadata copy", func(t *testing.T) {
			spec = spec.fresh(t)
			repo := newRepo(t, spec)
			publish(t, repo, &prepareSpec{}, artifact.PublishOptions{})
			pinned := resolve(t, repo, defaultRef())
			edit(t, spec.fake, metadataKeyFor(spec.packaging), sameLengthEdit)
			_, err := repo.Read(context.Background(), pinned)
			require.ErrorIs(t, err, errUtils.ErrArtifactIntegrityFailed)
		})
		t.Run(specName(spec)+"/document bytes", func(t *testing.T) {
			spec = spec.fresh(t)
			repo := newRepo(t, spec)
			prep, _ := packageSpec(t)
			publish(t, repo, prep, artifact.PublishOptions{})
			pinned := resolve(t, repo, defaultRef())
			edit(t, spec.fake, documentKeyFor(spec.packaging, "README.md"), sameLengthEdit)
			stream, err := repo.OpenDocument(context.Background(), pinned, artifact.DocumentReadme)
			require.NoError(t, err)
			_, err = io.Copy(io.Discard, stream.Reader)
			require.ErrorIs(t, err, errUtils.ErrArtifactIntegrityFailed)
		})
	}
}

// Tampered managed state fails integrity instead of being trusted.
func TestTamperedState_FailsIntegrity(t *testing.T) {
	setup := func(t *testing.T) (*Repository, *s3test.Server, artifact.PinnedReference) {
		spec := noneSpec().fresh(t)
		repo := newRepo(t, spec)
		result := publish(t, repo, &prepareSpec{}, artifact.PublishOptions{})
		return repo, spec.fake, result.Pinned
	}
	t.Run("record bytes do not hash to their name", func(t *testing.T) {
		repo, fake, pinned := setup(t)
		edit(t, fake, layout.RecordPath(pinned.RecordDigest), sameLengthEdit)
		_, err := repo.Resolve(context.Background(), defaultRef())
		require.ErrorIs(t, err, errUtils.ErrArtifactIntegrityFailed)
	})
	t.Run("reference is not canonical", func(t *testing.T) {
		repo, fake, _ := setup(t)
		fake.Seed(layout.ReferencePath(layout.ReferenceKey(testName, testVersion)), []byte(`{"not":"a reference"}`), nil)
		_, err := repo.Resolve(context.Background(), defaultRef())
		require.ErrorIs(t, err, errUtils.ErrArtifactIntegrityFailed)
	})
	t.Run("representation is missing", func(t *testing.T) {
		repo, _, pinned := setup(t)
		require.NoError(t, repo.deleteObject(context.Background(), layout.RepresentationPath(layout.ReferenceKey(testName, testVersion))))
		_, err := repo.Inspect(context.Background(), pinned)
		require.ErrorIs(t, err, errUtils.ErrArtifactIntegrityFailed)
		_, err = repo.Read(context.Background(), pinned)
		require.ErrorIs(t, err, errUtils.ErrArtifactIntegrityFailed)
	})
	t.Run("an unchanged republish detects a missing representation", func(t *testing.T) {
		repo, _, pinned := setup(t)
		require.NoError(t, repo.deleteObject(context.Background(), layout.RepresentationPath(layout.ReferenceKey(testName, testVersion))))
		_, err := repo.Publish(context.Background(), prepare(t, repo, &prepareSpec{opID: "op-again"}), artifact.PublishOptions{ExpectedPrior: &pinned})
		require.ErrorIs(t, err, errUtils.ErrArtifactIntegrityFailed)
	})
}

// A reader notices when the reference moves while it reads from the mutable payload location, and
// discards the bytes instead of serving them under the old digest.
func TestRead_ReaderReportsTheReferenceMoving(t *testing.T) {
	for _, spec := range []*rspec{noneSpec(), archiveSpec(artifact.PackagingTgz)} {
		t.Run(specName(spec), func(t *testing.T) {
			spec = spec.fresh(t)
			repo := newRepo(t, spec)
			base := publish(t, repo, &prepareSpec{labels: map[string]string{"rev": "1"}}, artifact.PublishOptions{})
			content, err := repo.Read(context.Background(), base.Pinned)
			require.NoError(t, err)
			defer content.Close()

			// Same payload bytes, different Record: only the generation recheck can tell.
			publish(t, repo, &prepareSpec{labels: map[string]string{"rev": "2"}}, artifact.PublishOptions{Force: true, ExpectedPrior: &base.Pinned})
			err = readAll(t, content)
			if spec.packaging == artifact.PackagingNone {
				require.ErrorIs(t, err, errUtils.ErrArtifactPublicationUnavailable)
				require.ErrorContains(t, err, "changed while it was being read")
				return
			}
			// An archive embeds the Record, so the replaced bytes already fail their digest.
			require.Error(t, err)
			assert.True(t, errors.Is(err, errUtils.ErrArtifactPublicationUnavailable) || errors.Is(err, errUtils.ErrArtifactIntegrityFailed), "%v", err)
		})
	}
}

func TestRead_ReaderReportsDeletion(t *testing.T) {
	spec := noneSpec().fresh(t)
	repo := newRepo(t, spec)
	base := publish(t, repo, &prepareSpec{}, artifact.PublishOptions{})
	content, err := repo.Read(context.Background(), base.Pinned)
	require.NoError(t, err)
	defer content.Close()
	_, err = repo.Delete(context.Background(), base.Pinned, artifact.DeleteOptions{})
	require.NoError(t, err)
	require.Error(t, readAll(t, content), "bytes of a deleted artifact are never reported as valid")
}

func TestCanceledContextStopsReads(t *testing.T) {
	repo := newRepo(t, noneSpec())
	base := publish(t, repo, &prepareSpec{}, artifact.PublishOptions{})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := repo.Resolve(ctx, defaultRef())
	require.ErrorIs(t, err, context.Canceled)
	_, err = repo.Read(ctx, base.Pinned)
	require.ErrorIs(t, err, context.Canceled)
	_, err = repo.List(ctx, artifact.ListQuery{})
	require.ErrorIs(t, err, context.Canceled)
}

// Objects added inside a payload prefix are never adopted into the artifact.
func TestExtraObjectsInsideAPayloadAreNotAdopted(t *testing.T) {
	spec := noneSpec().fresh(t)
	repo := newRepo(t, spec)
	result := publish(t, repo, &prepareSpec{}, artifact.PublishOptions{})
	spec.fake.Seed("team/app/1.0.0/injected.txt", []byte("not in the inventory"), nil)
	content, err := repo.Read(context.Background(), result.Pinned)
	require.NoError(t, err)
	defer content.Close()
	for index := range content.Streams {
		assert.NotEqual(t, "injected.txt", content.Streams[index].Path)
	}
}

// A payload that grows past its declared size is rejected as soon as it exceeds it.
func TestRead_PayloadThatGrowsIsRejected(t *testing.T) {
	spec := noneSpec().fresh(t)
	repo := newRepo(t, spec)
	result := publish(t, repo, &prepareSpec{}, artifact.PublishOptions{})
	edit(t, spec.fake, payloadKey(artifact.PackagingNone, "data.txt"), func(data []byte) []byte { return append(append([]byte(nil), data...), "extra"...) })
	content, err := repo.Read(context.Background(), result.Pinned)
	require.NoError(t, err)
	defer content.Close()
	require.ErrorIs(t, readAll(t, content), errUtils.ErrArtifactIntegrityFailed)
}
