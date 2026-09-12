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

// ParseLabelsFlag parses a slice of key=value (or key:value) pairs into a map[string]string.
// The input is expected to already be split into individual pairs -- pflag's StringSlice flag
// type comma-splits within a single occurrence and accumulates across repeated occurrences
// (e.g. --labels a=1,b=2 --labels c=3), so no further splitting happens here.
func ParseLabelsFlag(input []string) (map[string]string, error) {
	defer perf.Track(nil, "tags.ParseLabelsFlag")()

	if len(input) == 0 {
		return nil, nil
	}

	result := make(map[string]string)
	for _, pair := range input {
		pair = strings.TrimSpace(pair)
		if pair == "" {
			continue
		}
		key, value, err := splitLabelPair(pair)
		if err != nil {
			return nil, err
		}
		result[key] = value
	}
	return result, nil
}

// splitLabelPair splits a single "key=value" or "key:value" pair on whichever
// separator (= or :) occurs first in the string, so a value that itself
// contains the other separator is preserved verbatim (e.g. "key:val=ue" ->
// {"key": "val=ue"}; "key=val:ue" -> {"key": "val:ue"}).
func splitLabelPair(pair string) (string, string, error) {
	sepIdx := strings.IndexAny(pair, "=:")
	if sepIdx == -1 {
		return "", "", fmt.Errorf("%w: invalid label %q, expected key=value or key:value", errUtils.ErrInvalidFlag, pair)
	}

	key := strings.TrimSpace(pair[:sepIdx])
	if key == "" {
		return "", "", fmt.Errorf("%w: invalid label %q, expected key=value or key:value", errUtils.ErrInvalidFlag, pair)
	}
	return key, strings.TrimSpace(pair[sepIdx+1:]), nil
}
