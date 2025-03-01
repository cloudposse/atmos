package utils

import (
	"fmt"
	"net/url"
)

func MaskBasicAuth(rawURL string) (string, error) {
	parsedURL, err := url.Parse(rawURL)
	if err != nil {
		return "", fmt.Errorf("failed to parse URL: %w", err)
	}

	if parsedURL.User != nil {
		// If both username and password are set, mask both, otherwise mask only the username
		if _, hasPassword := parsedURL.User.Password(); hasPassword {
			parsedURL.User = url.UserPassword("xxx", "xxx")
		} else {
			parsedURL.User = url.User("xxx")
		}
	}

	return parsedURL.String(), nil
}
