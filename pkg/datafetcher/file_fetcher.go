package datafetcher

import (
	"fmt"
	"os"
)

// FileFetcher fetches data from a file.
type FileFetcher struct {
}

func (f FileFetcher) FetchData(source string) ([]byte, error) {
	data, err := os.ReadFile(source)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}
	return data, nil
}
