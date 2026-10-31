package local

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/opencontainers/go-digest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/artifact"
	"github.com/cloudposse/atmos/pkg/artifact/layout"
)

// cloneWithBlobs returns a prepared artifact sharing the Record but with different blobs, without
// copying the original's once-guard.
func cloneWithBlobs(p *artifact.PreparedArtifact, blobs []artifact.StagedBlob) *artifact.PreparedArtifact {
	return &artifact.PreparedArtifact{
		Record: p.Record, IntentDigest: p.IntentDigest, OperationID: p.OperationID, PreparedAt: p.PreparedAt,
		Packaging: p.Packaging, Blobs: blobs,
	}
}

func fileBlob(path string, data []byte) artifact.StagedBlob {
	return artifact.StagedBlob{
		Descriptor: artifact.StagedBlob{}.Descriptor, Role: artifact.BlobPayload, Path: path,
		Open: func(context.Context) (io.ReadCloser, error) { return io.NopCloser(bytes.NewReader(data)), nil },
	}
}

func TestPublish_RejectsInconsistentPreparedBlobs(t *testing.T) {
	files := map[string]string{"a.txt": "alpha", "b/c.txt": "gamma"}
	docFiles := map[string]string{"a.txt": "alpha", ".atmos/artifact/README.md": "docs\n"}
	tests := []struct {
		name  string
		spec  *repoSpec
		files map[string]string
		edit  func(p *artifact.PreparedArtifact) *artifact.PreparedArtifact
		want  error
	}{
		{"unopenable blob", dirSpec(), files, func(p *artifact.PreparedArtifact) *artifact.PreparedArtifact {
			blobs := append([]artifact.StagedBlob(nil), p.Blobs...)
			blobs[0].Open = nil
			return cloneWithBlobs(p, blobs)
		}, errUtils.ErrArtifactConfigInvalid},
		{"inventory file without a blob", dirSpec(), files, func(p *artifact.PreparedArtifact) *artifact.PreparedArtifact {
			return cloneWithBlobs(p, p.Blobs[:1])
		}, errUtils.ErrArtifactIntegrityFailed},
		{"blob that is not in the inventory", dirSpec(), files, func(p *artifact.PreparedArtifact) *artifact.PreparedArtifact {
			extra := p.Blobs[0]
			extra.Path = "unlisted.txt"
			return cloneWithBlobs(p, append(append([]artifact.StagedBlob(nil), p.Blobs...), extra))
		}, errUtils.ErrArtifactIntegrityFailed},
		{"blob without a path", dirSpec(), files, func(p *artifact.PreparedArtifact) *artifact.PreparedArtifact {
			blobs := append([]artifact.StagedBlob(nil), p.Blobs...)
			blobs[0].Path = ""
			return cloneWithBlobs(p, blobs)
		}, errUtils.ErrArtifactIntegrityFailed},
		{"duplicate path", dirSpec(), files, func(p *artifact.PreparedArtifact) *artifact.PreparedArtifact {
			return cloneWithBlobs(p, append(append([]artifact.StagedBlob(nil), p.Blobs...), p.Blobs[0]))
		}, errUtils.ErrArtifactIntegrityFailed},
		{"document blob missing", dirSpec(), docFiles, func(p *artifact.PreparedArtifact) *artifact.PreparedArtifact {
			var blobs []artifact.StagedBlob
			for _, blob := range p.Blobs {
				if blob.Role == artifact.BlobPayload {
					blobs = append(blobs, blob)
				}
			}
			return cloneWithBlobs(p, blobs)
		}, errUtils.ErrArtifactIntegrityFailed},
		{"duplicate document blob", dirSpec(), docFiles, func(p *artifact.PreparedArtifact) *artifact.PreparedArtifact {
			return cloneWithBlobs(p, append(append([]artifact.StagedBlob(nil), p.Blobs...), p.Blobs[len(p.Blobs)-1]))
		}, errUtils.ErrArtifactRecordInvalid},
		{"document blob with the wrong digest", dirSpec(), docFiles, func(p *artifact.PreparedArtifact) *artifact.PreparedArtifact {
			blobs := append([]artifact.StagedBlob(nil), p.Blobs...)
			blobs[len(blobs)-1].Descriptor.Digest = digest.FromString("other")
			return cloneWithBlobs(p, blobs)
		}, errUtils.ErrArtifactIntegrityFailed},
		{"archive without a payload blob", archiveSpec(artifact.PackagingTgz), files, func(p *artifact.PreparedArtifact) *artifact.PreparedArtifact {
			return cloneWithBlobs(p, nil)
		}, errUtils.ErrArtifactPackagingInvalid},
		{"archive with two payload blobs", archiveSpec(artifact.PackagingTgz), files, func(p *artifact.PreparedArtifact) *artifact.PreparedArtifact {
			return cloneWithBlobs(p, append(append([]artifact.StagedBlob(nil), p.Blobs...), p.Blobs[0]))
		}, errUtils.ErrArtifactPackagingInvalid},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := newRepo(t, tt.spec)
			prepared := tt.edit(prepare(t, repo, &prepareSpec{files: tt.files}))
			_, err := repo.Publish(context.Background(), prepared, artifact.PublishOptions{})
			require.ErrorIs(t, err, tt.want)
			_, err = repo.Resolve(context.Background(), defaultRef())
			require.ErrorIs(t, err, errUtils.ErrArtifactNotFound, "a rejected preparation leaves no pending reference behind")
			_, statErr := os.Stat(repo.root)
			assert.True(t, os.IsNotExist(statErr), "and creates nothing")
		})
	}
}

// A blob whose bytes disagree with its descriptor fails the copy; the reference stays unavailable
// until a correct retry repairs it, and the bad bytes are never visible.
func TestPublish_CorruptStagedBlobIsNeverCommitted(t *testing.T) {
	for _, spec := range []*repoSpec{dirSpec(), archiveSpec(artifact.PackagingTgz)} {
		t.Run(spec.kind, func(t *testing.T) {
			repo := newRepo(t, spec)
			good := prepare(t, repo, &prepareSpec{})
			for _, lie := range []string{"digest", "short", "long"} {
				blobs := append([]artifact.StagedBlob(nil), good.Blobs...)
				original := blobs[0].Open
				switch lie {
				case "digest":
					blobs[0].Open = func(ctx context.Context) (io.ReadCloser, error) {
						reader, err := original(ctx)
						if err != nil {
							return nil, err
						}
						data, readErr := io.ReadAll(reader)
						_ = reader.Close()
						if len(data) > 0 {
							data[0] ^= 0xff
						}
						return io.NopCloser(bytes.NewReader(data)), readErr
					}
				case "short":
					blobs[0].Open = func(ctx context.Context) (io.ReadCloser, error) {
						reader, err := original(ctx)
						if err != nil {
							return nil, err
						}
						data, _ := io.ReadAll(reader)
						_ = reader.Close()
						return io.NopCloser(bytes.NewReader(data[:len(data)-1])), nil
					}
				case "long":
					blobs[0].Open = func(ctx context.Context) (io.ReadCloser, error) {
						reader, err := original(ctx)
						if err != nil {
							return nil, err
						}
						data, _ := io.ReadAll(reader)
						_ = reader.Close()
						return io.NopCloser(bytes.NewReader(append(data, 'x'))), nil
					}
				}
				_, err := repo.Publish(context.Background(), cloneWithBlobs(good, blobs), artifact.PublishOptions{})
				require.ErrorIs(t, err, errUtils.ErrArtifactIntegrityFailed, lie)
				_, err = repo.Resolve(context.Background(), defaultRef())
				require.ErrorIs(t, err, errUtils.ErrArtifactPublicationUnavailable, "%s: no partial success is advertised", lie)
				assert.NoDirExists(t, filepath.Join(repo.root, filepath.FromSlash(layout.StagingPath(good.OperationID))), "%s: a failed attempt cleans its staging", lie)
			}
			_, err := repo.Publish(context.Background(), good, artifact.PublishOptions{})
			require.NoError(t, err, "the correct retry repairs the reference")
			assertIntact(t, repo, defaultRef())
		})
	}
}

// Cancellation mid-publication surfaces as a context error and leaves the same repairable state.
func TestPublish_CancellationLeavesARepairableState(t *testing.T) {
	repo := newRepo(t, dirSpec())
	good := prepare(t, repo, &prepareSpec{files: map[string]string{"a.txt": "a", "b.txt": "b"}})
	ctx, cancel := context.WithCancel(context.Background())
	blobs := append([]artifact.StagedBlob(nil), good.Blobs...)
	original := blobs[0].Open
	blobs[0].Open = func(openCtx context.Context) (io.ReadCloser, error) {
		cancel()
		return original(openCtx)
	}
	_, err := repo.Publish(ctx, cloneWithBlobs(good, blobs), artifact.PublishOptions{})
	require.ErrorIs(t, err, context.Canceled)
	_, err = repo.Resolve(context.Background(), defaultRef())
	require.ErrorIs(t, err, errUtils.ErrArtifactPublicationUnavailable)

	_, err = repo.Publish(context.Background(), good, artifact.PublishOptions{})
	require.NoError(t, err)
	assertIntact(t, repo, defaultRef())
}

// If the journal was lost, a same-intent retry still repairs the pending reference.
func TestPendingReference_RepairsWithoutItsJournal(t *testing.T) {
	var fault armedFault
	spec := dirSpec()
	spec = spec.withOpts(fault.option())
	repo := newRepo(t, spec)
	publish(t, repo, &prepareSpec{files: map[string]string{"data.txt": "v1"}}, artifact.PublishOptions{})
	old := resolve(t, repo, defaultRef())
	replacement := prepare(t, repo, &prepareSpec{files: map[string]string{"data.txt": "v2"}, opID: "op-2"})
	fault.hook = crashAt(faultAfterMetadata)
	_, err := repo.Publish(context.Background(), replacement, artifact.PublishOptions{Force: true, ExpectedPrior: &old})
	require.ErrorIs(t, err, errCrash)

	require.NoError(t, os.RemoveAll(filepath.Join(repo.root, filepath.FromSlash(layout.ManagedRoot), "operations")))
	retry, err := repo.Publish(context.Background(), replacement, artifact.PublishOptions{Force: true})
	require.NoError(t, err)
	assert.Equal(t, artifact.OutcomeReplaced, retry.Outcome)
	assertIntact(t, repo, defaultRef())
	_, err = repo.Read(context.Background(), old)
	require.ErrorIs(t, err, errUtils.ErrArtifactNotFound)
}

func TestAbsentRootReportsNotFoundEverywhere(t *testing.T) {
	repo := newRepo(t, signedSpec(archiveSpec(artifact.PackagingTgz)))
	ctx := context.Background()
	pinned := artifact.PinnedReference{Ref: defaultRef(), RecordDigest: digest.FromString("x"), ResolvedDigest: digest.FromString("x")}
	subject := artifact.SignatureSubject{Kind: artifact.SubjectRecord, Digest: digest.FromString("x")}

	_, err := repo.Resolve(ctx, defaultRef())
	require.ErrorIs(t, err, errUtils.ErrArtifactNotFound)
	_, err = repo.Pin(ctx, defaultRef())
	require.ErrorIs(t, err, errUtils.ErrArtifactNotFound)
	_, err = repo.Inspect(ctx, pinned)
	require.ErrorIs(t, err, errUtils.ErrArtifactNotFound)
	_, err = repo.Read(ctx, pinned)
	require.ErrorIs(t, err, errUtils.ErrArtifactNotFound)
	_, err = repo.OpenDocument(ctx, pinned, artifact.DocumentReadme)
	require.ErrorIs(t, err, errUtils.ErrArtifactNotFound)
	_, err = repo.Delete(ctx, pinned, artifact.DeleteOptions{Scope: artifact.DeleteObject})
	require.ErrorIs(t, err, errUtils.ErrArtifactNotFound)
	_, err = repo.Sign(ctx, subject, newSigner(), artifact.SignOptions{})
	require.ErrorIs(t, err, errUtils.ErrArtifactNotFound)
	_, err = repo.Verify(ctx, subject, newSigner().TrustedPolicy())
	require.ErrorIs(t, err, errUtils.ErrArtifactNotFound)
	_, err = repo.ReadEvidence(ctx, &pinned)
	require.ErrorIs(t, err, errUtils.ErrArtifactNotFound)
	_, err = repo.WriteEvidence(ctx, &pinned, nil, nil)
	require.ErrorIs(t, err, errUtils.ErrArtifactNotFound)
	_, statErr := os.Stat(repo.root)
	assert.True(t, os.IsNotExist(statErr), "no read or failed mutation created the root")
}

func TestCanceledContextStopsReads(t *testing.T) {
	repo := newRepo(t, dirSpec())
	publish(t, repo, &prepareSpec{}, artifact.PublishOptions{})
	pinned := resolve(t, repo, defaultRef())
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := repo.Resolve(ctx, defaultRef())
	require.ErrorIs(t, err, context.Canceled)
	_, err = repo.Read(ctx, pinned)
	require.ErrorIs(t, err, context.Canceled)
	_, err = repo.ReadEvidence(ctx, &pinned)
	require.ErrorIs(t, err, context.Canceled)
	_, err = repo.WriteEvidence(ctx, &pinned, nil, nil)
	require.ErrorIs(t, err, context.Canceled)
}

func TestRead_OpeningFailsCleanlyWhenAPayloadFileIsMissing(t *testing.T) {
	repo := newRepo(t, dirSpec())
	result, _ := publishFilePackage(t, repo)
	pinned := resolve(t, repo, defaultRef())
	require.NoError(t, os.Remove(filepath.Join(result.Location, "nested", "deep", "file.txt")))
	content, err := repo.Read(context.Background(), pinned)
	require.ErrorIs(t, err, errUtils.ErrArtifactIntegrityFailed)
	assert.Nil(t, content)

	require.NoError(t, os.RemoveAll(result.Location))
	_, err = repo.Read(context.Background(), pinned)
	require.ErrorIs(t, err, errUtils.ErrArtifactIntegrityFailed, "a missing payload directory is damage, not absence")
}

func TestRead_ReaderReportsTheReferenceMovingOrDisappearing(t *testing.T) {
	for _, spec := range []*repoSpec{dirSpec(), archiveSpec(artifact.PackagingTgz)} {
		t.Run(spec.kind, func(t *testing.T) {
			repo := newRepo(t, spec)
			publish(t, repo, &prepareSpec{}, artifact.PublishOptions{})
			pinned := resolve(t, repo, defaultRef())
			content, err := repo.Read(context.Background(), pinned)
			require.NoError(t, err)
			defer content.Close()
			// Remove only the reference record: payload handles stay valid on every platform.
			require.NoError(t, os.Remove(filepath.Join(repo.root, filepath.FromSlash(layout.ReferencePath(layout.ReferenceKey(testName, testVersion))))))
			require.ErrorIs(t, readAll(t, content), errUtils.ErrArtifactNotFound, "a deleted reference is reported, not served")
		})
	}
}

func TestRead_PayloadThatGrowsIsRejected(t *testing.T) {
	repo := newRepo(t, dirSpec())
	result := publish(t, repo, &prepareSpec{files: map[string]string{"data.txt": "short"}}, artifact.PublishOptions{})
	pinned := resolve(t, repo, defaultRef())
	tamper(t, repo.root, filepath.ToSlash(mustRel(t, repo.root, filepath.Join(result.Location, "data.txt"))), appendJunk)
	content, err := repo.Read(context.Background(), pinned)
	require.NoError(t, err)
	defer content.Close()
	require.ErrorIs(t, readAll(t, content), errUtils.ErrArtifactIntegrityFailed, "bytes beyond the declared size are never served")
}

func TestUnchangedRepublishDetectsAMissingRepresentation(t *testing.T) {
	repo := newRepo(t, dirSpec())
	prepared := prepare(t, repo, &prepareSpec{})
	_, err := repo.Publish(context.Background(), prepared, artifact.PublishOptions{})
	require.NoError(t, err)
	current := resolve(t, repo, defaultRef())
	require.NoError(t, os.Remove(filepath.Join(repo.root, filepath.FromSlash(layout.RepresentationPath(layout.ReferenceKey(testName, testVersion))))))
	_, err = repo.Publish(context.Background(), prepared, artifact.PublishOptions{ExpectedPrior: &current})
	require.ErrorIs(t, err, errUtils.ErrArtifactIntegrityFailed)
}

func TestRootThatIsAFileFailsClearly(t *testing.T) {
	file := filepath.Join(t.TempDir(), "root")
	require.NoError(t, os.WriteFile(file, []byte("x"), 0o644))
	repo := newRepo(t, &repoSpec{root: file})
	_, err := repo.Publish(context.Background(), prepare(t, repo, &prepareSpec{}), artifact.PublishOptions{})
	require.ErrorIs(t, err, errUtils.ErrArtifactConfigInvalid)
	_, err = repo.Lock(context.Background(), lockRequest("r"))
	require.ErrorIs(t, err, errUtils.ErrArtifactConfigInvalid)
	_, err = repo.List(context.Background(), artifact.ListQuery{})
	require.Error(t, err)
}

func TestInventoryInsideTheGeneratedDistributionDirIsRejected(t *testing.T) {
	repo := newRepo(t, dirSpec())
	prepared := prepare(t, repo, &prepareSpec{files: map[string]string{"data.txt": "x"}})
	record := prepared.Record
	record.Inventory = append(append([]artifact.Entry(nil), record.Inventory...), artifact.Entry{
		Path: ".atmos/artifact/extra.txt", Type: artifact.EntryFile, Size: 1, Mode: 0o644, Digest: digest.FromString("x"),
	})
	require.NoError(t, record.Seal())
	conflicting := &artifact.PreparedArtifact{
		Record: record, IntentDigest: prepared.IntentDigest, OperationID: "op-conflict", PreparedAt: prepared.PreparedAt,
		Packaging: artifact.PackagingNone,
		Blobs:     append(append([]artifact.StagedBlob(nil), prepared.Blobs...), fileBlob(".atmos/artifact/extra.txt", []byte("x"))),
	}
	conflicting.Blobs[len(conflicting.Blobs)-1].Descriptor = prepared.Blobs[0].Descriptor
	conflicting.Blobs[len(conflicting.Blobs)-1].Descriptor.Digest = digest.FromString("x")
	conflicting.Blobs[len(conflicting.Blobs)-1].Descriptor.Size = 1
	_, err := repo.Publish(context.Background(), conflicting, artifact.PublishOptions{})
	require.ErrorIs(t, err, errUtils.ErrArtifactUnsafePath)
}

func TestFormattedErrorsNameTheOperationAndReference(t *testing.T) {
	var fault armedFault
	spec := dirSpec()
	spec = spec.withOpts(fault.option())
	repo := newRepo(t, spec)
	fault.hook = crashAt(faultBeforePayload)
	prepared := prepare(t, repo, &prepareSpec{opID: "op-visible"})
	_, err := repo.Publish(context.Background(), prepared, artifact.PublishOptions{})
	require.ErrorIs(t, err, errCrash)
	_, err = repo.Resolve(context.Background(), defaultRef())
	require.ErrorContains(t, err, fmt.Sprintf("%s:%s", testName, testVersion))
	require.ErrorContains(t, err, "op-visible")
	require.ErrorContains(t, err, "same intent")
}
