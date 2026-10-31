package s3

import (
	"crypto/rand"
	"encoding/hex"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/artifact"
	"github.com/cloudposse/atmos/pkg/artifact/artifacttest"
	"github.com/cloudposse/atmos/pkg/artifact/blob/s3/s3test"
	"github.com/cloudposse/atmos/pkg/schema"
)

// TestEmulatorConformance runs the shared repository contract suite against a real S3-compatible
// endpoint (Floci, MinIO, or AWS). It is skipped unless ATMOS_BLOB_S3_ENDPOINT and
// ATMOS_BLOB_S3_BUCKET are set (ATMOS_BLOB_S3_REGION is optional); credentials come from the usual
// AWS_ACCESS_KEY_ID and AWS_SECRET_ACCESS_KEY variables and the bucket must already exist. The
// repositories talk through the production SDK client, so the run also proves that the endpoint
// enforces conditional writes (the conditional-write probe fails the run otherwise). Every subtest
// uses a unique key prefix, so runs and packagings never collide.
func TestEmulatorConformance(t *testing.T) {
	emulator := s3test.EmulatorFromEnv(t)
	for _, packaging := range []artifact.Packaging{artifact.PackagingNone, artifact.PackagingTgz} {
		t.Run(string(packaging), func(t *testing.T) {
			signer := newSigner()
			artifacttest.RunRepositoryConformance(t, &artifacttest.Harness{
				New: func(t *testing.T) artifact.Repository {
					var raw [6]byte
					_, err := rand.Read(raw[:])
					require.NoError(t, err)
					cfg := &schema.ArtifactRepositoryConfig{
						Kind: artifact.RepositoryKindS3, Packaging: string(packaging),
						Options: map[string]any{
							"bucket": emulator.Bucket, "endpoint": emulator.Endpoint, "region": emulator.Region,
							"prefix": "atmos-artifacts-conformance/" + hex.EncodeToString(raw[:]),
							"tags":   map[string]any{"purpose": "conformance"},
						},
					}
					repo, err := New("emulator", cfg, WithVerifier(&artifacttest.FakeVerifier{}), WithPartSize(5<<20))
					require.NoError(t, err)
					return repo
				},
				Packaging:       packaging,
				Signer:          signer,
				TrustedPolicy:   signer.TrustedPolicy(),
				UntrustedPolicy: signer.UntrustedPolicy(),
			})
		})
	}
}
