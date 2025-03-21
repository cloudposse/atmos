package datafetcher

import (
	"fmt"
	"os"
)

// fileFetcher fetches data from a file.
type fileFetcher struct{}

func (f fileFetcher) FetchData(source string) ([]byte, error) {
	data, err := os.ReadFile(source)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}
	return data, nil
}
