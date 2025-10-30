package utils

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// ParseDurationFlexible parses a duration string in multiple formats:
// 1. Integer (treated as seconds): "3600" = 1 hour
// 2. Go duration format (supports combinations): "1h30m", "90m", "5400s" = 1.5 hours
// 3. Simple suffix for days (not supported by Go): "2d" = 2 days
// Returns duration in seconds, or error if invalid.
func ParseDurationFlexible(durationStr string) (int64, error) {
	durationStr = strings.TrimSpace(durationStr)
	if durationStr == "" {
		return 0, fmt.Errorf("duration string is empty")
	}

	// Try parsing as integer (seconds).
	if intVal, err := strconv.ParseInt(durationStr, 10, 64); err == nil {
		if intVal > 0 {
			return intVal, nil
		}
		return 0, fmt.Errorf("duration must be positive, got: %d", intVal)
	}

	// Try parsing with Go's time.ParseDuration (supports "1h", "1h30m", "90s", etc.).
	if dur, err := time.ParseDuration(durationStr); err == nil {
		seconds := int64(dur.Seconds())
		if seconds > 0 {
			return seconds, nil
		}
		return 0, fmt.Errorf("duration must be positive, got: %s", durationStr)
	}

	// Try simple suffix parsing for days (which time.ParseDuration doesn't support).
	if len(durationStr) > 1 {
		unit := durationStr[len(durationStr)-1]
		valPart := durationStr[:len(durationStr)-1]
		if valInt, err := strconv.ParseInt(valPart, 10, 64); err == nil && valInt > 0 {
			switch unit {
			case 'd':
				return valInt * 86400, nil // days to seconds
			}
		}
	}

	return 0, fmt.Errorf("invalid duration format: %s (expected: integer seconds, Go duration like '1h30m', or days like '2d')", durationStr)
}
