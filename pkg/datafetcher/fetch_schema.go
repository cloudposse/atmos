package datafetcher

import (
	"errors"
	"os"
	"strings"

	"github.com/cloudposse/atmos/pkg/downloader"
	"github.com/cloudposse/atmos/pkg/schema"
)

// Fetcher is an interface for fetching data from various sources.
type Fetcher interface {
	FetchData(source string) ([]byte, error)
}

var ErrUnsupportedSource = errors.New("unsupported source type")

//go:generate go run go.uber.org/mock/mockgen@v0.6.0 -source=$GOFILE -destination=mock_$GOFILE -package=$GOPACKAGE
type DataFetcher interface {
	GetData(source string) ([]byte, error)
}

type dataFetcher struct {
	fileDownloader downloader.FileDownloader
	atmosConfig    *schema.AtmosConfiguration
}

// NewDataFetcher creates a new dataFetcher instance.
func NewDataFetcher(atmosConfig *schema.AtmosConfiguration) *dataFetcher {
	return &dataFetcher{
		fileDownloader: downloader.NewGoGetterDownloader(atmosConfig),
		atmosConfig:    atmosConfig,
	}
}

// GetData returns the data based on source.
func (d *dataFetcher) GetData(source string) ([]byte, error) {
	fetcher, err := d.getDataFetcher(source)
	if err != nil {
		return nil, err
	}
	return fetcher.FetchData(source)
}

// getDataFetcher returns the appropriate Fetcher based on the input source.
func (d *dataFetcher) getDataFetcher(source string) (Fetcher, error) {
	switch {
	case strings.HasPrefix(source, "http://") || strings.HasPrefix(source, "https://"):
		return downloader.NewGoGetterDownloader(d.atmosConfig), nil
	case strings.HasPrefix(source, "atmos://"):
		return atmosFetcher{}, nil
	case strings.Contains(source, "{") && strings.Contains(source, "}"):
		return inlineJsonFetcher{}, nil
	default:
		if _, err := os.Stat(source); err == nil {
			return fileFetcher{}, nil
		}
		return nil, ErrUnsupportedSource
	}
}
