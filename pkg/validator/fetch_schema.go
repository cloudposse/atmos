package validator

import (
	"errors"
	"os"
	"strings"
)

// Fetcher is an interface for fetching data from various sources.
type Fetcher interface {
	Fetch() ([]byte, error)
}

// getDataFetcher returns the appropriate Fetcher based on the input source.
func getDataFetcher(source string) (Fetcher, error) {
	switch {
	case strings.HasPrefix(source, "http://") || strings.HasPrefix(source, "https://"):
		return &URLFetcher{URL: source}, nil
	case strings.HasPrefix(source, "atmos://"):
		return &AtmosFetcher{Key: source}, nil
	default:
		if _, err := os.Stat(source); err == nil {
			return &FileFetcher{FilePath: source}, nil
		}
		return nil, errors.New("unsupported source type")
	}
}

// GetData returns the data based on source
func GetData(source string) ([]byte, error) {
	fetcher, err := getDataFetcher(source)
	if err != nil {
		return nil, err
	}
	return fetcher.Fetch()
}
