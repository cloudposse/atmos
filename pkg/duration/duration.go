// Package duration provides utilities for parsing human-readable duration strings.
package duration

import (
	"strconv"
	"strings"
	"time"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
)

// Duration constants in seconds for named periods.
const (
	secondsPerMinute = 60
	secondsPerHour   = 3600
	secondsPerDay    = 86400
	secondsPerWeek   = 604800
	secondsPerMonth  = 2592000  // 30 days.
	secondsPerYear   = 31536000 // 365 days.

	// Integer parsing constants.
	base10    = 10 // Decimal base for strconv.ParseInt.
	bitSize64 = 64 // Bit size for int64 parsing.
)

// Parse parses a duration string into seconds.
//
// Supported formats:
//   - Integer seconds: "3600" → 3600
//   - Duration with suffix: "1h", "30m", "7d", "10s" → seconds
//   - Keywords: "minute", "hourly", "daily", "weekly", "monthly", "yearly"
//
// Examples:
//
//	Parse("3600")    → 3600, nil
//	Parse("1h")      → 3600, nil
//	Parse("7d")      → 604800, nil
//	Parse("daily")   → 86400, nil
//	Parse("invalid") → 0, error
func Parse(s string) (int64, error) {
	defer perf.Track(nil, "duration.Parse")()

	freq := strings.TrimSpace(s)

	// Try parsing as integer seconds.
	if intVal, err := strconv.ParseInt(freq, base10, bitSize64); err == nil {
		if intVal > 0 {
			return intVal, nil
		}
	}

	// Parse duration with suffix (e.g., "1h", "5m", "30s", "7d").
	if len(freq) > 1 {
		unit := freq[len(freq)-1]
		valPart := freq[:len(freq)-1]
		if valInt, err := strconv.ParseInt(valPart, base10, bitSize64); err == nil && valInt > 0 {
			switch unit {
			case 's':
				return valInt, nil
			case 'm':
				return valInt * secondsPerMinute, nil
			case 'h':
				return valInt * secondsPerHour, nil
			case 'd':
				return valInt * secondsPerDay, nil
			default:
				return 0, errUtils.Build(errUtils.ErrInvalidDuration).
					WithExplanation("Unrecognized duration unit").
					WithContext("unit", string(unit)).
					WithHint("Use 's' (seconds), 'm' (minutes), 'h' (hours), or 'd' (days)").
					Err()
			}
		}
	}

	// Handle predefined keywords.
	switch freq {
	case "minute":
		return secondsPerMinute, nil
	case "hourly":
		return secondsPerHour, nil
	case "daily":
		return secondsPerDay, nil
	case "weekly":
		return secondsPerWeek, nil
	case "monthly":
		return secondsPerMonth, nil
	case "yearly":
		return secondsPerYear, nil
	default:
		return 0, errUtils.Build(errUtils.ErrInvalidDuration).
			WithExplanation("Unrecognized duration format").
			WithContext("value", freq).
			WithHint("Use formats like '1h', '30m', '7d', or keywords like 'daily', 'weekly'").
			Err()
	}
}

// ParseDuration parses a duration string and returns a time.Duration.
//
// This is a convenience wrapper around Parse that converts the result
// from seconds to time.Duration.
//
// Examples:
//
//	ParseDuration("1h")    → 1 * time.Hour, nil
//	ParseDuration("7d")    → 7 * 24 * time.Hour, nil
//	ParseDuration("daily") → 24 * time.Hour, nil
func ParseDuration(s string) (time.Duration, error) {
	defer perf.Track(nil, "duration.ParseDuration")()

	seconds, err := Parse(s)
	if err != nil {
		return 0, err
	}
	return time.Duration(seconds) * time.Second, nil
}
