package lockfile

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
)

// sourceQueryDigest retains fetch identity lost by redacting a URL's query.
// Compare the original encoding conservatively: even an unrecognized query
// option can change fetched bytes. Never persist its raw value in the receipt.
func sourceQueryDigest(source string) string {
	_, query, found := strings.Cut(source, "?")
	if !found {
		return ""
	}
	query, _, _ = strings.Cut(query, "#")
	if query == "" {
		return ""
	}
	digest := sha256.Sum256([]byte(query))
	return "sha256:" + hex.EncodeToString(digest[:])
}
