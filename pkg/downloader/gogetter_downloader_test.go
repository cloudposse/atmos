package downloader

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/hashicorp/go-getter"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	httpClient "github.com/cloudposse/atmos/pkg/http"
)

func TestGoGetterClient_Get(t *testing.T) {
	// Setup test file
	srcDir := t.TempDir()

	testFile := filepath.Join(srcDir, "test.txt")
	err := os.WriteFile(testFile, []byte("test content"), 0o644)
	assert.NoError(t, err)

	dstDir := t.TempDir()

	// Create real go-getter client
	client := &getter.Client{
		Ctx:  context.Background(),
		Src:  testFile,
		Dst:  filepath.Join(dstDir, "test.txt"),
		Mode: getter.ClientModeFile,
	}

	gc := &goGetterClient{client: client}

	// Test the real Get implementation
	err = gc.Get()
	assert.NoError(t, err)

	// Verify file exists
	_, err = os.Stat(filepath.Join(dstDir, "test.txt"))
	assert.NoError(t, err)
}

func TestGoGetterClientFactory_NewClient(t *testing.T) {
	tests := []struct {
		name         string
		src          string
		dest         string
		mode         ClientMode
		expectedMode getter.ClientMode
	}{
		{
			name:         "Mode Any",
			src:          "source.txt",
			dest:         "dest.txt",
			mode:         ClientModeAny,
			expectedMode: getter.ClientModeAny,
		},
		{
			name:         "Mode Dir",
			src:          "source-dir",
			dest:         "dest-dir",
			mode:         ClientModeDir,
			expectedMode: getter.ClientModeDir,
		},
		{
			name:         "Mode File",
			src:          "source.txt",
			dest:         "dest.txt",
			mode:         ClientModeFile,
			expectedMode: getter.ClientModeFile,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			factory := &goGetterClientFactory{}
			ctx := context.Background()

			client, err := factory.NewClient(ctx, tt.src, tt.dest, tt.mode)
			assert.NoError(t, err)
			assert.NotNil(t, client)

			gc, ok := client.(*goGetterClient)
			assert.True(t, ok)

			assert.Equal(t, ctx, gc.client.Ctx)
			assert.Equal(t, tt.src, gc.client.Src)
			assert.Equal(t, tt.dest, gc.client.Dst)
			assert.Equal(t, tt.expectedMode, gc.client.Mode)
		})
	}
}

func TestRegisterCustomDetectors(t *testing.T) {
	// Save and restore original detectors
	originalDetectors := getter.Detectors
	defer func() {
		getter.Detectors = originalDetectors
	}()

	getter.Detectors = []getter.Detector{}

	config := &schema.AtmosConfiguration{}
	registerCustomDetectors(config, "")

	assert.Equal(t, 1, len(getter.Detectors))
	// Can't assert type precisely without NewCustomGitHubDetector implementation
	assert.NotNil(t, getter.Detectors[0])
}

func TestDownloadDetectFormatAndParseFile(t *testing.T) {
	tempDir := t.TempDir()
	testFile := filepath.Join(tempDir, "test.json")
	jsonContent := []byte(`{"key": "value"}`)
	if err := os.WriteFile(testFile, jsonContent, 0o600); err != nil {
		t.Fatal(err)
	}
	config := fakeAtmosConfig()
	result, err := NewGoGetterDownloader(&config).FetchAndAutoParse("file://" + testFile)
	if err != nil {
		t.Errorf("DownloadDetectFormatAndParseFile error: %v", err)
	}
	resMap, ok := result.(map[string]any)
	if !ok {
		t.Errorf("Expected result to be a map, got %T", result)
	} else if resMap["key"] != "value" {
		t.Errorf("Expected key to be 'value', got %v", resMap["key"])
	}
}

// TestNewClient_AttachesGitHubTokenToHTTPGetter reproduces cloudposse/atmos CI flakiness from
// GitHub-side rate limiting: sources with an explicit scheme (e.g.
// https://raw.githubusercontent.com/...) never reach CustomGitDetector — go-getter's own
// Detect() short-circuits before any Detector runs once a URL has a scheme — so the http/https
// getter must attach a GitHub token itself when one is available, mirroring what
// CustomGitDetector already does for scheme-less shorthand sources.
func TestNewClient_AttachesGitHubTokenToHTTPGetter(t *testing.T) {
	t.Setenv("GITHUB_TOKEN", "ghp_test_token")
	t.Setenv("ATMOS_GITHUB_TOKEN", "")
	t.Setenv("ATMOS_PRO_GITHUB_TOKEN", "")

	factory := &goGetterClientFactory{}
	dc, err := factory.NewClient(context.Background(), "https://raw.githubusercontent.com/org/repo/main/file.yaml", t.TempDir(), ClientModeFile)
	require.NoError(t, err)

	ggc, ok := dc.(*goGetterClient)
	require.True(t, ok)
	httpGetter, ok := ggc.client.Getters["https"].(*getter.HttpGetter)
	require.True(t, ok)
	require.NotNil(t, httpGetter.Client, "an authenticated http.Client should be attached when a GitHub token is available")

	capture, ok := httpGetter.Client.Transport.(*metadataCapturingTransport)
	require.True(t, ok, "Transport should wrap a metadataCapturingTransport, got %T", httpGetter.Client.Transport)
	transport, ok := capture.base.(*httpClient.GitHubAuthenticatedTransport)
	require.True(t, ok, "capturing transport should wrap GitHubAuthenticatedTransport, got %T", capture.base)
	assert.Equal(t, "ghp_test_token", transport.GitHubToken)
	assert.NotNil(t, httpGetter.Client.CheckRedirect, "CheckRedirect from NewGitHubAuthenticatedHTTPClient must be preserved")
	assert.Same(t, ggc.metadata, capture, "goGetterClient.Metadata() must read the same transport attached to the http getter")
}

// TestNewClient_NoTokenAttachesMetadataCapturingTransport ensures that when no GitHub token is
// resolvable and no test client was injected, the http/https getter still gets an explicit
// *http.Client wrapping a metadataCapturingTransport (over Go's default transport) instead of a
// nil Client, so ETag/Last-Modified capture works even in the plain, unauthenticated case.
func TestNewClient_NoTokenAttachesMetadataCapturingTransport(t *testing.T) {
	t.Setenv("GITHUB_TOKEN", "")
	t.Setenv("ATMOS_GITHUB_TOKEN", "")
	t.Setenv("ATMOS_PRO_GITHUB_TOKEN", "")
	// Disable the `gh auth token` CLI fallback (see pkg/github/token.go) so this test is
	// deterministic on a dev machine that happens to have an authenticated gh CLI.
	t.Setenv("ATMOS_GITHUB_CLI", "")

	factory := &goGetterClientFactory{}
	dc, err := factory.NewClient(context.Background(), "https://raw.githubusercontent.com/org/repo/main/file.yaml", t.TempDir(), ClientModeFile)
	require.NoError(t, err)

	ggc, ok := dc.(*goGetterClient)
	require.True(t, ok)
	httpGetter, ok := ggc.client.Getters["https"].(*getter.HttpGetter)
	require.True(t, ok)
	require.NotNil(t, httpGetter.Client, "an http.Client wrapping the capturing transport should always be attached")
	capture, ok := httpGetter.Client.Transport.(*metadataCapturingTransport)
	require.True(t, ok, "Transport should be metadataCapturingTransport, got %T", httpGetter.Client.Transport)
	assert.Same(t, http.DefaultTransport, capture.base, "no auth/test transport to wrap, so RoundTrip falls back to http.DefaultTransport")
}

// TestNewClient_CustomHTTPClientTakesPrecedenceOverToken ensures a caller-supplied test client
// (WithHTTPClient) is never silently overridden by token-based auth injection, and that the
// caller-owned client is wrapped (for header capture) rather than mutated in place.
func TestNewClient_CustomHTTPClientTakesPrecedenceOverToken(t *testing.T) {
	t.Setenv("GITHUB_TOKEN", "ghp_test_token")

	baseTransport := &http.Transport{}
	custom := &http.Client{Transport: baseTransport}
	factory := &goGetterClientFactory{httpClient: custom}
	dc, err := factory.NewClient(context.Background(), "https://raw.githubusercontent.com/org/repo/main/file.yaml", t.TempDir(), ClientModeFile)
	require.NoError(t, err)

	ggc, ok := dc.(*goGetterClient)
	require.True(t, ok)
	httpGetter, ok := ggc.client.Getters["https"].(*getter.HttpGetter)
	require.True(t, ok)
	require.NotNil(t, httpGetter.Client)

	// The caller's client must never be mutated in place.
	assert.Same(t, baseTransport, custom.Transport, "the caller-supplied client's Transport must not be mutated")
	assert.NotSame(t, custom, httpGetter.Client, "an injected test client must be wrapped in a distinct client, not shared")

	capture, ok := httpGetter.Client.Transport.(*metadataCapturingTransport)
	require.True(t, ok, "Transport should wrap a metadataCapturingTransport, got %T", httpGetter.Client.Transport)
	assert.Same(t, baseTransport, capture.base, "the capturing transport must wrap the caller's original transport")
}

// TestFetchWithMetadata_CapturesHTTPHeaders proves an HTTP(S) fetch through FetchWithMetadata
// against a local httptest.Server captures ETag/Last-Modified from the response -- no real
// network call involved, per CLAUDE.md's test conventions.
func TestFetchWithMetadata_CapturesHTTPHeaders(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("ETag", `"abc123"`)
		w.Header().Set("Last-Modified", "Wed, 21 Oct 2015 07:28:00 GMT")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("content"))
	}))
	defer server.Close()

	config := fakeAtmosConfig()
	dest := filepath.Join(t.TempDir(), "downloaded.txt")
	metadata, err := NewGoGetterDownloader(&config).FetchWithMetadata(server.URL+"/file.txt", dest, ClientModeFile, 10*time.Second)
	require.NoError(t, err)
	assert.Equal(t, `"abc123"`, metadata.ETag)
	assert.Equal(t, "Wed, 21 Oct 2015 07:28:00 GMT", metadata.LastModified)
	assert.FileExists(t, dest)
}

// TestFetchWithMetadata_NoHeadersReturnsEmptyMetadata proves a fetch whose response carries
// neither header returns a zero-value FetchMetadata, not an error.
func TestFetchWithMetadata_NoHeadersReturnsEmptyMetadata(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("content"))
	}))
	defer server.Close()

	config := fakeAtmosConfig()
	dest := filepath.Join(t.TempDir(), "downloaded.txt")
	metadata, err := NewGoGetterDownloader(&config).FetchWithMetadata(server.URL+"/file.txt", dest, ClientModeFile, 10*time.Second)
	require.NoError(t, err)
	assert.Empty(t, metadata.ETag)
	assert.Empty(t, metadata.LastModified)
}

// TestFetchWithMetadata_LocalFileReturnsEmptyMetadata proves a non-HTTP (local file) fetch
// yields zero-value FetchMetadata, since the "file" getter never attaches an HTTP transport.
func TestFetchWithMetadata_LocalFileReturnsEmptyMetadata(t *testing.T) {
	srcDir := t.TempDir()
	testFile := filepath.Join(srcDir, "test.txt")
	require.NoError(t, os.WriteFile(testFile, []byte("test content"), 0o644))

	config := fakeAtmosConfig()
	dest := filepath.Join(t.TempDir(), "downloaded.txt")
	metadata, err := NewGoGetterDownloader(&config).FetchWithMetadata(testFile, dest, ClientModeFile, 10*time.Second)
	require.NoError(t, err)
	assert.Empty(t, metadata.ETag)
	assert.Empty(t, metadata.LastModified)
}

// Unix-specific test moved to gogetter_downloader_unix_test.go:
// - TestGoGetterGet_File
