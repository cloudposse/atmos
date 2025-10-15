package downloader

import (
	"context"
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
