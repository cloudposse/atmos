package downloader

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"testing"

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

	transport, ok := httpGetter.Client.Transport.(*httpClient.GitHubAuthenticatedTransport)
	require.True(t, ok, "Transport should be GitHubAuthenticatedTransport, got %T", httpGetter.Client.Transport)
	assert.Equal(t, "ghp_test_token", transport.GitHubToken)
}

// TestNewClient_NoTokenLeavesHTTPGetterOnGoGetterDefault ensures that when no GitHub token is
// resolvable, the http/https getter is left with a nil Client so go-getter falls back to its
// own default (unauthenticated) client rather than an Atmos-constructed one — preserving
// go-getter's existing timeout/transport semantics for the common no-token case.
func TestNewClient_NoTokenLeavesHTTPGetterOnGoGetterDefault(t *testing.T) {
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
	assert.Nil(t, httpGetter.Client, "go-getter's own default client should be used when no token is available")
}

// TestNewClient_CustomHTTPClientTakesPrecedenceOverToken ensures a caller-supplied test client
// (WithHTTPClient) is never silently overridden by token-based auth injection.
func TestNewClient_CustomHTTPClientTakesPrecedenceOverToken(t *testing.T) {
	t.Setenv("GITHUB_TOKEN", "ghp_test_token")

	custom := &http.Client{}
	factory := &goGetterClientFactory{httpClient: custom}
	dc, err := factory.NewClient(context.Background(), "https://raw.githubusercontent.com/org/repo/main/file.yaml", t.TempDir(), ClientModeFile)
	require.NoError(t, err)

	ggc, ok := dc.(*goGetterClient)
	require.True(t, ok)
	httpGetter, ok := ggc.client.Getters["https"].(*getter.HttpGetter)
	require.True(t, ok)
	assert.Same(t, custom, httpGetter.Client, "an injected test client must take precedence over token-based auth")
}

// Unix-specific test moved to gogetter_downloader_unix_test.go:
// - TestGoGetterGet_File
