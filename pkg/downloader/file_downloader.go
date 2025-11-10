package downloader

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/filetype"
	"github.com/google/uuid"
)

const errDownloadFileFormat = "%w: '%s': %v"

// fileDownloader handles downloading files and directories from various sources
// without exposing the underlying implementation.
type fileDownloader struct {
	clientFactory     ClientFactory
	tempPathGenerator func() string
	fileReader        func(string) ([]byte, error)
	atomicWriter      func(string, []byte, os.FileMode) error
}

// NewFileDownloader initializes a FileDownloader with dependency injection.
func NewFileDownloader(factory ClientFactory) FileDownloader {
	return &fileDownloader{
		clientFactory:     factory,
		tempPathGenerator: func() string { return filepath.Join(os.TempDir(), uuid.New().String()) },
		fileReader:        os.ReadFile,
		atomicWriter:      writeFileAtomicDefault,
	}
}

// Fetch fetches content from a given source and saves it to the destination.
func (fd *fileDownloader) Fetch(src, dest string, mode ClientMode, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	client, err := fd.clientFactory.NewClient(ctx, src, dest, mode)
	if err != nil {
		return fmt.Errorf("%w: %v", errUtils.ErrCreateDownloadClient, err)
	}

	return client.Get()
}

// FetchAutoParse downloads a remote file, detects its format, and parses it.
func (fd *fileDownloader) FetchAndAutoParse(src string) (any, error) {
	filePath := fd.tempPathGenerator()
	defer os.Remove(filePath)

	if err := fd.Fetch(src, filePath, ClientModeFile, 30*time.Second); err != nil {
		return nil, fmt.Errorf(errDownloadFileFormat, errUtils.ErrDownloadFile, src, err)
	}

	v, err := filetype.DetectFormatAndParseFile(fd.fileReader, filePath)
	if err != nil {
		return nil, fmt.Errorf("%w: '%s': %v", errUtils.ErrParseFile, src, err)
	}
	return v, nil
}

// FetchAndParseByExtension downloads a remote file and parses it based on its extension.
func (fd *fileDownloader) FetchAndParseByExtension(src string) (any, error) {
	filePath := fd.tempPathGenerator()
	defer os.Remove(filePath)

	if err := fd.Fetch(src, filePath, ClientModeFile, 30*time.Second); err != nil {
		return nil, fmt.Errorf(errDownloadFileFormat, errUtils.ErrDownloadFile, src, err)
	}

	// Create a custom reader that reads the downloaded file but uses the original URL for extension detection
	readFunc := func(filename string) ([]byte, error) {
		// Read the actual downloaded file, not the URL
		return fd.fileReader(filePath)
	}

	// Pass the original source URL for extension detection
	v, err := filetype.ParseFileByExtension(readFunc, src)
	if err != nil {
		return nil, fmt.Errorf("%w: '%s': %v", errUtils.ErrParseFile, src, err)
	}
	return v, nil
}

// FetchAndParseRaw downloads a remote file and always returns it as a raw string.
func (fd *fileDownloader) FetchAndParseRaw(src string) (any, error) {
	filePath := fd.tempPathGenerator()
	defer os.Remove(filePath)

	if err := fd.Fetch(src, filePath, ClientModeFile, 30*time.Second); err != nil {
		return nil, fmt.Errorf(errDownloadFileFormat, errUtils.ErrDownloadFile, src, err)
	}

	v, err := filetype.ParseFileRaw(fd.fileReader, filePath)
	if err != nil {
		return nil, fmt.Errorf("%w: '%s': %v", errUtils.ErrParseFile, src, err)
	}
	return v, nil
}

// FetchData fetches content from a given source and returns it as a byte slice.
func (fd *fileDownloader) FetchData(src string) ([]byte, error) {
	filePath := fd.tempPathGenerator()
	defer os.Remove(filePath)

	if err := fd.Fetch(src, filePath, ClientModeFile, 30*time.Second); err != nil {
		return nil, fmt.Errorf(errDownloadFileFormat, errUtils.ErrDownloadFile, src, err)
	}

	return fd.fileReader(filePath)
}

// FetchAtomic downloads a file atomically to the destination.
// Uses temp file + fsync + atomic rename to prevent partial downloads or corruption.
func (fd *fileDownloader) FetchAtomic(src, dest string, mode ClientMode, timeout time.Duration) error {
	// Download to a temporary location first.
	tempPath := fd.tempPathGenerator()
	defer os.Remove(tempPath)

	// Fetch to temp location.
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	client, err := fd.clientFactory.NewClient(ctx, src, tempPath, mode)
	if err != nil {
		return fmt.Errorf("%w: %v", errUtils.ErrCreateDownloadClient, err)
	}

	if err := client.Get(); err != nil {
		return err
	}

	// Read the downloaded content.
	data, err := fd.fileReader(tempPath)
	if err != nil {
		return fmt.Errorf("%w: failed to read downloaded file: %v", errUtils.ErrDownloadFile, err)
	}

	// Write atomically to final destination using injected writer.
	if err := fd.atomicWriter(dest, data, 0o644); err != nil {
		return fmt.Errorf("%w: failed to write file atomically: %v", errUtils.ErrDownloadFile, err)
	}

	return nil
}
