package downloader_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/cloudposse/atmos/pkg/downloader"
)

// legacyDownloaderContract freezes the public method set supported before optional
// context-aware downloading. The adapter below models an external implementation
// that delegates those methods without acquiring additional capabilities.
type legacyDownloaderContract interface {
	Fetch(string, string, downloader.ClientMode, time.Duration) error
	FetchAndAutoParse(string) (any, error)
	FetchAndParseByExtension(string) (any, error)
	FetchAndParseRaw(string) (any, error)
	FetchData(string) ([]byte, error)
	FetchAtomic(string, string, downloader.ClientMode, time.Duration) error
	FetchWithMetadata(string, string, downloader.ClientMode, time.Duration) (downloader.FetchMetadata, error)
}

type legacyDownloader struct{ legacyDownloaderContract }

var _ downloader.FileDownloader = legacyDownloader{}

func TestLegacyDownloaderRemainsUsable(t *testing.T) {
	t.Setenv("ATMOS_GITHUB_CLI", "")
	directory := t.TempDir()
	source, destination := filepath.Join(directory, "source.txt"), filepath.Join(directory, "destination.txt")
	require.NoError(t, os.WriteFile(source, []byte("legacy content"), 0o600))
	var client downloader.FileDownloader = legacyDownloader{legacyDownloaderContract: downloader.NewGoGetterDownloader(nil)}
	_, supportsContext := client.(downloader.ContextFileDownloader)
	assert.False(t, supportsContext, "legacy implementations must not be required to expose the optional capability")
	metadata, err := client.FetchWithMetadata(source, destination, downloader.ClientModeFile, time.Second)
	require.NoError(t, err)
	assert.Empty(t, metadata)
	content, err := os.ReadFile(destination)
	require.NoError(t, err)
	assert.Equal(t, "legacy content", string(content))
}

func TestBuiltInDownloadersExposeOptionalContextCapability(t *testing.T) {
	factory := downloader.NewMockClientFactory(gomock.NewController(t))
	for _, client := range []downloader.FileDownloader{downloader.NewFileDownloader(factory), downloader.NewGoGetterDownloader(nil)} {
		require.Implements(t, (*downloader.ContextFileDownloader)(nil), client)
	}
}
