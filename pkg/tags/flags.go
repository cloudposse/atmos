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

// labelsFlagSource is the default source name used in --labels parse errors.
const labelsFlagSource = "--labels"

// ParseLabelsFlag parses a slice of key=value (or key:value) pairs into a map[string]string.
// The input is shaped the way pflag's StringSlice flag type hands it over: comma-splitting within
// a single occurrence happens in pflag itself and repeated occurrences accumulate
// (e.g. --labels a=1,b=2 --labels c=3). Elements are still comma-split here so a value bound from
// an environment variable (one element holding the whole comma-separated list) parses the same
// way. Duplicate keys are last-wins. Errors name the "--labels" flag as their source.
func ParseLabelsFlag(input []string) (map[string]string, error) {
	defer perf.Track(nil, "tags.ParseLabelsFlag")()

	return ParseLabelsFlagFrom(strings.Join(input, ","), labelsFlagSource)
}

// ParseLabelsFlagFrom parses a comma-separated key=value (or key:value) list into a map[string]string.
// The source names where the value came from (for example "--labels (or ATMOS_LABELS)") and is
// included in the error message when a pair is malformed. Duplicate keys are last-wins.
func ParseLabelsFlagFrom(input, source string) (map[string]string, error) {
	defer perf.Track(nil, "tags.ParseLabelsFlagFrom")()

	if input == "" {
		return nil, nil
	}

	result := make(map[string]string)
	for _, pair := range strings.Split(input, ",") {
		pair = strings.TrimSpace(pair)
		if pair == "" {
			continue
		}
		key, value, ok := splitLabelPair(pair)
		if !ok {
			return nil, errUtils.Build(fmt.Errorf("%w: invalid label %q for %s, expected key=value or key:value", errUtils.ErrInvalidFlag, pair, source)).
				WithHint("Pass comma-separated key=value (or key:value) pairs, for example --labels=ci=auto,team=platform").
				Err()
		}
		result[key] = value
	}
	return result, nil
}

// splitLabelPair splits a single "key=value" or "key:value" pair on whichever
// separator (= or :) occurs first in the string, so a value that itself
// contains the other separator is preserved verbatim (e.g. "key:val=ue" ->
// {"key": "val=ue"}; "key=val:ue" -> {"key": "val:ue"}). It reports false when
// the pair has no separator or an empty key.
func splitLabelPair(pair string) (string, string, bool) {
	sepIdx := strings.IndexAny(pair, "=:")
	if sepIdx == -1 {
		return "", "", false
	}

	key := strings.TrimSpace(pair[:sepIdx])
	if key == "" {
		return "", "", false
	}
	return key, strings.TrimSpace(pair[sepIdx+1:]), true
}
