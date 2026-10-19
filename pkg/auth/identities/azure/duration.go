package azure

import (
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
)

// Sentinel errors for ISO-8601 duration parsing and conversion.
var (
	ErrEmptyDuration         = errors.New("empty ISO-8601 duration")
	ErrInvalidDurationPrefix = errors.New("invalid ISO-8601 duration: missing 'P' prefix")
	ErrDurationOverflow      = errors.New("ISO-8601 duration overflows time.Duration")
	ErrInvalidDuration       = errors.New("invalid ISO-8601 duration")
	ErrDurationNegative      = errors.New("invalid ISO-8601 duration: value must be non-negative")
)

const (
	bitSize64        = 64
	hoursPerDay      = 24
	daysPerWeek      = 7
	daysPerMonth     = 30
	daysPerYear      = 365
	secondsPerHour   = 3600
	secondsPerMinute = 60
)

// goDurationToISO8601 converts a Go time.Duration to the ISO-8601 duration ARM expects
// for scheduleInfo.expiration (for example 8h -> "PT8H", 90m -> "PT1H30M").
func goDurationToISO8601(d time.Duration) string {
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

// addUnit converts val of the given unit into nanoseconds and accumulates it into total,
// guarding against values that exceed or overflow the time.Duration range.
func addUnit(total time.Duration, val float64, unit time.Duration) (time.Duration, error) {
	if val < 0 || math.IsNaN(val) || math.IsInf(val, 0) {
		return 0, ErrDurationNegative
	}
	ns := val * float64(unit)
	if ns < 0 || ns > float64(math.MaxInt64) || float64(total)+ns > float64(math.MaxInt64) {
		return 0, ErrDurationOverflow
	}
	added := time.Duration(ns)
	if added < 0 || math.MaxInt64-added < total {
		return 0, ErrDurationOverflow
	}
	return total + added, nil
}

func isDurationDigit(r rune) bool {
	return (r >= '0' && r <= '9') || r == '.'
}

func dateUnit(r rune) (time.Duration, bool) {
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

func timeUnit(r rune) (time.Duration, bool) {
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

func parseUnitValue(numStr string, r rune, partName string) (float64, error) {
	if numStr == "" {
		return 0, fmt.Errorf("%w: bad unit designator %q in %s part", ErrInvalidDuration, r, partName)
	}
	val, err := strconv.ParseFloat(numStr, bitSize64)
	if err != nil {
		return 0, fmt.Errorf("%w: invalid number %q for unit %q in %s: %w", ErrInvalidDuration, numStr, r, partName, err)
	}
	return val, nil
}

// parsePart parses a segment (date or time) of an ISO-8601 duration.
func parsePart(part string, total time.Duration, partName string, getUnit func(rune) (time.Duration, bool)) (time.Duration, bool, error) {
	if part == "" {
		return total, false, nil
	}
	var cur strings.Builder
	found := false
	for _, r := range part {
		if isDurationDigit(r) {
			cur.WriteRune(r)
			continue
		}
		unit, ok := getUnit(r)
		if !ok {
			return 0, false, fmt.Errorf("%w: character %q in %s part", ErrInvalidDuration, r, partName)
		}
		val, err := parseUnitValue(cur.String(), r, partName)
		if err != nil {
			return 0, false, err
		}
		total, err = addUnit(total, val, unit)
		if err != nil {
			return 0, false, err
		}
		cur.Reset()
		found = true
	}
	if cur.Len() > 0 {
		return 0, false, fmt.Errorf("%w: unparsed number %q in %s part", ErrInvalidDuration, cur.String(), partName)
	}
	return total, found, nil
}

// parseDatePart parses the date component of an ISO-8601 duration string (Y, M, W, D).
func parseDatePart(part string, total time.Duration) (time.Duration, bool, error) {
	return parsePart(part, total, "date", dateUnit)
}

// parseTimePart parses the time component of an ISO-8601 duration string (H, M, S).
func parseTimePart(part string, total time.Duration) (time.Duration, bool, error) {
	return parsePart(part, total, "time", timeUnit)
}

// splitDurationParts separates an ISO-8601 string (without 'P') into date and time parts.
func splitDurationParts(s string) (string, string, error) {
	tIdx := strings.IndexAny(s, "Tt")
	if tIdx < 0 {
		return s, "", nil
	}
	datePart, timePart := s[:tIdx], s[tIdx+1:]
	if timePart == "" && datePart == "" {
		return "", "", fmt.Errorf("%w: no designators found", ErrInvalidDuration)
	}
	return datePart, timePart, nil
}

// parseISO8601Duration parses an ISO-8601 duration string into a time.Duration.
// It supports day (D), hour (H), minute (M), and second (S) designators, as well as
// week (W), month (M), and year (Y).
func parseISO8601Duration(s string) (time.Duration, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, ErrEmptyDuration
	}
	if !strings.HasPrefix(strings.ToUpper(s), "P") {
		return 0, fmt.Errorf("%w: missing 'P' prefix in %q", ErrInvalidDurationPrefix, s)
	}
	s = s[1:]
	if s == "" {
		return 0, fmt.Errorf("%w: empty after 'P'", ErrInvalidDuration)
	}

	datePart, timePart, err := splitDurationParts(s)
	if err != nil {
		return 0, err
	}

	total, dateFound, err := parseDatePart(datePart, 0)
	if err != nil {
		return 0, err
	}

	total, timeFound, err := parseTimePart(timePart, total)
	if err != nil {
		return 0, err
	}

	if !dateFound && !timeFound {
		return 0, fmt.Errorf("%w: no valid designators found", ErrInvalidDuration)
	}

	return total, nil
}
