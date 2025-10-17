package downloader

import (
	"fmt"
	"net/url"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
)

// MaskedSecret is used internally for credential masking.
// We use "REDACTED" instead of "***" because url.UserPassword() would URL-encode
// "***" as "%2A%2A%2A", making URLs harder to read. We post-process the output
// to replace "REDACTED" with "***" for traditional credential masking appearance.
const MaskedSecret = "REDACTED"

func maskBasicAuth(rawURL string) (string, error) {
	parsedURL, err := url.Parse(rawURL)
	if err != nil {
		return "", fmt.Errorf("%w: %v", errUtils.ErrParseURL, err)
	}

	if parsedURL.User != nil {
		// If both username and password are set, mask both, otherwise mask only the username.
		if _, hasPassword := parsedURL.User.Password(); hasPassword {
			parsedURL.User = url.UserPassword(MaskedSecret, MaskedSecret)
		} else {
			parsedURL.User = url.User(MaskedSecret)
		}
	}

	result := parsedURL.String()

	// Post-process: Replace REDACTED with *** for cleaner output.
	// This avoids URL encoding issues while providing traditional credential masking.
	result = strings.ReplaceAll(result, "REDACTED:REDACTED", "***")
	result = strings.ReplaceAll(result, "REDACTED", "***")

	return result, nil
}
