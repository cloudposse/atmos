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

// stubGitHubToken overrides getGitHubToken for the duration of the test.
func stubGitHubToken(t *testing.T, token string) {
	t.Helper()
	orig := getGitHubToken
	t.Cleanup(func() { getGitHubToken = orig })
	getGitHubToken = func() string { return token }
}

func TestGoGetterClientFactory_NewClient_GitHubTokenAuth(t *testing.T) {
	t.Run("github raw URL with token gets an authenticated client", func(t *testing.T) {
		stubGitHubToken(t, "test-token")

		factory := &goGetterClientFactory{}
		client, err := factory.NewClient(context.Background(), "https://raw.githubusercontent.com/org/repo/main/file.yaml", "dest.yaml", ClientModeFile)
		assert.NoError(t, err)

		gc, ok := client.(*goGetterClient)
		assert.True(t, ok)

		httpGetter, ok := gc.client.Getters["https"].(*getter.HttpGetter)
		assert.True(t, ok)
		assert.NotNil(t, httpGetter.Client, "expected an authenticated client to be attached")
	})

	t.Run("github raw URL with no token leaves the default client untouched", func(t *testing.T) {
		stubGitHubToken(t, "")

		factory := &goGetterClientFactory{}
		client, err := factory.NewClient(context.Background(), "https://raw.githubusercontent.com/org/repo/main/file.yaml", "dest.yaml", ClientModeFile)
		assert.NoError(t, err)

		gc, ok := client.(*goGetterClient)
		assert.True(t, ok)

		httpGetter, ok := gc.client.Getters["https"].(*getter.HttpGetter)
		assert.True(t, ok)
		assert.Nil(t, httpGetter.Client, "no client should be attached without a token")
	})

	t.Run("non-GitHub URL is never authenticated even when a token is available", func(t *testing.T) {
		stubGitHubToken(t, "test-token")

		factory := &goGetterClientFactory{}
		client, err := factory.NewClient(context.Background(), "https://example.com/file.yaml", "dest.yaml", ClientModeFile)
		assert.NoError(t, err)

		gc, ok := client.(*goGetterClient)
		assert.True(t, ok)

		httpGetter, ok := gc.client.Getters["https"].(*getter.HttpGetter)
		assert.True(t, ok)
		assert.Nil(t, httpGetter.Client, "non-GitHub sources must not receive the authenticated client")
	})

	t.Run("explicit custom client takes precedence over token auth", func(t *testing.T) {
		stubGitHubToken(t, "test-token")

		custom := &http.Client{}
		factory := &goGetterClientFactory{httpClient: custom}
		client, err := factory.NewClient(context.Background(), "https://raw.githubusercontent.com/org/repo/main/file.yaml", "dest.yaml", ClientModeFile)
		assert.NoError(t, err)

		gc, ok := client.(*goGetterClient)
		assert.True(t, ok)

		httpGetter, ok := gc.client.Getters["https"].(*getter.HttpGetter)
		assert.True(t, ok)
		assert.Same(t, custom, httpGetter.Client, "an explicitly configured client must win over token auth")
	})
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

// Unix-specific test moved to gogetter_downloader_unix_test.go:
// - TestGoGetterGet_File
