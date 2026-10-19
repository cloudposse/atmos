package utils

import (
	"errors"
	"fmt"
	"strings"

	"github.com/cloudposse/atmos/pkg/duration"
)

// ErrDurationEmpty is returned when the duration string is empty or blank.
var ErrDurationEmpty = errors.New("duration string is empty")

// ParseDurationFlexible parses a duration string in the shared Atmos duration format
// (see pkg/duration): bare integers are seconds, Go durations such as "1h30m", days such
// as "2d", and keywords such as "daily". Returns whole seconds, or an error for values
// that are not positive.
func ParseDurationFlexible(durationStr string) (int64, error) {
	if strings.TrimSpace(durationStr) == "" {
		return 0, ErrDurationEmpty
	}
	seconds, err := duration.Parse(durationStr)
	if err != nil {
		return 0, fmt.Errorf("invalid duration format: %s (expected: integer seconds, Go duration like '1h30m', or days like '2d'): %w", durationStr, err)
	}
	return seconds, nil
}
