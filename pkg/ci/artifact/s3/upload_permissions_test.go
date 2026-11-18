package s3

import (
	"bytes"
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/artifact/blob/s3/s3test"
	"github.com/cloudposse/atmos/pkg/ci/artifact"
)

const (
	partSize = 8 << 20
	denied   = 403
)

// denyEverythingButPutObject models a role that may only write objects. On an SSE-KMS bucket such
// a role holds s3:PutObject and kms:GenerateDataKey but not kms:Decrypt, so every operation that
// reads an object back, and every multipart operation, is refused.
func denyEverythingButPutObject(server *s3test.Server) {
	for _, op := range []string{
		s3test.OpGetObject,
		s3test.OpHeadObject,
		s3test.OpGetObjectTagging,
		s3test.OpListObjectsV2,
		s3test.OpDeleteObject,
		s3test.OpCreateMultipartUpload,
		s3test.OpUploadPart,
		s3test.OpCompleteMultipartUpload,
		s3test.OpAbortMultipartUpload,
	} {
		server.Inject(s3test.Fault{Op: op, Times: -1, Action: s3test.FailStatus, Status: denied, Code: "AccessDenied"})
	}
}

func patterned(n int) []byte {
	data := make([]byte, n)
	for i := range data {
		data[i] = byte(i*7 + i/251)
	}
	return data
}

// An upload that worked with the previous single-PutObject adapter must keep working for a role
// that can only put objects, whatever the artifact size, including sizes past the multipart
// threshold. The adapter may only issue PutObject requests.
func TestUploadNeedsOnlyPutObjectPermission(t *testing.T) {
	tests := []struct {
		name string
		size int
	}{
		{name: "empty", size: 0},
		{name: "small", size: 1 << 10},
		{name: "exactly one part", size: partSize},
		{name: "one byte past one part", size: partSize + 1},
		{name: "multiple parts", size: partSize*2 + 123},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := s3test.NewServer(t)
			denyEverythingButPutObject(server)
			store := &Store{client: server.Client(), bucket: server.Bucket(), prefix: "planfiles"}
			data := patterned(tt.size)
			metadata := &artifact.Metadata{Stack: "dev", Component: "vpc", SHA: "abc"}

			err := store.Upload(context.Background(), "dev/vpc/abc.tfplan", bytes.NewReader(data), int64(len(data)), metadata)

			require.NoError(t, err)
			stored, ok := server.Object("planfiles/dev/vpc/abc.tfplan")
			require.True(t, ok)
			assert.True(t, bytes.Equal(data, stored.Data), "stored %d bytes, want %d", len(stored.Data), len(data))
			_, ok = server.Object("planfiles/dev/vpc/abc.tfplan.metadata.json")
			assert.True(t, ok, "the metadata sidecar is uploaded")
			for _, request := range server.Requests() {
				assert.Equal(t, s3test.OpPutObject, request.Op, "request %d", request.Seq)
				assert.Empty(t, request.Query.Get("uploadId"), "no multipart request")
			}
			assert.Empty(t, server.OpenUploads())
		})
	}
}

// Refusing an upload for lack of PutObject permission is reported, not hidden by a fallback.
func TestUploadReportsPutObjectDenied(t *testing.T) {
	server := s3test.NewServer(t)
	server.Inject(s3test.Fault{Op: s3test.OpPutObject, Times: -1, Action: s3test.FailStatus, Status: denied, Code: "AccessDenied"})
	store := &Store{client: server.Client(), bucket: server.Bucket()}

	err := store.Upload(context.Background(), "k", bytes.NewReader(patterned(partSize+1)), -1, nil)

	require.Error(t, err)
	assert.Empty(t, server.Keys())
}
