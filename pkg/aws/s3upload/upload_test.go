package s3upload

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/smithy-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
)

type memoryS3 struct {
	objects map[string]*s3.HeadObjectOutput
	bodies  map[string]string
	writes  int
	reads   int
	headErr error
	putErr  error
}

func (m *memoryS3) HeadObject(_ context.Context, input *s3.HeadObjectInput, _ ...func(*s3.Options)) (*s3.HeadObjectOutput, error) {
	m.reads++
	if m.headErr != nil {
		return nil, m.headErr
	}
	if obj := m.objects[aws.ToString(input.Key)]; obj != nil {
		return obj, nil
	}
	return nil, &smithy.GenericAPIError{Code: "NotFound"}
}

func (m *memoryS3) PutObject(_ context.Context, input *s3.PutObjectInput, _ ...func(*s3.Options)) (*s3.PutObjectOutput, error) {
	m.writes++
	if m.putErr != nil {
		return nil, m.putErr
	}
	body, err := io.ReadAll(input.Body)
	if err != nil {
		return nil, err
	}
	key := aws.ToString(input.Key)
	m.objects[key] = &s3.HeadObjectOutput{Metadata: input.Metadata, ContentLength: input.ContentLength, ContentType: input.ContentType, CacheControl: input.CacheControl}
	m.bodies[key] = string(body)
	return &s3.PutObjectOutput{}, nil
}

func newMemoryS3() *memoryS3 {
	return &memoryS3{objects: map[string]*s3.HeadObjectOutput{}, bodies: map[string]string{}}
}

func writeSource(t *testing.T, name, content string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(name), 0o755))
	require.NoError(t, os.WriteFile(name, []byte(content), 0o600))
}

func TestUploadIncremental(t *testing.T) {
	t.Parallel()
	source := filepath.Join(t.TempDir(), "artifact.txt")
	writeSource(t, source, "original")
	client := newMemoryS3()
	opts := Options{Source: source, Destination: "s3://artifacts/releases/"}
	first, err := Upload(t.Context(), client, opts)
	require.NoError(t, err)
	assert.Equal(t, 1, first.Uploaded)
	assert.Equal(t, []string{"s3://artifacts/releases/artifact.txt"}, first.URIs)
	second, err := Upload(t.Context(), client, opts)
	require.NoError(t, err)
	assert.Equal(t, 1, second.Unchanged)
	assert.Equal(t, 1, client.writes)
	// Same length, different bytes: size and ETag alone are insufficient.
	writeSource(t, source, "modified")
	third, err := Upload(t.Context(), client, opts)
	require.NoError(t, err)
	assert.Equal(t, 1, third.Uploaded)
	assert.Equal(t, "modified", client.bodies["releases/artifact.txt"])
	opts.CacheControl = "max-age=3600"
	_, err = Upload(t.Context(), client, opts)
	require.NoError(t, err)
	assert.Equal(t, 3, client.writes)
	opts.ContentType = "application/octet-stream"
	_, err = Upload(t.Context(), client, opts)
	require.NoError(t, err)
	assert.Equal(t, 4, client.writes)
	// Objects written without Atmos metadata must be uploaded once.
	client.objects["releases/artifact.txt"].Metadata = nil
	_, err = Upload(t.Context(), client, opts)
	require.NoError(t, err)
	assert.Equal(t, 5, client.writes)
}

func TestUploadDirectoryAndPreservesRemote(t *testing.T) {
	t.Parallel()
	source := t.TempDir()
	writeSource(t, filepath.Join(source, "a.txt"), "a")
	writeSource(t, filepath.Join(source, "nested", "b.json"), "{}")
	client := newMemoryS3()
	client.bodies["releases/old.txt"] = "keep"
	result, err := Upload(t.Context(), client, Options{Source: source, Destination: "s3://artifacts/releases"})
	require.NoError(t, err)
	assert.Equal(t, []string{"s3://artifacts/releases/a.txt", "s3://artifacts/releases/nested/b.json"}, result.URIs)
	assert.Equal(t, 2, result.Uploaded)
	assert.Equal(t, "keep", client.bodies["releases/old.txt"])
	assert.Equal(t, "application/json", aws.ToString(client.objects["releases/nested/b.json"].ContentType))
	require.NoError(t, os.Remove(filepath.Join(source, "a.txt")))
	result, err = Upload(t.Context(), client, Options{Source: source, Destination: "s3://artifacts/releases"})
	require.NoError(t, err)
	assert.Equal(t, 1, result.Unchanged)
	assert.Equal(t, "a", client.bodies["releases/a.txt"])
}

func TestUploadFailsClosed(t *testing.T) {
	t.Parallel()
	source := filepath.Join(t.TempDir(), "file")
	writeSource(t, source, "content")
	denied := &smithy.GenericAPIError{Code: "AccessDenied"}
	client := newMemoryS3()
	client.headErr = denied
	_, err := Upload(t.Context(), client, Options{Source: source, Destination: "s3://bucket/key"})
	require.ErrorIs(t, err, denied)
	assert.Zero(t, client.writes)
	client.headErr = nil
	client.putErr = denied
	_, err = Upload(t.Context(), client, Options{Source: source, Destination: "s3://bucket/key"})
	require.ErrorIs(t, err, denied)
	assert.Empty(t, client.objects)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, err = Upload(ctx, client, Options{Source: source, Destination: "s3://bucket/key"})
	assert.ErrorIs(t, err, context.Canceled)
}

func TestUploadRejectsSourcesBeforeWrites(t *testing.T) {
	t.Parallel()
	source := t.TempDir()
	writeSource(t, filepath.Join(source, "a.txt"), "a")
	require.NoError(t, os.Symlink(filepath.Join(source, "a.txt"), filepath.Join(source, "z-link")))
	client := newMemoryS3()
	_, err := Upload(t.Context(), client, Options{Source: source, Destination: "s3://bucket/"})
	require.ErrorIs(t, err, errUtils.ErrS3UploadSource)
	assert.Zero(t, client.writes)
	_, err = Upload(t.Context(), client, Options{Source: filepath.Join(source, "missing"), Destination: "s3://bucket/"})
	assert.ErrorIs(t, err, os.ErrNotExist)
}

func TestParseDestination(t *testing.T) {
	t.Parallel()
	for _, uri := range []string{"", "https://bucket/key", "s3://", "s3://user@bucket/key", "s3://bucket:9000/key", "s3://bucket/key?versionId=1", "s3://bucket/key#fragment", "s3://bucket/key?", "%zz"} {
		t.Run(uri, func(t *testing.T) {
			t.Parallel()
			_, _, err := ParseDestination(uri)
			assert.ErrorIs(t, err, errUtils.ErrS3UploadDestination)
		})
	}
	bucket, key, err := ParseDestination("s3://bucket/a%20b/%23.zip")
	require.NoError(t, err)
	assert.Equal(t, "bucket", bucket)
	assert.Equal(t, "a b/#.zip", key)
}

func TestUploadEmptyAndExtensionless(t *testing.T) {
	t.Parallel()
	source := t.TempDir()
	client := newMemoryS3()
	result, err := Upload(t.Context(), client, Options{Source: source, Destination: "s3://bucket"})
	require.NoError(t, err)
	assert.Empty(t, result.URIs)
	writeSource(t, filepath.Join(source, "empty"), "")
	_, err = Upload(t.Context(), client, Options{Source: source, Destination: "s3://bucket"})
	require.NoError(t, err)
	assert.Equal(t, "", client.bodies["empty"])
	assert.False(t, isMissing(errors.New("404")))
}

func TestUploadEscapesObjectURIs(t *testing.T) {
	t.Parallel()
	source := t.TempDir()
	writeSource(t, filepath.Join(source, "artifact #1?.zip"), "archive")
	client := newMemoryS3()
	result, err := Upload(t.Context(), client, Options{Source: source, Destination: "s3://bucket/release%20files/"})
	require.NoError(t, err)
	require.Equal(t, []string{"s3://bucket/release%20files/artifact%20%231%3F.zip"}, result.URIs)
	bucket, key, err := ParseDestination(result.URIs[0])
	require.NoError(t, err)
	assert.Equal(t, "bucket", bucket)
	assert.Equal(t, "release files/artifact #1?.zip", key)
	assert.Equal(t, "archive", client.bodies[key])
}
