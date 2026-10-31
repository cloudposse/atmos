package s3

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/artifact/blob/s3/s3test"
	"github.com/cloudposse/atmos/pkg/ci/artifact"
)

// newFakeStore returns the legacy adapter over a fake S3 endpoint, using the SDK's default
// checksum behavior like production.
func newFakeStore(t *testing.T, prefix string) (*Store, *s3test.Server) {
	t.Helper()
	fake := s3test.NewServer(t)
	return &Store{client: fake.LegacyClient(), bucket: fake.Bucket(), prefix: prefix}, fake
}

func TestAdapterKeepsKeyPatternAndSidecarFormat(t *testing.T) {
	store, fake := newFakeStore(t, "planfiles")
	created := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	metadata := &artifact.Metadata{Stack: "dev", Component: "vpc", SHA: "abc", CreatedAt: created, Custom: map[string]string{"k": "v"}}
	body := []byte("tar-bytes\x00\x01")

	require.NoError(t, store.Upload(context.Background(), "dev/vpc/abc.tar", bytes.NewReader(body), int64(len(body)), metadata))

	assert.Equal(t, []string{"planfiles/dev/vpc/abc.tar", "planfiles/dev/vpc/abc.tar.metadata.json"}, fake.Keys())
	data, ok := fake.Object("planfiles/dev/vpc/abc.tar")
	require.True(t, ok)
	assert.Equal(t, body, data.Data)
	assert.Empty(t, data.Tags, "the legacy adapter adds no object tags")

	sidecar, ok := fake.Object("planfiles/dev/vpc/abc.tar.metadata.json")
	require.True(t, ok)
	want, err := json.MarshalIndent(metadata, "", "  ")
	require.NoError(t, err)
	assert.Equal(t, want, sidecar.Data, "the sidecar is byte-identical to the legacy encoding")
	assert.Equal(t, "application/json", sidecar.ContentType)

	puts := fake.RequestsFor(s3test.OpPutObject)
	require.Len(t, puts, 2, "one request per object, data first and sidecar second")
	assert.Equal(t, "planfiles/dev/vpc/abc.tar", puts[0].Key)
	assert.Equal(t, "planfiles/dev/vpc/abc.tar.metadata.json", puts[1].Key)
	for _, put := range puts {
		assert.Empty(t, put.Header.Get("If-None-Match"), "legacy writes stay unconditional")
		assert.Empty(t, put.Header.Get("X-Amz-Tagging"))
	}
}

func TestAdapterLegacyKeyQuirksArePreserved(t *testing.T) {
	store, fake := newFakeStore(t, "pre/fix/")
	require.NoError(t, store.Upload(context.Background(), "a//b/../c.tar", strings.NewReader("x"), 1, nil))
	assert.Equal(t, []string{"pre/fix/a/c.tar"}, fake.Keys(), "path.Join cleaning of prefix and key is unchanged; nil metadata writes no sidecar")
}

func TestAdapterRoundTripWithoutPrefix(t *testing.T) {
	store, fake := newFakeStore(t, "")
	ctx := context.Background()
	metadata := &artifact.Metadata{Stack: "s", Component: "c", SHA: "1"}
	require.NoError(t, store.Upload(ctx, "k.tar", strings.NewReader("payload"), 7, metadata))
	assert.Equal(t, []string{"k.tar", "k.tar.metadata.json"}, fake.Keys())

	reader, got, err := store.Download(ctx, "k.tar")
	require.NoError(t, err)
	var buf bytes.Buffer
	_, err = buf.ReadFrom(reader)
	require.NoError(t, err)
	require.NoError(t, reader.Close())
	assert.Equal(t, "payload", buf.String())
	require.NotNil(t, got)
	assert.Equal(t, "s", got.Stack)

	exists, err := store.Exists(ctx, "k.tar")
	require.NoError(t, err)
	assert.True(t, exists)
	exists, err = store.Exists(ctx, "missing.tar")
	require.NoError(t, err)
	assert.False(t, exists)

	_, _, err = store.Download(ctx, "missing.tar")
	require.ErrorIs(t, err, errUtils.ErrArtifactNotFound)
	_, err = store.GetMetadata(ctx, "missing.tar")
	require.ErrorIs(t, err, errUtils.ErrArtifactNotFound)
}

func TestAdapterDownloadWithoutSidecarReturnsNilMetadata(t *testing.T) {
	store, fake := newFakeStore(t, "")
	fake.Seed("bare.tar", []byte("data"), nil)
	reader, meta, err := store.Download(context.Background(), "bare.tar")
	require.NoError(t, err)
	require.NoError(t, reader.Close())
	assert.Nil(t, meta)

	minimal, err := store.GetMetadata(context.Background(), "bare.tar")
	require.NoError(t, err)
	assert.False(t, minimal.CreatedAt.IsZero(), "falls back to the object's modification time")
}

func TestAdapterDeleteRemovesObjectAndSidecarAndIsIdempotent(t *testing.T) {
	store, fake := newFakeStore(t, "p")
	ctx := context.Background()
	require.NoError(t, store.Upload(ctx, "a.tar", strings.NewReader("x"), 1, &artifact.Metadata{}))
	require.NoError(t, store.Delete(ctx, "a.tar"))
	assert.Empty(t, fake.Keys())
	require.NoError(t, store.Delete(ctx, "a.tar"))
}

func TestAdapterListHandlesPrefixSidecarsAndPagination(t *testing.T) {
	store, fake := newFakeStore(t, "plans")
	const total = 1105
	for i := range total {
		key := fmt.Sprintf("plans/dev/c%04d.tar", i)
		fake.Seed(key, []byte("x"), nil)
		fake.Seed(key+metadataSuffix, []byte(fmt.Sprintf(`{"stack":"dev","component":"c%04d","sha":"s"}`, i)), nil)
	}
	fake.Seed("plans/prod/other.tar", []byte("x"), nil)
	fake.Seed("elsewhere/dev/x.tar", []byte("x"), nil)

	infos, err := store.List(context.Background(), artifact.Query{Stacks: []string{"dev"}})
	require.NoError(t, err)
	require.Len(t, infos, total, "sidecars are skipped and every page is consumed")
	assert.Equal(t, "dev/c0000.tar", infos[0].Name, "names are relative to the prefix")
	assert.Equal(t, "dev/c1104.tar", infos[total-1].Name)
	require.NotNil(t, infos[0].Metadata)
	assert.Equal(t, "c0000", infos[0].Metadata.Component)
	assert.Equal(t, int64(1), infos[0].Size)
	assert.False(t, infos[0].LastModified.IsZero())
}

func TestAdapterBundledStoreChecksumAndTarSurviveTheTransport(t *testing.T) {
	store, fake := newFakeStore(t, "ci")
	bundled := artifact.NewBundledStore(store)
	files := []artifact.FileEntry{{Name: "plan.tfplan", Data: strings.NewReader("plan-bytes"), Size: 10}}
	require.NoError(t, bundled.Upload(context.Background(), "dev/vpc/sha.tar", files, &artifact.Metadata{Stack: "dev", Component: "vpc", SHA: "sha"}))

	stored, ok := fake.Object("ci/dev/vpc/sha.tar")
	require.True(t, ok)
	sum := sha256.Sum256(stored.Data)
	meta, err := bundled.GetMetadata(context.Background(), "dev/vpc/sha.tar")
	require.NoError(t, err)
	assert.Equal(t, hex.EncodeToString(sum[:]), meta.SHA256)

	got, _, err := bundled.Download(context.Background(), "dev/vpc/sha.tar")
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "plan.tfplan", got[0].Name)
}

func TestAdapterLargeUploadStreamsAsMultipartWithLegacyClient(t *testing.T) {
	store, fake := newFakeStore(t, "")
	body := bytes.Repeat([]byte("0123456789abcdef"), (9<<20)/16)
	require.NoError(t, store.Upload(context.Background(), "big.tar", bytes.NewReader(body), int64(len(body)), nil))

	stored, ok := fake.Object("big.tar")
	require.True(t, ok)
	assert.Equal(t, len(body), len(stored.Data))
	assert.True(t, bytes.Equal(body, stored.Data))
	assert.Equal(t, 1, fake.CompletedUploads())
	assert.Empty(t, fake.OpenUploads())
	for _, req := range fake.RequestsFor(s3test.OpUploadPart) {
		assert.Empty(t, req.Header.Get("X-Amz-Checksum-Crc32"), "multipart requests carry no optional checksums")
	}
}

func TestAdapterWrapsTransportFailuresInLegacySentinels(t *testing.T) {
	store, fake := newFakeStore(t, "")
	ctx := context.Background()
	fake.Inject(s3test.Fault{Op: s3test.OpPutObject, Status: 403, Code: "AccessDenied", Times: -1})
	err := store.Upload(ctx, "k.tar", strings.NewReader("x"), 1, nil)
	require.ErrorIs(t, err, errUtils.ErrArtifactUploadFailed)

	fake.ClearFaults()
	fake.Inject(s3test.Fault{Op: s3test.OpGetObject, Status: 403, Code: "AccessDenied", Times: -1})
	_, _, err = store.Download(ctx, "k.tar")
	require.ErrorIs(t, err, errUtils.ErrArtifactDownloadFailed)

	fake.ClearFaults()
	fake.Inject(s3test.Fault{Op: s3test.OpListObjectsV2, Status: 403, Code: "AccessDenied", Times: -1})
	_, err = store.List(ctx, artifact.Query{All: true})
	require.ErrorIs(t, err, errUtils.ErrArtifactListFailed)

	fake.ClearFaults()
	fake.Inject(s3test.Fault{Op: s3test.OpHeadObject, Status: 403, Code: "Forbidden", Times: -1})
	_, err = store.Exists(ctx, "k.tar")
	require.ErrorIs(t, err, errUtils.ErrArtifactListFailed)
	_, err = store.GetMetadata(ctx, "k.tar")
	require.ErrorIs(t, err, errUtils.ErrArtifactMetadataFailed)

	fake.ClearFaults()
	fake.Inject(s3test.Fault{Op: s3test.OpDeleteObject, Status: 403, Code: "AccessDenied", Times: -1})
	require.ErrorIs(t, store.Delete(ctx, "k.tar"), errUtils.ErrArtifactDeleteFailed)
}
