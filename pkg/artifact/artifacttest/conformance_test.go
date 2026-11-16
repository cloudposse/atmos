package artifacttest_test

import (
	"context"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/artifact"
	"github.com/cloudposse/atmos/pkg/artifact/artifacttest"
)

func TestInMemoryRepositoryConformance(t *testing.T) {
	for _, packaging := range []artifact.Packaging{artifact.PackagingNone, artifact.PackagingTar, artifact.PackagingTgz, artifact.PackagingZip} {
		t.Run(string(packaging), func(t *testing.T) {
			signer := &artifacttest.FakeSigner{Identity: "release@example.com", Issuer: "https://issuer.example.com"}
			artifacttest.RunRepositoryConformance(t, &artifacttest.Harness{
				New:             func(*testing.T) artifact.Repository { return artifacttest.New(artifacttest.Options{}) },
				Packaging:       packaging,
				Signer:          signer,
				TrustedPolicy:   signer.TrustedPolicy(),
				UntrustedPolicy: signer.UntrustedPolicy(),
			})
		})
	}
}

func TestInMemoryRepositoryConformance_RestrictedCapabilities(t *testing.T) {
	caps := artifacttest.DefaultCapabilities()
	caps.Write = false
	caps.EnumerateCollection = false
	caps.Delete = artifact.DeleteNone
	caps.EvidenceFormats = nil
	artifacttest.RunRepositoryConformance(t, &artifacttest.Harness{
		New: func(*testing.T) artifact.Repository {
			return artifacttest.New(artifacttest.Options{Capabilities: &caps})
		},
	})
}

func TestInMemoryRepositoryConformance_NoEnumerationNoDelete(t *testing.T) {
	caps := artifacttest.DefaultCapabilities()
	caps.EnumerateCollection = false
	caps.EnumerateNamed = false
	caps.Delete = artifact.DeleteNone
	caps.EvidenceFormats = nil
	artifacttest.RunRepositoryConformance(t, &artifacttest.Harness{
		New: func(*testing.T) artifact.Repository {
			return artifacttest.New(artifacttest.Options{Capabilities: &caps})
		},
	})
}

func TestRepository_LockRejectsLeasesAndWrongTokens(t *testing.T) {
	repo := artifacttest.New(artifacttest.Options{})
	ctx := context.Background()
	_, err := repo.Lock(ctx, artifact.LockRequest{Resource: "r", Owner: "o", Lease: 1})
	require.ErrorIs(t, err, errUtils.ErrArtifactUnsupportedCapability)

	handle, err := repo.Lock(ctx, artifact.LockRequest{Resource: "r", Owner: "o"})
	require.NoError(t, err)
	assert.Equal(t, []string{"r"}, repo.HeldLocks())
	require.ErrorIs(t, handle.Renew(ctx, 1), errUtils.ErrArtifactUnsupportedCapability)
	_, expires := handle.ExpiresAt()
	assert.False(t, expires)
	require.NoError(t, handle.Release(ctx))
	require.ErrorIs(t, handle.Release(ctx), errUtils.ErrArtifactConflict, "a stale handle cannot release a lock it no longer owns")
	assert.Empty(t, repo.HeldLocks())
}

// deadlineRecorder notes every competing-owner Lock call made without a deadline.
type deadlineRecorder struct {
	artifact.Repository
	unbounded atomic.Int32
}

func (r *deadlineRecorder) Lock(ctx context.Context, request artifact.LockRequest) (artifact.LockHandle, error) {
	if request.Owner == "owner-b" {
		if _, ok := ctx.Deadline(); !ok {
			r.unbounded.Add(1)
		}
	}
	return r.Repository.Lock(ctx, request)
}

func TestConformance_LockAcquisitionsAreBounded(t *testing.T) {
	var recorders []*deadlineRecorder
	signer := &artifacttest.FakeSigner{Identity: "release@example.com", Issuer: "https://issuer.example.com"}
	artifacttest.RunRepositoryConformance(t, &artifacttest.Harness{
		Signer:          signer,
		TrustedPolicy:   signer.TrustedPolicy(),
		UntrustedPolicy: signer.UntrustedPolicy(),
		New: func(*testing.T) artifact.Repository {
			recorder := &deadlineRecorder{Repository: artifacttest.New(artifacttest.Options{})}
			recorders = append(recorders, recorder)
			return recorder
		},
	})
	require.NotEmpty(t, recorders)
	for _, recorder := range recorders {
		assert.Zero(t, recorder.unbounded.Load(), "a competing acquisition must carry a deadline so a leaked claim cannot hang the suite")
	}
}
