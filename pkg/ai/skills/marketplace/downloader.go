package marketplace

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"time"

	"github.com/cloudposse/atmos/pkg/downloader"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// Downloader handles downloading skills from Git repositories.
type Downloader struct{}

// NewDownloader creates a new downloader instance.
func NewDownloader() *Downloader {
	return &Downloader{}
}

// Download clones a Git repository to a temporary directory, or, for a "local"
// source (see ParseSource), copies it from disk instead. Either way it returns the
// path to the temporary directory the caller should treat as the downloaded skill.
func (d *Downloader) Download(ctx context.Context, source *SourceInfo) (string, error) {
	defer perf.Track(nil, "marketplace.Downloader.Download")()

	// Create temporary directory.
	tempDir, err := os.MkdirTemp("", "atmos-skill-*")
	if err != nil {
		return "", fmt.Errorf("%w: failed to create temp directory: %w", ErrDownloadFailed, err)
	}

	if source.Type == "local" {
		// No network or Git clone involved -- just copy the directory tree,
		// reusing the same copyDir helper the multi-skill and client-distribution
		// paths already use.
		if err := copyDir(source.URL, tempDir); err != nil {
			os.RemoveAll(tempDir)
			return "", fmt.Errorf("%w: failed to copy local skill source %s: %w", ErrDownloadFailed, source.URL, err)
		}
		return tempDir, nil
	}

	raw := source.URL
	u, err := url.Parse(raw)
	if err != nil {
		os.RemoveAll(tempDir)
		return "", err
	}
	q := u.Query()
	if source.Ref != "" {
		q.Set("ref", source.Ref)
	}
	u.RawQuery = q.Encode()
	dl := downloader.NewGoGetterDownloader(&schema.AtmosConfiguration{})
	_, err = dl.(downloader.ContextFileDownloader).FetchWithMetadataContext(ctx, "git::"+u.String(), tempDir, downloader.ClientModeDir, 5*time.Minute)
	if err != nil {
		os.RemoveAll(tempDir)
		return "", fmt.Errorf("%w: %w", ErrDownloadFailed, err)
	}

	return tempDir, nil
}
