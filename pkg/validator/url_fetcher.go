package validator

import (
	"fmt"
	"io"
	"net/http"
)

// URLFetcher fetches content from a URL.
type URLFetcher struct {
	URL string
}

func (u *URLFetcher) Fetch() ([]byte, error) {
	resp, err := http.Get(u.URL)
	if err != nil {
		return nil, fmt.Errorf("failed to download URL: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to fetch URL, status: %d", resp.StatusCode)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("error reading response body: %w", err)
	}
	return data, nil
}
