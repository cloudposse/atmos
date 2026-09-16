//go:build mage

package main

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var errFakeS3Command = errors.New("fake S3 command failed")

type fakeS3DeployRunner struct {
	manifest       *s3DeployManifest
	manifestOutput []byte
	manifestError  error
	failMatch      func([]string) bool
	outputMatch    func([]string) []byte
	inspect        func(*testing.T, []string)
	calls          [][]string
	t              *testing.T
}

func (r *fakeS3DeployRunner) Run(args ...string) ([]byte, error) {
	call := append([]string(nil), args...)
	r.calls = append(r.calls, call)
	if isS3ManifestRead(call) {
		if r.manifestError != nil {
			return r.manifestOutput, r.manifestError
		}
		data, err := json.Marshal(r.manifest)
		if err != nil {
			return nil, err
		}
		if err := os.WriteFile(call[3], data, 0o600); err != nil {
			return nil, err
		}
	}
	if r.inspect != nil {
		r.inspect(r.t, call)
	}
	if r.failMatch != nil && r.failMatch(call) {
		return []byte("forced failure"), errFakeS3Command
	}
	if r.outputMatch != nil {
		return r.outputMatch(call), nil
	}
	return nil, nil
}

func isS3ManifestRead(args []string) bool {
	return len(args) >= 4 && args[0] == "s3" && args[1] == "cp" &&
		strings.HasPrefix(args[2], "s3://") && strings.HasSuffix(args[2], s3DeployManifestName)
}

func writeS3TestFile(t *testing.T, root, relative, content string) string {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(relative))
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
	return path
}

func TestS3DeployContentType(t *testing.T) {
	tests := map[string]string{
		"index.html":    "text/html; charset=utf-8",
		"assets/app.js": "application/javascript; charset=utf-8",
		"feed.xml":      "application/xml; charset=utf-8",
		"image.png":     "image/png",
		"unknown.zzz":   "application/octet-stream",
	}
	for path, expected := range tests {
		assert.Equal(t, expected, s3DeployContentType(path), path)
	}
}

func TestParseS3DeployURI(t *testing.T) {
	t.Run("bucket root", func(t *testing.T) {
		location, err := parseS3DeployURI("s3://example")
		require.NoError(t, err)
		assert.Equal(t, s3DeployLocation{Bucket: "example", URI: "s3://example/"}, location)
	})

	t.Run("prefix normalization", func(t *testing.T) {
		location, err := parseS3DeployURI("s3://example/pr-42")
		require.NoError(t, err)
		assert.Equal(t, "pr-42", location.Prefix)
		assert.Equal(t, "s3://example/pr-42/", location.URI)
	})

	for _, value := range []string{"", "https://example", "s3:///missing-bucket"} {
		_, err := parseS3DeployURI(value)
		require.ErrorIs(t, err, errS3DeployInvalidURI)
	}
}

func TestCompileS3ProtectedPatterns(t *testing.T) {
	patterns, err := compileS3ProtectedPatterns("\n pr-* \nassets/refarch/handoffs/*\n")
	require.NoError(t, err)
	require.Len(t, patterns, 2)
	assert.Equal(t, "pr-*", patterns[0].Raw)
	assert.True(t, matchesS3ProtectedPath("pr-42/assets/app.js", patterns))
	assert.True(t, matchesS3ProtectedPath("assets/refarch/handoffs/demo/file.pdf", patterns))
	assert.False(t, matchesS3ProtectedPath("index.html", patterns))

	_, err = compileS3ProtectedPatterns("[")
	require.Error(t, err)
}

func TestS3DeployTargetValidatesInputsBeforeCallingAWS(t *testing.T) {
	t.Run("protected pattern", func(t *testing.T) {
		t.Setenv("PROTECTED_PATTERNS", "[")
		err := (S3{}).Deploy(t.TempDir(), "s3://example/")
		require.Error(t, err)
	})

	t.Run("local directory", func(t *testing.T) {
		t.Setenv("PROTECTED_PATTERNS", "")
		err := (S3{}).Deploy(filepath.Join(t.TempDir(), "missing"), "s3://example/")
		require.ErrorIs(t, err, errS3DeployInvalidLocalDir)
	})
}

func TestS3DeployAWSCLIReportsMissingExecutable(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	_, err := (s3DeployAWSCLI{}).Run("s3", "ls")
	require.Error(t, err)
}

func TestBuildS3DeployManifestIsContentBased(t *testing.T) {
	root := t.TempDir()
	path := writeS3TestFile(t, root, "nested/file.txt", "hello\n")
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
	assert.NotContains(t, first.Files, s3DeployManifestName)
}

func TestBuildS3DeployManifestErrors(t *testing.T) {
	_, err := buildS3DeployManifest(filepath.Join(t.TempDir(), "missing"))
	require.Error(t, err)

	_, err = s3DeployFileMetadata(filepath.Join(t.TempDir(), "missing"), "missing")
	require.Error(t, err)
}

func TestWriteS3DeployManifestErrors(t *testing.T) {
	err := writeS3DeployManifest(filepath.Join(t.TempDir(), "missing", "manifest.json"), s3DeployManifest{
		Version: s3DeployManifestVersion,
		Files:   map[string]s3DeployFile{},
	})
	require.Error(t, err)
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

func TestS3DeployerUnchangedPerformsNoWrites(t *testing.T) {
	root := t.TempDir()
	writeS3TestFile(t, root, "index.html", "hello\n")
	manifest, err := buildS3DeployManifest(root)
	require.NoError(t, err)
	runner := &fakeS3DeployRunner{manifest: &manifest, t: t}

	err = newS3Deployer(runner).Deploy(root, "s3://example/site/", nil)
	require.NoError(t, err)
	require.Len(t, runner.calls, 1)
	assert.True(t, isS3ManifestRead(runner.calls[0]))
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
	oldManifest := s3DeployManifest{Version: 1, Files: oldFiles}
	protected, err := compileS3ProtectedPatterns("pr-*")
	require.NoError(t, err)

	var deletedKeys []string
	var uploadedHTML bool
	var uploadedManifest bool
	runner := &fakeS3DeployRunner{manifest: &oldManifest, t: t}
	runner.inspect = func(t *testing.T, call []string) {
		if len(call) >= 8 && call[0] == "s3" && call[1] == "cp" && call[3] == "s3://example/site/" {
			if valueAfter(call, "--content-type") == "text/html; charset=utf-8" {
				uploadedHTML = true
				data, readErr := os.ReadFile(filepath.Join(call[2], "index.html"))
				require.NoError(t, readErr)
				assert.Equal(t, "new\n", string(data))
			}
		}
		if len(call) >= 6 && call[0] == "s3api" && call[1] == "delete-objects" {
			requestPath := strings.TrimPrefix(valueAfter(call, "--delete"), "file://")
			data, readErr := os.ReadFile(requestPath)
			require.NoError(t, readErr)
			var request struct {
				Objects []map[string]string `json:"Objects"`
			}
			require.NoError(t, json.Unmarshal(data, &request))
			for _, object := range request.Objects {
				deletedKeys = append(deletedKeys, object["Key"])
			}
		}
		if len(call) >= 6 && call[0] == "s3" && call[1] == "cp" && strings.HasSuffix(call[3], s3DeployManifestName) && !strings.HasPrefix(call[2], "s3://") {
			uploadedManifest = true
			assert.Equal(t, "application/json; charset=utf-8", valueAfter(call, "--content-type"))
		}
	}

	err = newS3Deployer(runner).Deploy(root, "s3://example/site/", protected)
	require.NoError(t, err)
	assert.True(t, uploadedHTML)
	assert.True(t, uploadedManifest)
	assert.Equal(t, []string{"site/removed.txt"}, deletedKeys)
	assert.NotContains(t, deletedKeys, "site/pr-42/keep.txt")
}

func TestS3DeployerBootstrapRestampsTextMetadata(t *testing.T) {
	root := t.TempDir()
	writeS3TestFile(t, root, "index.html", "hello\n")
	protected, err := compileS3ProtectedPatterns("img/demos/*")
	require.NoError(t, err)
	runner := &fakeS3DeployRunner{
		manifestError:  errFakeS3Command,
		manifestOutput: []byte("NoSuchKey"),
		t:              t,
	}

	err = newS3Deployer(runner).Deploy(root, "s3://example/", protected)
	require.NoError(t, err)
	require.Greater(t, len(runner.calls), len(s3TextContentTypes))

	var syncCalls [][]string
	var htmlRestamp, manifestUpload bool
	for _, call := range runner.calls {
		if len(call) >= 5 && call[0] == "s3" && call[1] == "sync" {
			syncCalls = append(syncCalls, call)
		}
		if len(call) >= 8 && call[0] == "s3" && call[1] == "cp" && valueAfter(call, "--include") == "*.html" {
			htmlRestamp = true
			assert.Equal(t, "REPLACE", valueAfter(call, "--metadata-directive"))
			assert.Equal(t, "text/html; charset=utf-8", valueAfter(call, "--content-type"))
		}
		if len(call) >= 4 && call[0] == "s3" && call[1] == "cp" && strings.HasSuffix(call[3], s3DeployManifestName) && !strings.HasPrefix(call[2], "s3://") {
			manifestUpload = true
		}
	}
	require.Len(t, syncCalls, 2)
	assert.False(t, slices.Contains(syncCalls[0], "--delete"))
	assert.Empty(t, valueAfter(syncCalls[0], "--exclude"))
	assert.True(t, slices.Contains(syncCalls[1], "--delete"))
	assert.Equal(t, "img/demos/*", valueAfter(syncCalls[1], "--exclude"))
	assert.True(t, htmlRestamp)
	assert.True(t, manifestUpload)
}

func TestS3DeployerBootstrapWithoutProtectedPathsUsesOneSync(t *testing.T) {
	runner := &fakeS3DeployRunner{t: t}
	err := newS3Deployer(runner).bootstrap(t.TempDir(), "s3://example/", nil)
	require.NoError(t, err)

	var syncCalls [][]string
	for _, call := range runner.calls {
		if len(call) >= 2 && call[0] == "s3" && call[1] == "sync" {
			syncCalls = append(syncCalls, call)
		}
	}
	require.Len(t, syncCalls, 1)
	assert.True(t, slices.Contains(syncCalls[0], "--delete"))
}

func TestS3DeployerValidationAndManifestErrors(t *testing.T) {
	runner := &fakeS3DeployRunner{t: t}
	deployer := newS3Deployer(runner)

	err := deployer.Deploy(filepath.Join(t.TempDir(), "missing"), "s3://example/", nil)
	require.ErrorIs(t, err, errS3DeployInvalidLocalDir)

	err = deployer.Deploy(t.TempDir(), "https://example", nil)
	require.ErrorIs(t, err, errS3DeployInvalidURI)

	root := t.TempDir()
	writeS3TestFile(t, root, "index.html", "hello")
	runner.manifestError = errFakeS3Command
	runner.manifestOutput = []byte("AccessDenied")
	err = deployer.Deploy(root, "s3://example/", nil)
	require.ErrorIs(t, err, errS3DeployAWSCommand)

	runner.manifestError = nil
	runner.manifestOutput = nil
	runner.manifest = nil
	runner.inspect = func(t *testing.T, call []string) {
		if isS3ManifestRead(call) {
			require.NoError(t, os.WriteFile(call[3], []byte("not-json"), 0o600))
		}
	}
	err = deployer.Deploy(root, "s3://example/", nil)
	require.ErrorIs(t, err, errS3DeployInvalidManifest)

	runner.inspect = nil
	runner.manifest = &s3DeployManifest{Version: 99, Files: map[string]s3DeployFile{}}
	err = deployer.Deploy(root, "s3://example/", nil)
	require.ErrorIs(t, err, errS3DeployUnsupportedState)

	runner.manifest = &s3DeployManifest{Version: s3DeployManifestVersion}
	err = deployer.Deploy(root, "s3://example/", nil)
	require.ErrorIs(t, err, errS3DeployInvalidManifest)
}

func TestS3DeployerPropagatesWriteFailure(t *testing.T) {
	root := t.TempDir()
	writeS3TestFile(t, root, "index.html", "new")
	current, err := buildS3DeployManifest(root)
	require.NoError(t, err)
	old := current
	old.Files = map[string]s3DeployFile{"index.html": {SHA256: "old"}}
	runner := &fakeS3DeployRunner{
		manifest: &old,
		failMatch: func(call []string) bool {
			return len(call) >= 4 && call[0] == "s3" && call[1] == "cp" && call[3] == "s3://example/"
		},
		t: t,
	}

	err = newS3Deployer(runner).Deploy(root, "s3://example/", nil)
	require.ErrorIs(t, err, errS3DeployAWSCommand)
}

func TestDeleteRemovedBatchesRequests(t *testing.T) {
	deleted := make([]string, s3DeleteBatchSize+1)
	for index := range deleted {
		deleted[index] = "file-" + strings.Repeat("x", index%3)
	}
	runner := &fakeS3DeployRunner{t: t}
	location := s3DeployLocation{Bucket: "example", Prefix: "site", URI: "s3://example/site/"}

	err := newS3Deployer(runner).deleteRemoved(location, deleted, t.TempDir())
	require.NoError(t, err)
	require.Len(t, runner.calls, 2)
	assert.Equal(t, "delete-objects", runner.calls[0][1])
	assert.Equal(t, "delete-objects", runner.calls[1][1])
}

func TestDeleteRemovedRejectsPerObjectErrors(t *testing.T) {
	runner := &fakeS3DeployRunner{
		t: t,
		outputMatch: func(call []string) []byte {
			if len(call) >= 2 && call[0] == "s3api" && call[1] == "delete-objects" {
				return []byte(`{"Errors":[{"Key":"site/file.txt","Code":"AccessDenied","Message":"denied"}]}`)
			}
			return nil
		},
	}
	location := s3DeployLocation{Bucket: "example", Prefix: "site", URI: "s3://example/site/"}

	err := newS3Deployer(runner).deleteRemoved(location, []string{"file.txt"}, t.TempDir())
	require.ErrorIs(t, err, errS3DeployPartialDelete)
}

func TestValidateS3DeleteResponse(t *testing.T) {
	for _, output := range [][]byte{nil, []byte("  \n"), []byte(`{}`), []byte(`{"Errors":[]}`)} {
		require.NoError(t, validateS3DeleteResponse(output))
	}

	err := validateS3DeleteResponse([]byte("not-json"))
	require.ErrorIs(t, err, errS3DeployInvalidDelete)
}

func TestLinkOrCopyS3DeployFile(t *testing.T) {
	root := t.TempDir()
	source := writeS3TestFile(t, root, "source.txt", "content")
	destination := filepath.Join(root, "destination.txt")
	require.NoError(t, linkOrCopyS3DeployFile(source, destination))
	data, err := os.ReadFile(destination)
	require.NoError(t, err)
	assert.Equal(t, "content", string(data))

	fallbackDestination := filepath.Join(root, "fallback.txt")
	require.NoError(t, os.WriteFile(fallbackDestination, []byte("existing"), 0o600))
	require.NoError(t, linkOrCopyS3DeployFile(source, fallbackDestination))
	data, err = os.ReadFile(fallbackDestination)
	require.NoError(t, err)
	assert.Equal(t, "content", string(data))

	err = linkOrCopyS3DeployFile(source, filepath.Join(root, "missing", "destination.txt"))
	require.Error(t, err)

	err = linkOrCopyS3DeployFile(filepath.Join(root, "missing"), filepath.Join(root, "other"))
	require.Error(t, err)
}

func valueAfter(args []string, flag string) string {
	for index := 0; index+1 < len(args); index++ {
		if args[index] == flag {
			return args[index+1]
		}
	}
	return ""
}
