package local

import (
	"errors"
	"testing"

	"github.com/cloudposse/atmos/pkg/artifact"
	"github.com/cloudposse/atmos/pkg/artifact/artifacttest"
)

// errCrash is the error the fault hooks simulate a crash with.
var errCrash = errors.New("simulated crash")

func TestDirRepositoryConformance(t *testing.T) {
	signer := newSigner()
	artifacttest.RunRepositoryConformance(t, &artifacttest.Harness{
		New: func(t *testing.T) artifact.Repository {
			return newRepo(t, &repoSpec{kind: artifact.RepositoryKindLocalDir, opts: []Option{WithVerifier(&artifacttest.FakeVerifier{})}})
		},
		Packaging:       artifact.PackagingNone,
		Signer:          signer,
		TrustedPolicy:   signer.TrustedPolicy(),
		UntrustedPolicy: signer.UntrustedPolicy(),
	})
}

func TestArchiveRepositoryConformance(t *testing.T) {
	for _, packaging := range []artifact.Packaging{artifact.PackagingTar, artifact.PackagingTgz, artifact.PackagingZip} {
		t.Run(string(packaging), func(t *testing.T) {
			signer := newSigner()
			artifacttest.RunRepositoryConformance(t, &artifacttest.Harness{
				New: func(t *testing.T) artifact.Repository {
					return newRepo(t, &repoSpec{kind: artifact.RepositoryKindLocalArchive, packaging: packaging, opts: []Option{WithVerifier(&artifacttest.FakeVerifier{})}})
				},
				Packaging:       packaging,
				Signer:          signer,
				TrustedPolicy:   signer.TrustedPolicy(),
				UntrustedPolicy: signer.UntrustedPolicy(),
			})
		})
	}
}

// Without a verifier the repositories advertise no evidence formats and Sign returns a capability
// error instead of a successful no-op.
func TestRepositoryConformance_WithoutVerifier(t *testing.T) {
	for _, spec := range []*repoSpec{dirSpec(), archiveSpec(artifact.PackagingTgz)} {
		t.Run(spec.kind, func(t *testing.T) {
			artifacttest.RunRepositoryConformance(t, &artifacttest.Harness{
				New: func(t *testing.T) artifact.Repository { return newRepo(t, spec) },
			})
		})
	}
}
