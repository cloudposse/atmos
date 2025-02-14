package validator

import (
	"fmt"
	"os"
)

// FileFetcher fetches data from a file.
type FileFetcher struct {
	FilePath string
}

func (f *FileFetcher) Fetch() ([]byte, error) {
	data, err := os.ReadFile(f.FilePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}
	return data, nil
}
