package downloader

import (
	"fmt"
	"net/url"

	errUtils "github.com/cloudposse/atmos/errors"
)

const MaskedSecret = "xxx"

func maskBasicAuth(rawURL string) (string, error) {
	parsedURL, err := url.Parse(rawURL)
	if err != nil {
		return "", fmt.Errorf("%w: %v", errUtils.ErrParseURL, err)
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
