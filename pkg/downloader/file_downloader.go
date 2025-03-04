package downloader

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/cloudposse/atmos/pkg/utils"
	"github.com/google/uuid"
	"github.com/hashicorp/hcl"
	"gopkg.in/yaml.v2"
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

	return fd.detectFormatAndParse(filePath)
}

// FetchData fetches content from a given source and returns it as a byte slice.
func (fd *fileDownloader) FetchData(src string) ([]byte, error) {
	filePath := fd.tempPathGenerator()

	if err := fd.Fetch(src, filePath, ClientModeFile, 30*time.Second); err != nil {
		return nil, fmt.Errorf("failed to download file '%s': %w", src, err)
	}

	return fd.fileReader(filePath)
}

func (fd *fileDownloader) detectFormatAndParse(filename string) (any, error) {
	var v any

	var err error

	d, err := fd.fileReader(filename)
	if err != nil {
		return nil, err
	}

	data := string(d)
	switch {
	case utils.IsJSON(data):
		err = json.Unmarshal(d, &v)
		if err != nil {
			return nil, err
		}
	case utils.IsHCL(data):
		err = hcl.Unmarshal(d, &v)
		if err != nil {
			return nil, err
		}
	case utils.IsYAML(data):
		err = yaml.Unmarshal(d, &v)
		if err != nil {
			return nil, err
		}
	default:
		v = data
	}

	return v, nil
}
