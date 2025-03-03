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

// Test helper to create a temporary directory.
func createTempDir(t *testing.T) string {
	dir, err := os.MkdirTemp("", "test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	return dir
}

func TestGoGetterClient_Get(t *testing.T) {
	// Setup test file
	srcDir := createTempDir(t)
	defer os.RemoveAll(srcDir)

	testFile := filepath.Join(srcDir, "test.txt")
	err := os.WriteFile(testFile, []byte("test content"), 0o600)
	assert.NoError(t, err)

	dstDir := createTempDir(t)
	defer os.RemoveAll(dstDir)

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
	registerCustomDetectors(config)

	assert.Equal(t, 1, len(getter.Detectors))
	assert.IsType(t, &customGitHubDetector{}, getter.Detectors[0])
}

func TestNewGoGetterDownloader(t *testing.T) {
	// Save and restore original detectors
	originalDetectors := getter.Detectors
	defer func() {
		getter.Detectors = originalDetectors
	}()

	getter.Detectors = []getter.Detector{}

	config := &schema.AtmosConfiguration{}
	downloader := NewGoGetterDownloader(config)

	assert.NotNil(t, downloader)
	assert.Equal(t, 1, len(getter.Detectors))
}
