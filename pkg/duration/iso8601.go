package duration

import (
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
)

// Sentinel errors for ISO-8601 duration parsing. Each wraps ErrInvalid so callers can match
// either the specific or the general error.
var (
	// ErrISO8601Empty is returned for an empty ISO-8601 duration.
	ErrISO8601Empty = fmt.Errorf("%w: empty ISO-8601 duration", ErrInvalid)
	// ErrISO8601Prefix is returned when the 'P' designator is missing.
	ErrISO8601Prefix = fmt.Errorf("%w: missing 'P' prefix", ErrInvalid)
	// ErrISO8601Designator is returned for a malformed or unknown designator.
	ErrISO8601Designator = fmt.Errorf("%w: invalid ISO-8601 designator", ErrInvalid)
)

const (
	hoursPerDay  = 24
	daysPerWeek  = 7
	daysPerMonth = 30
	daysPerYear  = 365
)

// isISO8601 reports whether raw looks like an ISO-8601 duration ("P..." or "p...").
func isISO8601(raw string) bool {
	return len(raw) > 1 && (raw[0] == 'P' || raw[0] == 'p')
}

// FormatISO8601 converts a Go time.Duration to the ISO-8601 form used by cloud APIs such as
// Azure PIM scheduleInfo.expiration (for example 8h -> "PT8H", 90m -> "PT1H30M"). Zero and
// negative durations format as "".
func FormatISO8601(d time.Duration) string {
	totalSeconds := int64(d.Seconds())
	if totalSeconds <= 0 {
		return ""
	}
	hours := totalSeconds / secondsPerHour
	minutes := (totalSeconds % secondsPerHour) / secondsPerMinute
	seconds := totalSeconds % secondsPerMinute

	var sb strings.Builder
	sb.WriteString("PT")
	if hours > 0 {
		fmt.Fprintf(&sb, "%dH", hours)
	}
	if minutes > 0 {
		fmt.Fprintf(&sb, "%dM", minutes)
	}
	if seconds > 0 {
		fmt.Fprintf(&sb, "%dS", seconds)
	}
	return sb.String()
}

// ParseISO8601 parses an ISO-8601 duration string into a time.Duration. It supports the
// date designators Y, M, W, D (365, 30, 7, and 1 days) and the time designators H, M, S,
// including decimal values such as "PT0.5H". Negative values are rejected.
func ParseISO8601(s string) (time.Duration, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, ErrISO8601Empty
	}
	if !strings.HasPrefix(strings.ToUpper(s), "P") {
		return 0, fmt.Errorf("%w in %q", ErrISO8601Prefix, s)
	}
	body := s[1:]
	if body == "" {
		return 0, fmt.Errorf("%w: empty after 'P' in %q", ErrISO8601Designator, s)
	}

	datePart, timePart, err := splitISO8601Parts(body)
	if err != nil {
		return 0, err
	}
	total, dateFound, err := parseISO8601Part(datePart, 0, "date", iso8601DateUnit)
	if err != nil {
		return 0, err
	}
	total, timeFound, err := parseISO8601Part(timePart, total, "time", iso8601TimeUnit)
	if err != nil {
		return 0, err
	}
	if !dateFound && !timeFound {
		return 0, fmt.Errorf("%w: no designators found in %q", ErrISO8601Designator, s)
	}
	return total, nil
}

// splitISO8601Parts separates an ISO-8601 body (without 'P') into date and time parts.
func splitISO8601Parts(s string) (string, string, error) {
	tIdx := strings.IndexAny(s, "Tt")
	if tIdx < 0 {
		return s, "", nil
	}
	datePart, timePart := s[:tIdx], s[tIdx+1:]
	if timePart == "" && datePart == "" {
		return "", "", fmt.Errorf("%w: no designators found", ErrISO8601Designator)
	}
	return datePart, timePart, nil
}

func iso8601DateUnit(r rune) (time.Duration, bool) {
	switch r {
	case 'Y', 'y':
		return daysPerYear * hoursPerDay * time.Hour, true
	case 'M', 'm':
		return daysPerMonth * hoursPerDay * time.Hour, true
	case 'W', 'w':
		return daysPerWeek * hoursPerDay * time.Hour, true
	case 'D', 'd':
		return hoursPerDay * time.Hour, true
	default:
		return 0, false
	}
}

func iso8601TimeUnit(r rune) (time.Duration, bool) {
	switch r {
	case 'H', 'h':
		return time.Hour, true
	case 'M', 'm':
		return time.Minute, true
	case 'S', 's':
		return time.Second, true
	default:
		return 0, false
	}
}

// parseISO8601Part parses one segment (date or time) of an ISO-8601 duration.
func parseISO8601Part(part string, total time.Duration, partName string, unitOf func(rune) (time.Duration, bool)) (time.Duration, bool, error) {
	if part == "" {
		return total, false, nil
	}
	var number strings.Builder
	found := false
	for _, r := range part {
		if (r >= '0' && r <= '9') || r == '.' {
			number.WriteRune(r)
			continue
		}
		unit, ok := unitOf(r)
		if !ok {
			return 0, false, fmt.Errorf("%w: character %q in %s part", ErrISO8601Designator, r, partName)
		}
		val, err := parseISO8601Number(number.String(), r, partName)
		if err != nil {
			return 0, false, err
		}
		total, err = addISO8601Unit(total, val, unit)
		if err != nil {
			return 0, false, err
		}
		number.Reset()
		found = true
	}
	if number.Len() > 0 {
		return 0, false, fmt.Errorf("%w: unparsed number %q in %s part", ErrISO8601Designator, number.String(), partName)
	}
	return total, found, nil
}

// parseISO8601Number parses the number preceding designator r, rejecting a missing number.
func parseISO8601Number(numStr string, r rune, partName string) (float64, error) {
	if numStr == "" {
		return 0, fmt.Errorf("%w: bad unit designator %q in %s part", ErrISO8601Designator, r, partName)
	}
	val, err := strconv.ParseFloat(numStr, bitSize64)
	if err != nil {
		return 0, fmt.Errorf("%w: invalid number %q for unit %q in %s part", ErrISO8601Designator, numStr, r, partName)
	}
	return val, nil
}

// addISO8601Unit converts val of the given unit into nanoseconds and accumulates it into total,
// guarding against values that exceed or overflow the time.Duration range.
func addISO8601Unit(total time.Duration, val float64, unit time.Duration) (time.Duration, error) {
	if val < 0 || math.IsNaN(val) || math.IsInf(val, 0) {
		return 0, fmt.Errorf("%w: ISO-8601 value %v", ErrNegative, val)
	}
	ns := val * float64(unit)
	if ns < 0 || ns > float64(math.MaxInt64) || float64(total)+ns > float64(math.MaxInt64) {
		return 0, ErrOverflow
	}
	added := time.Duration(ns)
	if added < 0 || math.MaxInt64-added < total {
		return 0, ErrOverflow
	}
	return total + added, nil
}

// Is reports whether err is any duration parsing error from this package.
func Is(err error) bool {
	return errors.Is(err, ErrInvalid) || errors.Is(err, ErrNotPositive) || errors.Is(err, ErrNegative) || errors.Is(err, ErrOverflow)
}
