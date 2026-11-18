package s3

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/artifact"
	"github.com/cloudposse/atmos/pkg/artifact/artifacttest"
	"github.com/cloudposse/atmos/pkg/artifact/blob"
)

// errTestSigner is the mutation failure the failing signer reports.
var errTestSigner = errors.New("signer unavailable")

// failingSigner fails to sign, so the mutation under the claim fails.
type failingSigner struct{ *artifacttest.FakeSigner }

func (failingSigner) Sign(context.Context, artifact.SignatureSubject) (*artifact.EvidencePayload, error) {
	return nil, errTestSigner
}

// A mutation that fails while the claim cannot be released reports both failures: the caller must
// learn about the claim that remains held, not only about the mutation error.
func TestClaimedMutations_KeepTheReleaseErrorBesideTheMutationError(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(t *testing.T, repo *Repository, pinned *artifact.PinnedReference) error
		wantErr error
	}{
		{
			name: "sign",
			mutate: func(t *testing.T, repo *Repository, pinned *artifact.PinnedReference) error {
				signer := failingSigner{newSigner()}
				_, err := repo.Sign(context.Background(), recordSubject(t, repo, pinned), signer, artifact.SignOptions{})
				return err
			},
			wantErr: errTestSigner,
		},
		{
			name: "write evidence",
			mutate: func(t *testing.T, repo *Repository, pinned *artifact.PinnedReference) error {
				blobs := []artifact.EvidenceBlob{{Evidence: artifact.SignatureEvidence{Subject: artifact.SignatureSubject{Kind: artifact.SubjectRecord}}}}
				_, err := repo.WriteEvidence(context.Background(), pinned, blobs, nil)
				return err
			},
			wantErr: errUtils.ErrArtifactRefInvalid,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			spec := signedSpec(t, noneSpec())
			seed := newRepo(t, spec)
			publish(t, seed, &prepareSpec{}, artifact.PublishOptions{})
			pinned := resolve(t, seed, defaultRef())

			// Once the claim is created, remove it behind the holder's back: the release then
			// finds nothing to release.
			store := &hookStore{Store: newStoreFor(t, spec)}
			store.afterPut = func(key string, info *blob.ObjectInfo, err error) (*blob.ObjectInfo, error) {
				if err == nil && strings.Contains(key, "/locks/") {
					require.NoError(t, store.Delete(context.Background(), key, blob.DeleteOptions{}))
				}
				return info, err
			}
			repo, err := newRepository("test", spec.config(spec.fake.Bucket()),
				[]Option{WithStore(store), WithVerifier(&artifacttest.FakeVerifier{}), withLockPoll(5 * time.Millisecond)}, &artifact.RepositoryOptions{})
			require.NoError(t, err)

			err = tt.mutate(t, repo, &pinned)
			require.Error(t, err)
			assert.ErrorIs(t, err, tt.wantErr, "the mutation error survives")
			assert.ErrorIs(t, err, errUtils.ErrArtifactConflict, "the release failure survives")
			assert.ErrorContains(t, err, "is not held")
		})
	}
}
