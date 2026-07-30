package tags

import (
	"fmt"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
)

// ParseTagsFlag parses a comma-separated tags string into a trimmed, non-empty slice.
func ParseTagsFlag(input string) []string {
	defer perf.Track(nil, "tags.ParseTagsFlag")()

	if input == "" {
		return nil
	}

	parts := strings.Split(input, ",")
	result := make([]string, 0, len(parts))
	for _, p := range parts {
		if trimmed := strings.TrimSpace(p); trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

// ParseLabelsFlag parses a comma-separated key=value list into a map[string]string.
func ParseLabelsFlag(input string) (map[string]string, error) {
	defer perf.Track(nil, "tags.ParseLabelsFlag")()

	if input == "" {
		return nil, nil
	}

	result := make(map[string]string)
	for _, pair := range strings.Split(input, ",") {
		pair = strings.TrimSpace(pair)
		if pair == "" {
			continue
		}
		key, value, found := strings.Cut(pair, "=")
		key = strings.TrimSpace(key)
		if !found || key == "" {
			return nil, fmt.Errorf("%w: invalid label %q, expected key=value", errUtils.ErrInvalidFlag, pair)
		}
		result[key] = strings.TrimSpace(value)
	}
	return result, nil
}
