package s3

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/smithy-go"
	"github.com/hashicorp/go-getter"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestS3SourceURLForms(t *testing.T) {
	for _, tc := range []struct{ uri, bucket, key, region, endpoint string }{
		{"s3://bucket/nested/template.yaml?region=us-east-2", "bucket", "nested/template.yaml", "us-east-2", ""},
		{"https://s3.us-east-2.amazonaws.com/bucket/nested/template.yaml", "bucket", "nested/template.yaml", "us-east-2", ""},
		{"https://s3.amazonaws.com/bucket/key", "bucket", "key", "us-east-1", ""},
		{"https://s3-us-west-2.amazonaws.com/bucket/key", "bucket", "key", "us-west-2", ""},
		{"https://bucket.s3.us-west-2.amazonaws.com/key", "bucket", "key", "us-west-2", ""},
		{"https://bucket.with.dots.s3-us-west-2.amazonaws.com/key", "bucket.with.dots", "key", "us-west-2", ""},
		{"https://s3.assets.s3.us-east-2.amazonaws.com/key", "s3.assets", "key", "us-east-2", ""},
		{"https://my.s3.assets.s3.us-east-2.amazonaws.com/key", "my.s3.assets", "key", "us-east-2", ""},
		{"https://s3.dualstack.us-west-2.amazonaws.com/bucket/key", "bucket", "key", "us-west-2", ""},
		{"https://s3.cn-north-1.amazonaws.com.cn/bucket/key", "bucket", "key", "cn-north-1", ""},
		{"http://localhost:1234/bucket/key?region=us-east-2", "bucket", "key", "us-east-2", "http://localhost:1234"},
	} {
		u, err := url.Parse(tc.uri)
		require.NoError(t, err)
		actual, err := parseS3SourceURL(u)
		require.NoError(t, err)
		assert.Equal(t, &sourceLocation{bucket: tc.bucket, key: tc.key, region: tc.region, endpoint: tc.endpoint}, actual)
	}
}

// Exercise real SDK signing and go-getter mode dispatch in parallel with distinct credentials.
func TestS3SourcesUseScopedCredentials(t *testing.T) {
	root := t.TempDir()
	var mu sync.Mutex
	seen := map[string][]string{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		seen[r.URL.Path] = append(seen[r.URL.Path], r.Header.Get("Authorization"))
		mu.Unlock()
		if r.URL.Query().Get("list-type") == "2" {
			w.Header().Set("Content-Type", "application/xml")
			_, _ = fmt.Fprint(w, `<ListBucketResult><IsTruncated>false</IsTruncated><Contents><Key>template.yaml</Key><Size>14</Size></Contents></ListBucketResult>`)
			return
		}
		_, _ = fmt.Fprint(w, "Resources: {}\n")
	}))
	defer server.Close()
	var wg sync.WaitGroup
	for _, account := range []string{"first", "second"} {
		account := account
		credentialFile := filepath.Join(root, account+".credentials")
		require.NoError(t, os.WriteFile(credentialFile, []byte("[source]\naws_access_key_id = "+account+"\naws_secret_access_key = synthetic\n"), 0o600))
		auth := &schema.AWSAuthContext{Profile: "source", CredentialsFile: credentialFile, ConfigFile: filepath.Join(root, "absent-config"), Region: "us-east-2"}
		wg.Add(1)
		go func() {
			defer wg.Done()
			ctx := WithAuthContext(context.Background(), auth)
			ctx, cancel := context.WithTimeout(ctx, time.Second*10)
			defer cancel()
			client := &getter.Client{Ctx: ctx, Src: "s3::" + server.URL + "/" + account + "/template.yaml?region=us-east-2", Dst: filepath.Join(root, account), Mode: getter.ClientModeAny, Getters: map[string]getter.Getter{"s3": NewGetter(ctx)}}
			err := client.Get()
			assert.NoError(t, err)
			data, err := os.ReadFile(filepath.Join(root, account, "template.yaml"))
			assert.NoError(t, err)
			assert.Equal(t, "Resources: {}\n", string(data))
		}()
	}
	wg.Wait()
	mu.Lock()
	defer mu.Unlock()
	for path, headers := range seen {
		account := strings.Split(strings.TrimPrefix(path, "/"), "/")[0]
		for _, header := range headers {
			assert.Contains(t, header, "Credential="+account+"/")
		}
	}
	assert.Contains(t, seen, "/first/template.yaml")
	assert.Contains(t, seen, "/second/template.yaml")
}

type s3SourceFixtureClient struct {
	keys    []string
	gets    int
	heads   int
	lists   int
	headErr error
	listErr error
	version string
}

func (c *s3SourceFixtureClient) HeadObject(context.Context, *s3.HeadObjectInput, ...func(*s3.Options)) (*s3.HeadObjectOutput, error) {
	c.heads++
	return &s3.HeadObjectOutput{}, c.headErr
}

func (c *s3SourceFixtureClient) ListObjectsV2(context.Context, *s3.ListObjectsV2Input, ...func(*s3.Options)) (*s3.ListObjectsV2Output, error) {
	c.lists++
	if c.listErr != nil {
		return nil, c.listErr
	}
	out := &s3.ListObjectsV2Output{}
	for _, key := range c.keys {
		if key == "" {
			continue
		}
		out.Contents = append(out.Contents, s3types.Object{Key: aws.String(key)})
	}
	return out, nil
}

func (c *s3SourceFixtureClient) GetObject(_ context.Context, input *s3.GetObjectInput, _ ...func(*s3.Options)) (*s3.GetObjectOutput, error) {
	c.gets++
	c.version = aws.ToString(input.VersionId)
	return &s3.GetObjectOutput{Body: io.NopCloser(strings.NewReader("canary"))}, nil
}

func TestS3SourceDirectoryBoundaries(t *testing.T) {
	for _, prefix := range []string{"", "prefix/"} {
		t.Run(prefix, func(t *testing.T) {
			client := &s3SourceFixtureClient{keys: []string{prefix, prefix + "nested/template.yaml"}}
			g := &Getter{ctx: t.Context(), newClient: func(context.Context, *sourceLocation) (sourceClient, error) { return client, nil }}
			u, err := url.Parse("s3://bucket/" + prefix)
			require.NoError(t, err)
			mode, err := g.ClientMode(u)
			require.NoError(t, err)
			assert.Equal(t, getter.ClientModeDir, mode)
			root := t.TempDir()
			require.NoError(t, g.Get(root, u))
			assert.FileExists(t, filepath.Join(root, "nested", "template.yaml"))
		})
	}
	for _, key := range []string{"prefix/../escape", "prefix/nested/escape"} {
		t.Run(key, func(t *testing.T) {
			root, outside := t.TempDir(), t.TempDir()
			require.NoError(t, os.Symlink(outside, filepath.Join(root, "nested")))
			client := &s3SourceFixtureClient{keys: []string{key}}
			g := &Getter{ctx: t.Context(), newClient: func(context.Context, *sourceLocation) (sourceClient, error) { return client, nil }}
			u, err := url.Parse("s3://bucket/prefix/")
			require.NoError(t, err)
			require.ErrorIs(t, g.Get(root, u), errUtils.ErrPathTraversal)
			assert.Zero(t, client.gets)
			files, err := os.ReadDir(outside)
			require.NoError(t, err)
			assert.Empty(t, files)
		})
	}
}

// TestS3ClientModeErrors preserves authorization failures and does not require
// ListBucket for exact objects or versioned downloads.
func TestS3ClientModeErrors(t *testing.T) {
	notFound := &smithy.GenericAPIError{Code: "NotFound", Message: "missing"}
	denied := &smithy.GenericAPIError{Code: "AccessDenied", Message: "denied"}
	for _, test := range []struct {
		name, key        string
		headErr, listErr error
		keys             []string
		mode             getter.ClientMode
		wantErr          error
		heads, lists     int
	}{
		{name: "object without listing permission", key: "template.yaml", listErr: denied, mode: getter.ClientModeFile, heads: 1},
		{name: "head denied", key: "template.yaml", headErr: denied, wantErr: denied, heads: 1},
		{name: "prefix", key: "prefix", headErr: notFound, keys: []string{"prefix-other", "prefix/template.yaml"}, mode: getter.ClientModeDir, heads: 1, lists: 1},
		{name: "prefix denied", key: "prefix", headErr: notFound, listErr: denied, wantErr: denied, heads: 1, lists: 1},
		{name: "object appears after head", key: "template.yaml", headErr: notFound, keys: []string{"template.yaml"}, mode: getter.ClientModeFile, heads: 1, lists: 1},
		{name: "absent", key: "missing", headErr: notFound, mode: getter.ClientModeFile, heads: 1, lists: 1},
		{name: "version", key: "template.yaml?version=recorded-version", headErr: denied, listErr: denied, mode: getter.ClientModeFile},
	} {
		t.Run(test.name, func(t *testing.T) {
			client := &s3SourceFixtureClient{keys: test.keys, headErr: test.headErr, listErr: test.listErr}
			g := &Getter{ctx: t.Context(), newClient: func(context.Context, *sourceLocation) (sourceClient, error) { return client, nil }}
			u, err := url.Parse("s3://bucket/" + test.key)
			require.NoError(t, err)
			mode, err := g.ClientMode(u)
			if test.wantErr != nil {
				require.ErrorIs(t, err, test.wantErr)
			} else {
				require.NoError(t, err)
				assert.Equal(t, test.mode, mode)
			}
			assert.Equal(t, test.heads, client.heads)
			assert.Equal(t, test.lists, client.lists)
			if test.name == "version" {
				require.NoError(t, g.GetFile(filepath.Join(t.TempDir(), "template.yaml"), u))
				assert.Equal(t, "recorded-version", client.version)
			}
		})
	}
}

// TestS3ResolverFailureNeverUsesAmbientCredentials proves scoped auth rejection
// remains fatal and is memoized for repeated stages of the same getter request.
func TestS3ResolverFailureNeverUsesAmbientCredentials(t *testing.T) {
	calls := 0
	denied := fmt.Errorf("source identity: %w", errUtils.ErrAwsCloudFormationIdentityResolutionFailed)
	ctx := WithAuthResolver(t.Context(), func(context.Context) (*schema.AWSAuthContext, error) { calls++; return nil, denied })
	g := NewGetter(ctx)
	u, err := url.Parse("s3://bucket/template.yaml")
	require.NoError(t, err)
	_, err = g.ClientMode(u)
	require.ErrorIs(t, err, denied)
	err = g.GetFile(filepath.Join(t.TempDir(), "template.yaml"), u)
	require.ErrorIs(t, err, denied)
	assert.Equal(t, 1, calls)
}

// TestS3ArchiveSubdirectory preserves go-getter archive extraction and //subdir
// selection while the custom getter handles only the authenticated object read.
func TestS3ArchiveSubdirectory(t *testing.T) {
	var archive bytes.Buffer
	compressed := gzip.NewWriter(&archive)
	tarWriter := tar.NewWriter(compressed)
	body := []byte("Resources: {}\n")
	require.NoError(t, tarWriter.WriteHeader(&tar.Header{Name: "module/template.yaml", Mode: 0o600, Size: int64(len(body))}))
	_, err := tarWriter.Write(body)
	require.NoError(t, err)
	require.NoError(t, tarWriter.Close())
	require.NoError(t, compressed.Close())
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/bucket/source.tar.gz", r.URL.Path)
		_, _ = w.Write(archive.Bytes())
	}))
	defer server.Close()
	root := t.TempDir()
	credentials := filepath.Join(root, "credentials")
	require.NoError(t, os.WriteFile(credentials, []byte("[source]\naws_access_key_id = archive\naws_secret_access_key = synthetic\n"), 0o600))
	ctx := WithAuthContext(t.Context(), &schema.AWSAuthContext{Profile: "source", CredentialsFile: credentials, ConfigFile: filepath.Join(root, "absent"), Region: "us-east-2"})
	dst := filepath.Join(root, "download")
	client := &getter.Client{Ctx: ctx, Src: "s3::" + server.URL + "/bucket/source.tar.gz//module", Dst: dst, Mode: getter.ClientModeAny, Getters: map[string]getter.Getter{"s3": NewGetter(ctx)}}
	require.NoError(t, client.Get())
	data, err := os.ReadFile(filepath.Join(dst, "template.yaml"))
	require.NoError(t, err)
	assert.Equal(t, body, data)
}

func TestS3FileRejectsDestinationSymlink(t *testing.T) {
	root := t.TempDir()
	outside := filepath.Join(root, "outside")
	require.NoError(t, os.WriteFile(outside, []byte("unchanged"), 0o600))
	dst := filepath.Join(root, "link")
	require.NoError(t, os.Symlink(outside, dst))
	client := &s3SourceFixtureClient{}
	g := &Getter{ctx: t.Context(), newClient: func(context.Context, *sourceLocation) (sourceClient, error) { return client, nil }}
	u, err := url.Parse("s3://bucket/template.yaml")
	require.NoError(t, err)
	require.ErrorIs(t, g.GetFile(dst, u), errUtils.ErrPathTraversal)
	assert.Zero(t, client.gets)
	data, err := os.ReadFile(outside)
	require.NoError(t, err)
	assert.Equal(t, "unchanged", string(data))
}

// TestS3SourceRejectsIntermediateSymlink checks slash-delimited object keys even
// when a later child is absent; Windows must inspect each key component too.
func TestS3SourceRejectsIntermediateSymlink(t *testing.T) {
	for _, tc := range []struct{ name, symlinkDir, key string }{
		{"first directory", "", "prefix/nested/missing/template.yaml"},
		{"deeper directory", "level", "prefix/level/nested/missing/template.yaml"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root, outside := t.TempDir(), t.TempDir()
			parent := filepath.Join(root, tc.symlinkDir)
			require.NoError(t, os.MkdirAll(parent, 0o755))
			require.NoError(t, os.Symlink(outside, filepath.Join(parent, "nested")))
			client := &s3SourceFixtureClient{keys: []string{tc.key}}
			g := &Getter{ctx: t.Context(), newClient: func(context.Context, *sourceLocation) (sourceClient, error) { return client, nil }}
			u, err := url.Parse("s3://bucket/prefix/")
			require.NoError(t, err)
			require.ErrorIs(t, g.Get(root, u), errUtils.ErrPathTraversal)
			assert.Zero(t, client.gets, "reject paths before retrieving object contents")
			entries, err := os.ReadDir(outside)
			require.NoError(t, err)
			assert.Empty(t, entries, "neither directories nor objects may escape the destination")
		})
	}
}

// TestS3SourceRejectsInvalidLocations ensures malformed sources fail before any
// authentication or object operation in both directory and file dispatch.
func TestS3SourceRejectsInvalidLocations(t *testing.T) {
	for _, uri := range []string{"s3:///template.yaml", "https://ec2.amazonaws.com/bucket/key"} {
		t.Run(uri, func(t *testing.T) {
			g := &Getter{ctx: t.Context(), newClient: func(context.Context, *sourceLocation) (sourceClient, error) {
				t.Error("invalid sources must not create a client")
				return nil, errUtils.ErrDownloadFile
			}}
			u, err := url.Parse(uri)
			require.NoError(t, err)
			_, err = g.ClientMode(u)
			require.ErrorIs(t, err, errUtils.ErrDownloadFile)
			require.ErrorIs(t, g.Get(t.TempDir(), u), errUtils.ErrDownloadFile)
			require.ErrorIs(t, g.GetFile(filepath.Join(t.TempDir(), "template.yaml"), u), errUtils.ErrDownloadFile)
		})
	}
}

// TestS3DirectoryListFailure preserves authorization failures without creating
// destination files or attempting object downloads.
func TestS3DirectoryListFailure(t *testing.T) {
	denied := &smithy.GenericAPIError{Code: "AccessDenied", Message: "denied"}
	client := &s3SourceFixtureClient{listErr: denied}
	g := &Getter{ctx: t.Context(), newClient: func(context.Context, *sourceLocation) (sourceClient, error) { return client, nil }}
	u, err := url.Parse("s3://bucket/prefix/")
	require.NoError(t, err)
	root := t.TempDir()
	require.ErrorIs(t, g.Get(root, u), denied)
	assert.Zero(t, client.gets)
	entries, err := os.ReadDir(root)
	require.NoError(t, err)
	assert.Empty(t, entries)
}
