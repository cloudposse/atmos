package blob

import (
	"fmt"
	"maps"
	"unicode/utf8"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
)

// S3 object tag limits, enforced for every store so tags are portable.
const (
	MaxTags          = 10
	MaxTagKeyChars   = 128
	MaxTagValueChars = 256
)

// MergeTags returns defaults overlaid with overrides. It returns nil when both are empty and never
// aliases either input.
func MergeTags(defaults, overrides map[string]string) map[string]string {
	defer perf.Track(nil, "blob.MergeTags")()

	if len(defaults) == 0 && len(overrides) == 0 {
		return nil
	}
	merged := make(map[string]string, len(defaults)+len(overrides))
	maps.Copy(merged, defaults)
	maps.Copy(merged, overrides)
	return merged
}

// ValidateTags checks the S3 object tag limits: at most [MaxTags] tags, non-empty keys of at most
// [MaxTagKeyChars] characters, and values of at most [MaxTagValueChars] characters.
func ValidateTags(tags map[string]string) error {
	defer perf.Track(nil, "blob.ValidateTags")()

	if len(tags) > MaxTags {
		return tagError(fmt.Sprintf("%d tags exceed the limit of %d", len(tags), MaxTags))
	}
	for key, value := range tags {
		if key == "" || utf8.RuneCountInString(key) > MaxTagKeyChars {
			return tagError(fmt.Sprintf("tag key %q must be 1-%d characters", key, MaxTagKeyChars))
		}
		if utf8.RuneCountInString(value) > MaxTagValueChars {
			return tagError(fmt.Sprintf("tag %q value exceeds %d characters", key, MaxTagValueChars))
		}
	}
	return nil
}

func tagError(reason string) error {
	return NewError(KindInvalid, OpValidate, "", fmt.Errorf("%w: %s", errUtils.ErrBlobInvalid, reason))
}

// CloneMap returns a copy of m, or nil when m is empty.
func CloneMap(m map[string]string) map[string]string {
	defer perf.Track(nil, "blob.CloneMap")()

	if len(m) == 0 {
		return nil
	}
	return maps.Clone(m)
}
