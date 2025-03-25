package utils

import (
	"fmt"
	"net/url"
)

// MaskBasicAuth replaces the username and password in a URL with "xxx" if present.
func MaskBasicAuth(rawURL string) (string, error) {
	parsedURL, err := url.Parse(rawURL)
	if err != nil {
		return "", fmt.Errorf("failed to parse URL: %w", err)
	}

	if parsedURL.User != nil {
		parsedURL.User = url.UserPassword("xxx", "xxx")
	}

	return parsedURL.String(), nil
}
