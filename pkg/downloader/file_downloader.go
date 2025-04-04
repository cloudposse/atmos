package downloader

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/cloudposse/atmos/pkg/filetype"
	"github.com/google/uuid"
)

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

	client, err := fd.clientFactory.NewClient(ctx, src, dest, mode)
	if err != nil {
		return fmt.Errorf("failed to create download client: %w", err)
	}

	return client.Get()
}

// FetchAutoParse downloads a remote file, detects its format, and parses it.
func (fd *fileDownloader) FetchAndAutoParse(src string) (any, error) {
	filePath := fd.tempPathGenerator()

	if err := fd.Fetch(src, filePath, ClientModeFile, 30*time.Second); err != nil {
		return nil, fmt.Errorf("failed to download file '%s': %w", src, err)
	}

	return filetype.DetectFormatAndParseFile(fd.fileReader, filePath)
}
