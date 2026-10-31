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
	"github.com/cloudposse/atmos/pkg/artifact/blob"
	"github.com/cloudposse/atmos/pkg/artifact/blob/s3/s3test"
)

// A tampered journal or representation can never address an object outside its location.
func TestObjectSetKeys_RejectsUnsafeKeys(t *testing.T) {
	for _, file := range []string{"../escape", "a/../../b", "/absolute", ""} {
		_, err := (objectSet{files: []string{file}}).keys(artifact.PackagingNone, "team/app/1.0.0")
		require.ErrorIs(t, err, errUtils.ErrArtifactUnsafePath, "file %q", file)
	}
	keys, err := (objectSet{files: []string{"a.txt"}, docs: []string{"README.md"}}).keys(artifact.PackagingTgz, "team/app/1.0.0.tgz")
	require.NoError(t, err)
	assert.Equal(t, []string{"team/app/1.0.0.tgz", "team/app/1.0.0.tgz.metadata.json", "team/app/1.0.0.tgz.docs/README.md"}, keys, "an archive owns the object and its sidecars, not per-file keys")
	_, err = (objectSet{}).keys(artifact.PackagingNone, "../x")
	require.ErrorIs(t, err, errUtils.ErrArtifactUnsafePath)
}

func TestClaimPaths(t *testing.T) {
	assert.Equal(t, []string{"a/b"}, claimPaths(artifact.PackagingNone, "a/b"))
	assert.Equal(t, []string{"a/b.tgz", "a/b.tgz.metadata.json", "a/b.tgz.docs"}, claimPaths(artifact.PackagingTgz, "a/b.tgz"))
}

func TestMapErr_KeepsTheCauseAndChoosesTheSentinel(t *testing.T) {
	repo := newRepo(t, noneSpec())
	tests := []struct {
		name string
		err  error
		is   error
	}{
		{name: "canceled", err: context.Canceled, is: context.Canceled},
		{name: "precondition", err: blob.NewError(blob.KindPrecondition, blob.OpPut, "k", nil), is: errUtils.ErrArtifactConflict},
		{name: "ambiguous", err: blob.NewError(blob.KindAmbiguous, blob.OpPut, "k", nil), is: errUtils.ErrArtifactPublicationAmbiguous},
		{name: "conditional unsupported", err: blob.NewError(blob.KindUnsupported, blob.OpPut, "k", nil), is: errUtils.ErrArtifactUnsupportedCapability},
		{name: "identity", err: blob.NewError(blob.KindIdentity, "client", "", nil), is: errUtils.ErrBlobIdentityUnresolved},
		{name: "other write", err: errors.New("boom"), is: errUtils.ErrArtifactUploadFailed},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mapped := repo.mapErr(opWrite, "object", tt.err)
			require.ErrorIs(t, mapped, tt.is)
			require.ErrorIs(t, mapped, tt.err, "the original error stays in the chain")
		})
	}
	require.NoError(t, repo.mapErr(opWrite, "object", nil))
	assert.ErrorIs(t, repo.mapErr(opRead, "object", errors.New("boom")), errUtils.ErrArtifactDownloadFailed)
	assert.ErrorIs(t, repo.mapErr(opDelete, "object", errors.New("boom")), errUtils.ErrArtifactDeleteFailed)
}

func TestNewJob_RejectsMalformedPreparations(t *testing.T) {
	repo := newRepo(t, noneSpec())
	_, err := repo.newJob(nil)
	require.ErrorIs(t, err, errUtils.ErrArtifactConfigInvalid)

	prepared := prepare(t, repo, &prepareSpec{})
	prepared.Packaging = "rar"
	_, err = repo.newJob(prepared)
	require.ErrorIs(t, err, errUtils.ErrArtifactUnsupportedCapability)

	noOpen := prepare(t, repo, &prepareSpec{})
	noOpen.Blobs[0].Open = nil
	_, err = repo.newJob(noOpen)
	require.ErrorIs(t, err, errUtils.ErrArtifactConfigInvalid)

	mismatched := prepare(t, repo, &prepareSpec{})
	mismatched.Blobs[0].Descriptor.Size++
	_, err = repo.newJob(mismatched)
	require.Error(t, err)

	archive := newRepo(t, archiveSpec(artifact.PackagingTgz))
	doubled := prepare(t, archive, &prepareSpec{})
	doubled.Blobs = append(doubled.Blobs, doubled.Blobs[0])
	_, err = archive.newJob(doubled)
	require.Error(t, err)
}

// An upload that is corrupted on its way out is never committed: the verifying reader fails the put.
func TestPublish_CorruptStagedBlobIsNeverCommitted(t *testing.T) {
	spec := noneSpec().fresh(t)
	repo := newRepo(t, spec)
	prepared := prepare(t, repo, &prepareSpec{files: bigFiles()})
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
			return &flippingReader{ReadCloser: reader}, nil
		}
	}
	_, err := repo.Publish(context.Background(), prepared, artifact.PublishOptions{})
	require.ErrorIs(t, err, errUtils.ErrArtifactIntegrityFailed)
	_, err = repo.Resolve(context.Background(), defaultRef())
	require.ErrorIs(t, err, errUtils.ErrArtifactPublicationUnavailable, "the reference never reached completed")
	assert.Empty(t, spec.fake.OpenUploads(), "the failed multipart upload was aborted")
	_, ok := spec.fake.Object("team/app/1.0.0/nested/big.bin")
	assert.False(t, ok, "the corrupted object was never completed")
}

func TestPublish_ServerErrorsAreUploadFailures(t *testing.T) {
	spec := noneSpec().fresh(t)
	repo := newRepo(t, spec)
	spec.fake.Inject(s3test.Fault{Op: s3test.OpPutObject, KeyPrefix: "team/app", Status: 403, Code: "AccessDenied", Times: -1})
	_, err := repo.Publish(context.Background(), prepare(t, repo, &prepareSpec{}), artifact.PublishOptions{})
	require.ErrorIs(t, err, errUtils.ErrArtifactUploadFailed)
	_, err = repo.Resolve(context.Background(), defaultRef())
	require.ErrorIs(t, err, errUtils.ErrArtifactPublicationUnavailable, "the failed upload leaves a pending reference for the retry to repair")
}

// flippingReader corrupts the first byte it delivers.
type flippingReader struct {
	io.ReadCloser
	flipped bool
}

func (f *flippingReader) Read(p []byte) (int, error) {
	n, err := f.ReadCloser.Read(p)
	if n > 0 && !f.flipped {
		p[0] ^= 0xff
		f.flipped = true
	}
	return n, err
}
