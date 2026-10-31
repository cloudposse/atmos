package s3

import (
	"crypto/rand"
	"encoding/hex"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/artifact/blob"
	"github.com/cloudposse/atmos/pkg/artifact/blob/blobtest"
	"github.com/cloudposse/atmos/pkg/artifact/blob/s3/s3test"
)

// TestEmulatorConformance runs the shared suite against a real endpoint. It is skipped unless
// ATMOS_BLOB_S3_ENDPOINT and ATMOS_BLOB_S3_BUCKET are set (see s3test.EmulatorFromEnv); the bucket
// must already exist. Every subtest uses a unique key prefix, so runs do not collide.
func TestEmulatorConformance(t *testing.T) {
	emulator := s3test.EmulatorFromEnv(t)
	blobtest.RunStoreConformance(t, func(t *testing.T) blob.Store {
		var raw [6]byte
		_, err := rand.Read(raw[:])
		require.NoError(t, err)
		s, err := New(
			emulator.Bucket,
			WithEndpoint(emulator.Endpoint),
			WithRegion(emulator.Region),
			WithPrefix("atmos-blob-conformance/"+hex.EncodeToString(raw[:])),
			WithConditionalMode(ConditionalProbe),
			WithPartSize(5<<20),
		)
		require.NoError(t, err)
		return s
	})
}
