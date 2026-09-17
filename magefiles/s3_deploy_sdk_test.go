//go:build mage

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/smithy-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var errFakeS3 = errors.New("fake S3 failure")

type capturedS3Put struct {
	bucket      string
	key         string
	contentType string
	body        []byte
}

type fakeS3DeployClient struct {
	getBody       []byte
	getErr        error
	getNilOutput  bool
	putErrForKey  map[string]error
	listOutputs   []*s3.ListObjectsV2Output
	listErr       error
	deleteOutputs []*s3.DeleteObjectsOutput
	deleteErr     error
	getInputs     []*s3.GetObjectInput
	puts          []capturedS3Put
	listInputs    []*s3.ListObjectsV2Input
	deleteInputs  []*s3.DeleteObjectsInput
}

func newFakeS3DeployClient() *fakeS3DeployClient {
	return &fakeS3DeployClient{putErrForKey: map[string]error{}}
}

func (f *fakeS3DeployClient) GetObject(
	_ context.Context,
	input *s3.GetObjectInput,
	_ ...func(*s3.Options),
) (*s3.GetObjectOutput, error) {
	f.getInputs = append(f.getInputs, input)
	if f.getErr != nil {
		return nil, f.getErr
	}
	if f.getNilOutput {
		return nil, nil
	}
	return &s3.GetObjectOutput{Body: io.NopCloser(bytes.NewReader(f.getBody))}, nil
}

func (f *fakeS3DeployClient) PutObject(
	_ context.Context,
	input *s3.PutObjectInput,
	_ ...func(*s3.Options),
) (*s3.PutObjectOutput, error) {
	key := aws.ToString(input.Key)
	body, err := io.ReadAll(input.Body)
	if err != nil {
		return nil, err
	}
	f.puts = append(f.puts, capturedS3Put{
		bucket:      aws.ToString(input.Bucket),
		key:         key,
		contentType: aws.ToString(input.ContentType),
		body:        body,
	})
	if putErr := f.putErrForKey[key]; putErr != nil {
		return nil, putErr
	}
	return &s3.PutObjectOutput{}, nil
}

func (f *fakeS3DeployClient) ListObjectsV2(
	_ context.Context,
	input *s3.ListObjectsV2Input,
	_ ...func(*s3.Options),
) (*s3.ListObjectsV2Output, error) {
	f.listInputs = append(f.listInputs, input)
	if f.listErr != nil {
		return nil, f.listErr
	}
	index := len(f.listInputs) - 1
	if index < len(f.listOutputs) {
		return f.listOutputs[index], nil
	}
	return &s3.ListObjectsV2Output{}, nil
}

func (f *fakeS3DeployClient) DeleteObjects(
	_ context.Context,
	input *s3.DeleteObjectsInput,
	_ ...func(*s3.Options),
) (*s3.DeleteObjectsOutput, error) {
	f.deleteInputs = append(f.deleteInputs, input)
	if f.deleteErr != nil {
		return nil, f.deleteErr
	}
	index := len(f.deleteInputs) - 1
	if index < len(f.deleteOutputs) {
		return f.deleteOutputs[index], nil
	}
	return &s3.DeleteObjectsOutput{}, nil
}

func mustMarshalS3Manifest(t *testing.T, manifest s3DeployManifest) []byte {
	t.Helper()
	data, err := json.Marshal(manifest)
	require.NoError(t, err)
	return data
}

func findS3Put(t *testing.T, puts []capturedS3Put, key string) capturedS3Put {
	t.Helper()
	for _, put := range puts {
		if put.key == key {
			return put
		}
	}
	t.Fatalf("missing S3 PUT for %s", key)
	return capturedS3Put{}
}

func deletedS3Keys(inputs []*s3.DeleteObjectsInput) []string {
	keys := make([]string, 0)
	for _, input := range inputs {
		for _, object := range input.Delete.Objects {
			keys = append(keys, aws.ToString(object.Key))
		}
	}
	return keys
}

func TestS3DeployerUnchangedPerformsNoWrites(t *testing.T) {
	root := t.TempDir()
	writeS3TestFile(t, root, "index.html", "hello\n")
	manifest, err := buildS3DeployManifest(root)
	require.NoError(t, err)
	client := newFakeS3DeployClient()
	client.getBody = mustMarshalS3Manifest(t, manifest)

	err = newS3Deployer(client).deployPaths(context.Background(), root, "s3://example/site/", nil)
	require.NoError(t, err)
	require.Len(t, client.getInputs, 1)
	assert.Empty(t, client.puts)
	assert.Empty(t, client.deleteInputs)
}

func TestS3DeployerIncrementalUploadPreservesMetadata(t *testing.T) {
	root := t.TempDir()
	writeS3TestFile(t, root, "index.html", "new\n")
	writeS3TestFile(t, root, "image.png", "png")
	current, err := buildS3DeployManifest(root)
	require.NoError(t, err)
	oldFiles := map[string]s3DeployFile{}
	for path, metadata := range current.Files {
		oldFiles[path] = metadata
	}
	oldFiles["index.html"] = s3DeployFile{SHA256: "old", Size: 4, ContentType: "text/html; charset=utf-8"}
	oldFiles["removed.txt"] = s3DeployFile{SHA256: "gone"}
	oldFiles["pr-42/keep.txt"] = s3DeployFile{SHA256: "protected"}
	protected, err := compileS3ProtectedPatterns("pr-*")
	require.NoError(t, err)
	client := newFakeS3DeployClient()
	client.getBody = mustMarshalS3Manifest(t, s3DeployManifest{Version: 1, Files: oldFiles})

	err = newS3Deployer(client).deployPaths(context.Background(), root, "s3://example/site/", protected)
	require.NoError(t, err)
	require.Len(t, client.puts, 2)
	html := findS3Put(t, client.puts, "site/index.html")
	assert.Equal(t, "example", html.bucket)
	assert.Equal(t, "text/html; charset=utf-8", html.contentType)
	assert.Equal(t, "new\n", string(html.body))
	manifestPut := findS3Put(t, client.puts, "site/"+s3DeployManifestName)
	assert.Equal(t, s3ManifestContentType, manifestPut.contentType)
	var uploadedManifest s3DeployManifest
	require.NoError(t, json.Unmarshal(manifestPut.body, &uploadedManifest))
	assert.Contains(t, uploadedManifest.Files, "pr-42/keep.txt")
	assert.Equal(t, []string{"site/removed.txt"}, deletedS3Keys(client.deleteInputs))
}

func TestS3DeployerBootstrapUsesSDKForListingAndCleanup(t *testing.T) {
	root := t.TempDir()
	writeS3TestFile(t, root, "index.html", "hello\n")
	protected, err := compileS3ProtectedPatterns("img/demos/*")
	require.NoError(t, err)
	client := newFakeS3DeployClient()
	client.getErr = &s3types.NoSuchKey{}
	client.listOutputs = []*s3.ListObjectsV2Output{
		{
			Contents: []s3types.Object{
				{Key: aws.String("site/index.html")},
				{Key: aws.String("site/stale.txt")},
				{Key: aws.String("site/img/demos/demo.mp4")},
				{Key: aws.String("site/" + s3DeployManifestName)},
			},
			IsTruncated:           aws.Bool(true),
			NextContinuationToken: aws.String("page-2"),
		},
		{Contents: []s3types.Object{{Key: aws.String("site/z-stale.txt")}}},
	}

	err = newS3Deployer(client).deployPaths(context.Background(), root, "s3://example/site", protected)
	require.NoError(t, err)
	assert.Equal(t, "site/", aws.ToString(client.listInputs[0].Prefix))
	assert.Nil(t, client.listInputs[0].ContinuationToken)
	assert.Equal(t, "page-2", aws.ToString(client.listInputs[1].ContinuationToken))
	assert.Equal(t, []string{"site/stale.txt", "site/z-stale.txt"}, deletedS3Keys(client.deleteInputs))
	html := findS3Put(t, client.puts, "site/index.html")
	assert.Equal(t, "text/html; charset=utf-8", html.contentType)
	manifestPut := findS3Put(t, client.puts, "site/"+s3DeployManifestName)
	var uploadedManifest s3DeployManifest
	require.NoError(t, json.Unmarshal(manifestPut.body, &uploadedManifest))
	assert.Equal(t, s3DeployFile{Protected: true}, uploadedManifest.Files["img/demos/demo.mp4"])
}

func TestUploadChangedRequiresManifestMetadata(t *testing.T) {
	client := newFakeS3DeployClient()
	err := newS3Deployer(client).uploadChanged(
		context.Background(),
		t.TempDir(),
		s3DeployLocation{Bucket: "example"},
		[]string{"missing.txt"},
		s3DeployManifest{Version: s3DeployManifestVersion, Files: map[string]s3DeployFile{}},
	)
	require.ErrorIs(t, err, errS3DeployMissingMetadata)
}

func TestS3DeployerValidationAndManifestErrors(t *testing.T) {
	deployer := newS3Deployer(newFakeS3DeployClient())
	err := deployer.deployPaths(context.Background(), filepath.Join(t.TempDir(), "missing"), "s3://example/", nil)
	require.ErrorIs(t, err, errS3DeployInvalidLocalDir)
	err = deployer.deployPaths(context.Background(), t.TempDir(), "https://example", nil)
	require.ErrorIs(t, err, errS3DeployInvalidURI)

	root := t.TempDir()
	writeS3TestFile(t, root, "index.html", "hello")
	tests := []struct {
		name     string
		body     []byte
		getErr   error
		nilOut   bool
		expected error
	}{
		{name: "access denied", getErr: errFakeS3, expected: errS3DeployAWSOperation},
		{name: "nil response", nilOut: true, expected: errS3DeployInvalidManifest},
		{name: "invalid JSON", body: []byte("not-json"), expected: errS3DeployInvalidManifest},
		{name: "unsupported version", body: mustMarshalS3Manifest(t, s3DeployManifest{Version: 99, Files: map[string]s3DeployFile{}}), expected: errS3DeployUnsupportedState},
		{name: "missing files", body: mustMarshalS3Manifest(t, s3DeployManifest{Version: 1}), expected: errS3DeployInvalidManifest},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			client := newFakeS3DeployClient()
			client.getBody = test.body
			client.getErr = test.getErr
			client.getNilOutput = test.nilOut
			err := newS3Deployer(client).deployPaths(context.Background(), root, "s3://example/", nil)
			require.ErrorIs(t, err, test.expected)
		})
	}
}

func TestS3DeployerPropagatesPutFailure(t *testing.T) {
	root := t.TempDir()
	writeS3TestFile(t, root, "index.html", "new")
	client := newFakeS3DeployClient()
	client.getBody = mustMarshalS3Manifest(t, s3DeployManifest{Version: 1, Files: map[string]s3DeployFile{
		"index.html": {SHA256: "old"},
	}})
	client.putErrForKey["index.html"] = errFakeS3

	err := newS3Deployer(client).deployPaths(context.Background(), root, "s3://example/", nil)
	require.ErrorIs(t, err, errS3DeployAWSOperation)
}

func TestDeleteRemovedBatchesAndRejectsFailures(t *testing.T) {
	deleted := make([]string, s3DeleteBatchSize+1)
	for index := range deleted {
		deleted[index] = "file-" + strings.Repeat("x", index%3)
	}
	location := s3DeployLocation{Bucket: "example", Prefix: "site"}
	client := newFakeS3DeployClient()
	require.NoError(t, newS3Deployer(client).deleteRemoved(context.Background(), location, deleted))
	require.Len(t, client.deleteInputs, 2)
	assert.Len(t, client.deleteInputs[0].Delete.Objects, s3DeleteBatchSize)
	assert.Len(t, client.deleteInputs[1].Delete.Objects, 1)

	client = newFakeS3DeployClient()
	client.deleteOutputs = []*s3.DeleteObjectsOutput{{Errors: []s3types.Error{{
		Key: aws.String("site/file.txt"), Code: aws.String("AccessDenied"), Message: aws.String("denied"),
	}}}}
	err := newS3Deployer(client).deleteRemoved(context.Background(), location, []string{"file.txt"})
	require.ErrorIs(t, err, errS3DeployPartialDelete)

	client = newFakeS3DeployClient()
	client.deleteErr = errFakeS3
	err = newS3Deployer(client).deleteRemoved(context.Background(), location, []string{"file.txt"})
	require.ErrorIs(t, err, errS3DeployAWSOperation)
}

func TestListStaleObjectsRejectsInvalidPagination(t *testing.T) {
	location := s3DeployLocation{Bucket: "example", Prefix: "site"}
	manifest := s3DeployManifest{Version: 1, Files: map[string]s3DeployFile{}}
	client := newFakeS3DeployClient()
	client.listErr = errFakeS3
	_, err := newS3Deployer(client).listStaleObjects(context.Background(), location, &manifest, nil)
	require.ErrorIs(t, err, errS3DeployAWSOperation)

	client = newFakeS3DeployClient()
	client.listOutputs = []*s3.ListObjectsV2Output{nil}
	_, err = newS3Deployer(client).listStaleObjects(context.Background(), location, &manifest, nil)
	require.ErrorIs(t, err, errS3DeployAWSOperation)

	client = newFakeS3DeployClient()
	client.listOutputs = []*s3.ListObjectsV2Output{{IsTruncated: aws.Bool(true)}}
	_, err = newS3Deployer(client).listStaleObjects(context.Background(), location, &manifest, nil)
	require.ErrorIs(t, err, errS3DeployAWSOperation)
}

func TestIsS3ManifestNotFound(t *testing.T) {
	assert.True(t, isS3ManifestNotFound(&s3types.NoSuchKey{}))
	assert.True(t, isS3ManifestNotFound(&smithy.GenericAPIError{Code: s3ManifestNotFoundErrorCode}))
	assert.False(t, isS3ManifestNotFound(errFakeS3))
}
