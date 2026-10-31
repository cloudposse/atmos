package s3

import (
	"testing"

	"github.com/cloudposse/atmos/pkg/artifact"
	"github.com/cloudposse/atmos/pkg/artifact/artifacttest"
)

func TestRepositoryConformance(t *testing.T) {
	for _, spec := range allSpecs {
		t.Run(specName(spec), func(t *testing.T) {
			signer := newSigner()
			artifacttest.RunRepositoryConformance(t, &artifacttest.Harness{
				New: func(t *testing.T) artifact.Repository {
					return newRepo(t, spec.withOpts(WithVerifier(&artifacttest.FakeVerifier{})))
				},
				Packaging:       spec.packaging,
				Signer:          signer,
				TrustedPolicy:   signer.TrustedPolicy(),
				UntrustedPolicy: signer.UntrustedPolicy(),
			})
		})
	}
}

// Without a verifier the repository advertises no evidence formats and Sign returns a capability
// error instead of a successful no-op.
func TestRepositoryConformance_WithoutVerifier(t *testing.T) {
	for _, spec := range []*rspec{noneSpec(), archiveSpec(artifact.PackagingTgz)} {
		t.Run(specName(spec), func(t *testing.T) {
			artifacttest.RunRepositoryConformance(t, &artifacttest.Harness{
				New:       func(t *testing.T) artifact.Repository { return newRepo(t, spec) },
				Packaging: spec.packaging,
			})
		})
	}
}
