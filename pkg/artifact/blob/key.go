package blob

import (
	"fmt"
	"strings"
	"unicode/utf8"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
)

// MaxKeyBytes is the longest key any store accepts. It matches the S3 object key limit.
const MaxKeyBytes = 1024

// ValidateKey checks that key is a portable relative key: non-empty, valid UTF-8 without NUL or
// backslash, no leading or trailing slash, and no empty, "." or ".." segments. Filesystem stores
// require it; S3 stores apply it by default so a key is valid on every backend.
func ValidateKey(key string) error {
	defer perf.Track(nil, "blob.ValidateKey")()

	if err := ValidateOpaqueKey(key); err != nil {
		return err
	}
	if strings.ContainsRune(key, '\\') {
		return invalidKey(key, "contains a backslash")
	}
	if strings.HasPrefix(key, "/") || strings.HasSuffix(key, "/") {
		return invalidKey(key, "has a leading or trailing slash")
	}
	for _, segment := range strings.Split(key, "/") {
		switch segment {
		case "":
			return invalidKey(key, "contains an empty segment")
		case ".", "..":
			return invalidKey(key, "contains a relative segment")
		}
	}
	return nil
}

// ValidateOpaqueKey applies only the limits every object store shares: non-empty, at most
// [MaxKeyBytes] bytes, valid UTF-8, and no NUL. It is used for legacy key patterns that are
// opaque strings rather than portable paths.
func ValidateOpaqueKey(key string) error {
	defer perf.Track(nil, "blob.ValidateOpaqueKey")()

	switch {
	case key == "":
		return invalidKey(key, "is empty")
	case len(key) > MaxKeyBytes:
		return invalidKey(key, "is too long")
	case !utf8.ValidString(key):
		return invalidKey(key, "is not valid UTF-8")
	case strings.ContainsRune(key, 0):
		return invalidKey(key, "contains NUL")
	}
	return nil
}

func invalidKey(key, reason string) error {
	return NewError(KindInvalid, OpValidate, key, fmt.Errorf("%w: key %s", errUtils.ErrBlobInvalid, reason))
}
