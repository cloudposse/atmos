package s3

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/artifact"
	"github.com/cloudposse/atmos/pkg/artifact/blob"
)

func TestDelegatedMutationsAcquireCanonicalClaim(t *testing.T) {
	for _, operation := range []string{"sign", "evidence", "delete"} {
		t.Run(operation, func(t *testing.T) {
			ctx := context.Background()
			spec := signedSpec(t, noneSpec()).withOpts(WithLockWait(time.Second))
			repo := newRepo(t, spec)
			pinned := publish(t, repo, &prepareSpec{}, artifact.PublishOptions{}).Pinned
			subject := recordSubject(t, repo, &pinned)
			signer := newSigner()
			_, err := repo.Sign(ctx, subject, signer, artifact.SignOptions{})
			require.NoError(t, err)
			evidence, err := repo.ReadEvidence(ctx, &pinned)
			require.NoError(t, err)
			require.Len(t, evidence, 1)
			coordinator := newRepo(t, spec.with(func(s *rspec) { s.prefix = "coordinator" }))
			delegated, err := coordinator.Lock(ctx, artifact.LockRequest{Resource: "destination/" + lockResource, Owner: "delegate"})
			require.NoError(t, err)
			t.Cleanup(func() { _ = delegated.Release(ctx) })
			mutate := func(ctx context.Context, lock artifact.LockHandle) error {
				switch operation {
				case "sign":
					_, err := repo.Sign(ctx, subject, signer, artifact.SignOptions{Lock: lock})
					return err
				case "evidence":
					_, err := repo.WriteEvidence(ctx, &pinned, evidence, lock)
					return err
				default:
					_, err := repo.Delete(ctx, pinned, artifact.DeleteOptions{Lock: lock})
					return err
				}
			}
			held, err := repo.Lock(ctx, artifact.LockRequest{Resource: lockResource, Owner: "direct"})
			require.NoError(t, err)
			before := objectSnapshot(t, spec.fake)
			wait, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
			defer cancel()
			require.ErrorIs(t, mutate(wait, delegated), context.DeadlineExceeded)
			assert.Equal(t, before, objectSnapshot(t, spec.fake), "failed acquisition must not mutate target data or evidence")
			require.NoError(t, held.Release(ctx))
			require.NoError(t, mutate(ctx, delegated))
			if operation == "delete" {
				_, err := repo.Resolve(ctx, defaultRef())
				require.Error(t, err)
			} else {
				after, err := repo.ReadEvidence(ctx, &pinned)
				require.NoError(t, err)
				require.NotEmpty(t, after)
			}
		})
	}
}

func TestMutationsRejectInvalidSameRepositoryClaims(t *testing.T) {
	for _, operation := range []string{"sign", "evidence", "delete"} {
		for _, invalid := range []string{"wrong-resource", "released", "stale"} {
			t.Run(operation+"/"+invalid, func(t *testing.T) {
				ctx := context.Background()
				spec := signedSpec(t, noneSpec())
				repo := newRepo(t, spec)
				pinned := publish(t, repo, &prepareSpec{}, artifact.PublishOptions{}).Pinned
				subject := recordSubject(t, repo, &pinned)
				signer := newSigner()
				_, err := repo.Sign(ctx, subject, signer, artifact.SignOptions{})
				require.NoError(t, err)
				evidence, err := repo.ReadEvidence(ctx, &pinned)
				require.NoError(t, err)
				resource := lockResource
				if invalid == "wrong-resource" {
					resource = "other:1"
				}
				lock, err := repo.Lock(ctx, artifact.LockRequest{Resource: resource, Owner: "owner"})
				require.NoError(t, err)
				if invalid == "released" {
					require.NoError(t, lock.Release(ctx))
				}
				if invalid == "stale" {
					claim := lock.(*claimHandle)
					require.NoError(t, repo.store.Delete(ctx, claim.key, blob.DeleteOptions{}))
					replacement, err := repo.Lock(ctx, artifact.LockRequest{Resource: resource, Owner: "replacement"})
					require.NoError(t, err)
					t.Cleanup(func() { _ = replacement.Release(ctx) })
				}
				before := objectSnapshot(t, spec.fake)
				switch operation {
				case "sign":
					_, err = repo.Sign(ctx, subject, signer, artifact.SignOptions{Lock: lock})
				case "evidence":
					_, err = repo.WriteEvidence(ctx, &pinned, evidence, lock)
				default:
					_, err = repo.Delete(ctx, pinned, artifact.DeleteOptions{Lock: lock})
				}
				require.ErrorIs(t, err, errUtils.ErrArtifactConflict)
				assert.Equal(t, before, objectSnapshot(t, spec.fake))
				if invalid == "wrong-resource" {
					require.NoError(t, lock.Release(ctx))
				}
			})
		}
	}
}
