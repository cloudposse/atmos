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

// unitMultipliers maps duration unit suffixes to their second multipliers.
var unitMultipliers = map[byte]int64{
	's': 1,
	'm': secondsPerMinute,
	'h': secondsPerHour,
	'd': secondsPerDay,
}

// keywordDurations maps keyword strings to their duration in seconds.
var keywordDurations = map[string]int64{
	"minute":  secondsPerMinute,
	"hourly":  secondsPerHour,
	"daily":   secondsPerDay,
	"weekly":  secondsPerWeek,
	"monthly": secondsPerMonth,
	"yearly":  secondsPerYear,
}

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
	if seconds, ok := parseAsInteger(freq); ok {
		return seconds, nil
	}

	// Try parsing as duration with suffix.
	if seconds, err := parseWithSuffix(freq); err != nil {
		return 0, err
	} else if seconds > 0 {
		return seconds, nil
	}

	// Try parsing as keyword.
	if seconds, ok := keywordDurations[freq]; ok {
		return seconds, nil
	}

	return 0, errUtils.Build(errUtils.ErrInvalidDuration).
		WithExplanation("Unrecognized duration format").
		WithContext("value", freq).
		WithHint("Use formats like '1h', '30m', '7d', or keywords like 'daily', 'weekly'").
		Err()
}

// parseAsInteger attempts to parse the string as a positive integer (seconds).
func parseAsInteger(s string) (int64, bool) {
	if intVal, err := strconv.ParseInt(s, base10, bitSize64); err == nil && intVal > 0 {
		return intVal, true
	}
	return 0, false
}

// parseWithSuffix attempts to parse a duration with a unit suffix (e.g., "1h", "30m").
// Returns (0, nil) if the string doesn't match the suffix pattern.
// Returns (0, error) if the suffix is unrecognized.
// Returns (seconds, nil) on success.
func parseWithSuffix(s string) (int64, error) {
	if len(s) <= 1 {
		return 0, nil
	}

	unit := s[len(s)-1]
	valPart := s[:len(s)-1]

	valInt, err := strconv.ParseInt(valPart, base10, bitSize64)
	if err != nil {
		// Not a valid number prefix - doesn't match suffix pattern.
		return 0, nil //nolint:nilerr // Intentionally returning nil - this means "not a suffix pattern".
	}
	if valInt <= 0 {
		return 0, nil
	}

	multiplier, ok := unitMultipliers[unit]
	if !ok {
		return 0, errUtils.Build(errUtils.ErrInvalidDuration).
			WithExplanation("Unrecognized duration unit").
			WithContext("unit", string(unit)).
			WithHint("Use 's' (seconds), 'm' (minutes), 'h' (hours), or 'd' (days)").
			Err()
	}

	return valInt * multiplier, nil
}

// maxDurationSeconds is the maximum seconds value that won't overflow time.Duration.
// time.Duration is int64 nanoseconds, so max is ~292 years in seconds.
const maxDurationSeconds = int64(^uint64(0)>>1) / int64(time.Second)

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

	// Guard against time.Duration overflow.
	if seconds > maxDurationSeconds {
		return 0, errUtils.Build(errUtils.ErrInvalidDuration).
			WithExplanation("Duration value too large").
			WithContext("seconds", seconds).
			WithHint("Maximum supported duration is approximately 292 years").
			Err()
	}

	return time.Duration(seconds) * time.Second, nil
}
