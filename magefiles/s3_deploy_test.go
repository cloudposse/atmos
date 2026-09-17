//go:build mage

package main

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func writeS3TestFile(t *testing.T, root, relative, content string) string {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(relative))
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
	return path
}

func TestS3DeployContentType(t *testing.T) {
	tests := map[string]struct {
		detected string
		expected string
	}{
		"index.html":    {detected: "text/plain; charset=utf-8", expected: "text/html; charset=utf-8"},
		"assets/app.js": {detected: "text/plain; charset=utf-8", expected: "text/javascript; charset=utf-8"},
		"image.png":     {detected: "application/octet-stream", expected: "image/png"},
		"unknown.zzz":   {detected: "text/plain; charset=utf-8", expected: "text/plain; charset=utf-8"},
	}
	for path, test := range tests {
		assert.Equal(t, test.expected, s3DeployContentType(path, test.detected), path)
	}

	// Go augments its MIME table from the operating system. macOS registers
	// application/xml while Ubuntu registers the equally valid text/xml.
	assert.Contains(
		t,
		[]string{"application/xml; charset=utf-8", "text/xml; charset=utf-8"},
		s3DeployContentType("feed.xml", "text/plain; charset=utf-8"),
	)
}

func TestParseS3DeployURI(t *testing.T) {
	t.Run("bucket root", func(t *testing.T) {
		location, err := parseS3DeployURI("s3://example")
		require.NoError(t, err)
		assert.Equal(t, s3DeployLocation{Bucket: "example", URI: "s3://example/"}, location)
		assert.Equal(t, "index.html", location.objectKey("index.html"))
		assert.Empty(t, location.listPrefix())
		relative, ok := location.relativeKey("index.html")
		assert.True(t, ok)
		assert.Equal(t, "index.html", relative)
	})

	t.Run("prefix normalization", func(t *testing.T) {
		location, err := parseS3DeployURI("s3://example/pr-42")
		require.NoError(t, err)
		assert.Equal(t, "pr-42", location.Prefix)
		assert.Equal(t, "s3://example/pr-42/", location.URI)
		assert.Equal(t, "pr-42/index.html", location.objectKey("index.html"))
		assert.Equal(t, "pr-42/", location.listPrefix())
		relative, ok := location.relativeKey("pr-42/index.html")
		assert.True(t, ok)
		assert.Equal(t, "index.html", relative)
		_, ok = location.relativeKey("pr-420/index.html")
		assert.False(t, ok)
	})

	for _, value := range []string{"", "https://example", "s3:///missing-bucket"} {
		_, err := parseS3DeployURI(value)
		require.ErrorIs(t, err, errS3DeployInvalidURI)
	}
}

func TestCompileS3ProtectedPatterns(t *testing.T) {
	patterns, err := compileS3ProtectedPatterns("\n pr-* \nassets/refarch/handoffs/**\nschemas/{v1,v2}/*\n")
	require.NoError(t, err)
	require.Len(t, patterns, 3)
	assert.Equal(t, "pr-*", patterns[0].Raw)
	assert.True(t, matchesS3ProtectedPath("pr-42/assets/app.js", patterns))
	assert.True(t, matchesS3ProtectedPath("assets/refarch/handoffs/demo/file.pdf", patterns))
	assert.True(t, matchesS3ProtectedPath("schemas/v2/schema.json", patterns))
	assert.False(t, matchesS3ProtectedPath("index.html", patterns))

	_, err = compileS3ProtectedPatterns("[")
	require.Error(t, err)
}

func TestS3DeployTargetValidatesBeforeLoadingAWS(t *testing.T) {
	original := loadS3DeployClient
	loadCalls := 0
	loadS3DeployClient = func(context.Context) (s3DeployClient, error) {
		loadCalls++
		return newFakeS3DeployClient(), nil
	}
	t.Cleanup(func() { loadS3DeployClient = original })

	t.Setenv("PROTECTED_PATTERNS", "[")
	err := (S3{}).Deploy(context.Background(), t.TempDir(), "s3://example/")
	require.Error(t, err)

	t.Setenv("PROTECTED_PATTERNS", "")
	err = (S3{}).Deploy(context.Background(), filepath.Join(t.TempDir(), "missing"), "s3://example/")
	require.ErrorIs(t, err, errS3DeployInvalidLocalDir)
	assert.Zero(t, loadCalls)
}

func TestS3DeployTargetUsesSDKClient(t *testing.T) {
	root := t.TempDir()
	writeS3TestFile(t, root, "index.html", "hello\n")
	manifest, err := buildS3DeployManifest(root)
	require.NoError(t, err)
	client := newFakeS3DeployClient()
	client.getBody = mustMarshalS3Manifest(t, manifest)

	original := loadS3DeployClient
	loadS3DeployClient = func(context.Context) (s3DeployClient, error) { return client, nil }
	t.Cleanup(func() { loadS3DeployClient = original })
	t.Setenv("PROTECTED_PATTERNS", "")

	require.NoError(t, (S3{}).Deploy(context.Background(), root, "s3://example/site"))
	require.Len(t, client.getInputs, 1)
	assert.Equal(t, "site/"+s3DeployManifestName, aws.ToString(client.getInputs[0].Key))
}

func TestS3DeployTargetReportsSDKLoadError(t *testing.T) {
	original := loadS3DeployClient
	loadS3DeployClient = func(context.Context) (s3DeployClient, error) { return nil, errFakeS3 }
	t.Cleanup(func() { loadS3DeployClient = original })
	t.Setenv("PROTECTED_PATTERNS", "")

	err := (S3{}).Deploy(context.Background(), t.TempDir(), "s3://example")
	require.ErrorIs(t, err, errFakeS3)
}

func TestBuildS3DeployManifestIsContentBased(t *testing.T) {
	root := t.TempDir()
	path := writeS3TestFile(t, root, "nested/file.txt", "hello\n")
	writeS3TestFile(t, root, "notes.custom", "detected text\n")
	writeS3TestFile(t, root, s3DeployManifestName, "ignored")

	first, err := buildS3DeployManifest(root)
	require.NoError(t, err)
	require.NoError(t, os.Chtimes(path, time.Time{}, time.Now().Add(time.Hour)))
	second, err := buildS3DeployManifest(root)
	require.NoError(t, err)

	assert.Equal(t, first, second)
	assert.Equal(t, s3DeployManifestVersion, first.Version)
	require.Contains(t, first.Files, "nested/file.txt")
	assert.Equal(t, int64(6), first.Files["nested/file.txt"].Size)
	assert.Equal(t, "text/plain; charset=utf-8", first.Files["nested/file.txt"].ContentType)
	assert.NotEmpty(t, first.Files["nested/file.txt"].SHA256)
	assert.Equal(t, "text/plain; charset=utf-8", first.Files["notes.custom"].ContentType)
	assert.NotContains(t, first.Files, s3DeployManifestName)
}

func TestBuildS3DeployManifestErrors(t *testing.T) {
	_, err := buildS3DeployManifest(filepath.Join(t.TempDir(), "missing"))
	require.Error(t, err)

	_, err = s3DeployFileMetadata(filepath.Join(t.TempDir(), "missing"), "missing")
	require.Error(t, err)
}

func TestValidateS3DeployFileMode(t *testing.T) {
	require.NoError(t, validateS3DeployFileMode("regular.txt", 0))

	for _, mode := range []fs.FileMode{fs.ModeSymlink, fs.ModeNamedPipe, fs.ModeDevice, fs.ModeSocket} {
		err := validateS3DeployFileMode("unsupported", mode)
		require.ErrorIs(t, err, errS3DeployUnsupportedFile)
	}
}

func TestDiffS3DeployManifests(t *testing.T) {
	protected, err := compileS3ProtectedPatterns("img/demos/*")
	require.NoError(t, err)
	oldManifest := s3DeployManifest{Version: 1, Files: map[string]s3DeployFile{
		"same.txt":           {SHA256: "same"},
		"changed.txt":        {SHA256: "old"},
		"removed.txt":        {SHA256: "gone"},
		"img/demos/demo.mp4": {SHA256: "protected"},
	}}
	newManifest := s3DeployManifest{Version: 1, Files: map[string]s3DeployFile{
		"same.txt":    {SHA256: "same"},
		"changed.txt": {SHA256: "new"},
		"added.txt":   {SHA256: "added"},
	}}

	changed, deleted := diffS3DeployManifests(oldManifest, newManifest, protected)
	assert.Equal(t, []string{"added.txt", "changed.txt"}, changed)
	assert.Equal(t, []string{"removed.txt"}, deleted)
}
