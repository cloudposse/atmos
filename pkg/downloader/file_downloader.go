package downloader

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/filetype"
	"github.com/cloudposse/atmos/pkg/github"
	log "github.com/cloudposse/atmos/pkg/logger"
)

const (
	// MinRateLimitRemaining is the minimum remaining rate limit before pre-check waits.
	MinRateLimitRemaining = 5
)

// isGitHubHTTPURL checks if the given URL is a GitHub HTTP URL that uses rate-limited APIs.
// This includes raw.githubusercontent.com for file downloads and github.com archive/release URLs.
func isGitHubHTTPURL(src string) bool {
	src = strings.ToLower(src)
	// Raw GitHub content (used for mixins, imports, templates).
	if strings.Contains(src, "raw.githubusercontent.com") {
		return true
	}
	// GitHub archive downloads (tarballs, zipballs).
	if strings.Contains(src, "github.com") &&
		(strings.Contains(src, "/archive/") || strings.Contains(src, "/releases/")) {
		return true
	}
	return false
}

// fileDownloader handles downloading files and directories from various sources
// without exposing the underlying implementation.
type fileDownloader struct {
	clientFactory     ClientFactory
	tempPathGenerator func() string
	fileReader        func(string) ([]byte, error)
}

// NewFileDownloader initializes a FileDownloader with dependency injection.
func NewFileDownloader(factory ClientFactory) FileDownloader {
	return &fileDownloader{
		clientFactory:     factory,
		tempPathGenerator: func() string { return filepath.Join(os.TempDir(), uuid.New().String()) },
		fileReader:        os.ReadFile,
	}
}

// Fetch fetches content from a given source and saves it to the destination.
func (fd *fileDownloader) Fetch(src, dest string, mode ClientMode, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	// Pre-check GitHub rate limits for GitHub HTTP URLs.
	if isGitHubHTTPURL(src) {
		if err := github.WaitForRateLimit(ctx, MinRateLimitRemaining); err != nil {
			log.Warn("Rate limit wait interrupted", "error", err)
			// Continue anyway - don't block on rate limit check failures.
		}
	}

	client, err := fd.clientFactory.NewClient(ctx, src, dest, mode)
	if err != nil {
		return errUtils.Build(errUtils.ErrCreateDownloadClient).
			WithCause(err).
			WithContext("url", src).
			WithHint("Check that the URL format is valid").
			Err()
	}

	return client.Get()
}

// FetchAutoParse downloads a remote file, detects its format, and parses it.
func (fd *fileDownloader) FetchAndAutoParse(src string) (any, error) {
	filePath := fd.tempPathGenerator()
	defer os.Remove(filePath)

	if err := fd.Fetch(src, filePath, ClientModeFile, 30*time.Second); err != nil {
		return nil, errUtils.Build(errUtils.ErrDownloadFile).
			WithCause(err).
			WithContext("url", src).
			WithHint("Check network connectivity and verify the URL is accessible").
			Err()
	}

	v, err := filetype.DetectFormatAndParseFile(fd.fileReader, filePath)
	if err != nil {
		return nil, errUtils.Build(errUtils.ErrParseFile).
			WithCause(err).
			WithContext("url", src).
			WithHint("Verify the file format matches the expected type").
			Err()
	}
	return v, nil
}

// FetchAndParseByExtension downloads a remote file and parses it based on its extension.
func (fd *fileDownloader) FetchAndParseByExtension(src string) (any, error) {
	filePath := fd.tempPathGenerator()
	defer os.Remove(filePath)

	if err := fd.Fetch(src, filePath, ClientModeFile, 30*time.Second); err != nil {
		return nil, errUtils.Build(errUtils.ErrDownloadFile).
			WithCause(err).
			WithContext("url", src).
			WithHint("Check network connectivity and verify the URL is accessible").
			Err()
	}

	// Create a custom reader that reads the downloaded file but uses the original URL for extension detection.
	readFunc := func(filename string) ([]byte, error) {
		// Read the actual downloaded file, not the URL.
		return fd.fileReader(filePath)
	}

	// Pass the original source URL for extension detection.
	v, err := filetype.ParseFileByExtension(readFunc, src)
	if err != nil {
		return nil, errUtils.Build(errUtils.ErrParseFile).
			WithCause(err).
			WithContext("url", src).
			WithHint("Verify the file format matches the expected type").
			Err()
	}
	return v, nil
}

// FetchAndParseRaw downloads a remote file and always returns it as a raw string.
func (fd *fileDownloader) FetchAndParseRaw(src string) (any, error) {
	filePath := fd.tempPathGenerator()
	defer os.Remove(filePath)

	if err := fd.Fetch(src, filePath, ClientModeFile, 30*time.Second); err != nil {
		return nil, errUtils.Build(errUtils.ErrDownloadFile).
			WithCause(err).
			WithContext("url", src).
			WithHint("Check network connectivity and verify the URL is accessible").
			Err()
	}

	v, err := filetype.ParseFileRaw(fd.fileReader, filePath)
	if err != nil {
		return nil, errUtils.Build(errUtils.ErrParseFile).
			WithCause(err).
			WithContext("url", src).
			WithHint("Verify the file format matches the expected type").
			Err()
	}
	return v, nil
}

// FetchData fetches content from a given source and returns it as a byte slice.
func (fd *fileDownloader) FetchData(src string) ([]byte, error) {
	filePath := fd.tempPathGenerator()
	defer os.Remove(filePath)

	if err := fd.Fetch(src, filePath, ClientModeFile, 30*time.Second); err != nil {
		return nil, errUtils.Build(errUtils.ErrDownloadFile).
			WithCause(err).
			WithContext("url", src).
			WithHint("Check network connectivity and verify the URL is accessible").
			Err()
	}

	return fd.fileReader(filePath)
}
