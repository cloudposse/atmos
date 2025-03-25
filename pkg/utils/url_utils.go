package utils

import (
	"fmt"
	"net/url"
)

const MaskedSecret = "xxx"

func MaskBasicAuth(rawURL string) (string, error) {
	parsedURL, err := url.Parse(rawURL)
	if err != nil {
		return "", fmt.Errorf("failed to parse URL: %w", err)
	}

	if parsedURL.User != nil {
		// If both username and password are set, mask both, otherwise mask only the username
		if _, hasPassword := parsedURL.User.Password(); hasPassword {
			parsedURL.User = url.UserPassword(MaskedSecret, MaskedSecret)
		} else {
			parsedURL.User = url.User(MaskedSecret)
		}
	}

	return parsedURL.String(), nil
}
