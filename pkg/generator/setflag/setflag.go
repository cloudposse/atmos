// Package setflag parses the strict `--set key=value` entries shared by the scaffold commands.
//
// Unlike the lenient flags.ParseStringMap helper, which silently skips a malformed entry, every
// function here reports it, so a scaffold answer is never dropped without the user knowing.
package setflag

import (
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
)

const formatHint = "Use the format: --set key=value"

// ParseEntry parses one `--set` entry in the format "key=value". Both key and value have leading and
// trailing whitespace trimmed, matching flags.ParseStringMap. It returns an error when the entry has no
// '=' or an empty key.
func ParseEntry(entry string) (key, value string, err error) {
	defer perf.Track(nil, "setflag.ParseEntry")()

	key, value, found := strings.Cut(entry, "=")
	if !found {
		return "", "", errUtils.Build(errUtils.ErrInvalidFlag).
			WithExplanationf("Malformed --set flag (missing '='): %s", entry).
			WithHint(formatHint).
			Err()
	}
	key = strings.TrimSpace(key)
	if key == "" {
		return "", "", errUtils.Build(errUtils.ErrInvalidFlag).
			WithExplanationf("Malformed --set flag (empty key): %s", entry).
			WithHint(formatHint).
			Err()
	}
	return key, strings.TrimSpace(value), nil
}

// Validate reports the first malformed `--set` entry.
func Validate(entries []string) error {
	defer perf.Track(nil, "setflag.Validate")()

	for _, entry := range entries {
		if _, _, err := ParseEntry(entry); err != nil {
			return err
		}
	}
	return nil
}
