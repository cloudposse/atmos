// Package duration is the single parser for every duration Atmos reads from configuration,
// flags, and environment variables.
//
// Accepted forms, in every place a duration is configured:
//
//   - Go durations: "500ms", "30s", "1h30m", "1.5s"
//   - Bare numbers are seconds: "10", 10, 1.5 (YAML integers and floats included)
//   - Days: "2d", "1d12h"
//   - Keywords: "minute", "hourly", "daily", "weekly", "monthly", "yearly"
//   - ISO-8601: "PT8H", "P1DT12H" (see iso8601.go)
//
// The package depends only on the standard library and mapstructure so that pkg/schema and
// the errors package can import it without cycles. Callers add their own hints and context.
package duration

import (
	"errors"
	"fmt"
	"math"
	"reflect"
	"strconv"
	"strings"
	"time"
)

// Sentinel errors. The errors package re-exports ErrInvalid as ErrInvalidDuration so
// errors.Is works with either name.
var (
	// ErrInvalid is returned for a value that is not a duration in any accepted form.
	ErrInvalid = errors.New("invalid duration format")
	// ErrNotPositive is returned when a positive duration is required and the value is zero or negative.
	ErrNotPositive = errors.New("duration must be positive")
	// ErrNegative is returned when a non-negative duration is required and the value is negative.
	ErrNegative = errors.New("duration must not be negative")
	// ErrOverflow is returned when a value does not fit in time.Duration.
	ErrOverflow = errors.New("duration overflows time.Duration")
)

// Duration constants in seconds for named periods.
const (
	secondsPerMinute = 60
	secondsPerHour   = 3600
	secondsPerDay    = 86400
	secondsPerWeek   = 604800
	secondsPerMonth  = 2592000  // 30 days.
	secondsPerYear   = 31536000 // 365 days.

	bitSize64 = 64

	// Format string for a sentinel error followed by the offending quoted value.
	errFormat = "%w: %q"
)

// keywordDurations maps keyword strings to their duration in seconds.
var keywordDurations = map[string]int64{
	"minute":  secondsPerMinute,
	"hourly":  secondsPerHour,
	"daily":   secondsPerDay,
	"weekly":  secondsPerWeek,
	"monthly": secondsPerMonth,
	"yearly":  secondsPerYear,
}

// Hint is the one-line format reminder callers attach to their errors.
const Hint = "Use a duration such as `30s`, `5m`, `1h30m`, or `2d`; a bare number means seconds."

// ParseAny parses a duration from a configuration value of any type. Strings go through
// ParseString; integers and floats are seconds; a time.Duration passes through. The result may
// be zero or negative; use ParseDuration or ParseNonNegative to enforce a sign.
func ParseAny(value any) (time.Duration, error) {
	switch v := value.(type) {
	case nil:
		return 0, fmt.Errorf("%w: empty value", ErrInvalid)
	case time.Duration:
		return v, nil
	case string:
		return ParseString(v)
	}
	rv := reflect.ValueOf(value)
	switch rv.Kind() { //nolint:exhaustive // Other kinds are not durations.
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return secondsToDuration(float64(rv.Int()))
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return secondsToDuration(float64(rv.Uint()))
	case reflect.Float32, reflect.Float64:
		return secondsToDuration(rv.Float())
	}
	return 0, fmt.Errorf("%w: unsupported type %T", ErrInvalid, value)
}

// ParseString parses a duration string in any accepted form. The result may be zero or
// negative; use ParseDuration or ParseNonNegative to enforce a sign.
func ParseString(s string) (time.Duration, error) {
	raw := strings.TrimSpace(s)
	if raw == "" {
		return 0, fmt.Errorf("%w: empty value", ErrInvalid)
	}
	if seconds, ok := keywordDurations[strings.ToLower(raw)]; ok {
		return time.Duration(seconds) * time.Second, nil
	}
	if isISO8601(raw) {
		return ParseISO8601(raw)
	}
	// Bare numbers (integers or decimals) are seconds.
	if seconds, err := strconv.ParseFloat(raw, bitSize64); err == nil {
		return secondsToDuration(seconds)
	}
	if d, ok, err := parseWithDays(raw); ok || err != nil {
		return d, err
	}
	d, err := time.ParseDuration(raw)
	if err != nil {
		// Keep Go's message (`time: invalid duration "..."`) as the cause: it names the value and
		// is what users and existing tests recognize.
		return 0, fmt.Errorf("%w: %w", ErrInvalid, err)
	}
	return d, nil
}

// ParseDuration parses a duration that must be greater than zero.
func ParseDuration(s string) (time.Duration, error) {
	d, err := ParseString(s)
	if err != nil {
		return 0, err
	}
	if d <= 0 {
		return 0, fmt.Errorf(errFormat, ErrNotPositive, strings.TrimSpace(s))
	}
	return d, nil
}

// ParseNonNegative parses a duration that may be zero but not negative.
func ParseNonNegative(s string) (time.Duration, error) {
	d, err := ParseString(s)
	if err != nil {
		return 0, err
	}
	if d < 0 {
		return 0, fmt.Errorf(errFormat, ErrNegative, strings.TrimSpace(s))
	}
	return d, nil
}

// Parse parses a duration and returns whole seconds. The value must be at least one second;
// this is the contract used for cache TTLs, cleanup periods, and the version-check frequency.
func Parse(s string) (int64, error) {
	d, err := ParseDuration(s)
	if err != nil {
		return 0, err
	}
	if d < time.Second {
		return 0, fmt.Errorf("%w: %q is shorter than one second", ErrNotPositive, strings.TrimSpace(s))
	}
	return int64(d / time.Second), nil
}

// secondsToDuration converts seconds to time.Duration, guarding against overflow.
func secondsToDuration(seconds float64) (time.Duration, error) {
	if math.IsNaN(seconds) || math.IsInf(seconds, 0) {
		return 0, fmt.Errorf("%w: %v", ErrInvalid, seconds)
	}
	ns := seconds * float64(time.Second)
	if ns > float64(math.MaxInt64) || ns < float64(math.MinInt64) {
		return 0, fmt.Errorf("%w: %v seconds", ErrOverflow, seconds)
	}
	return time.Duration(ns), nil
}

// parseWithDays parses "<n>d" optionally followed by a Go duration, such as "2d" or "1d12h".
// It reports ok=false when the value has no day component.
func parseWithDays(raw string) (time.Duration, bool, error) {
	idx := strings.IndexByte(raw, 'd')
	if idx <= 0 {
		return 0, false, nil
	}
	days, err := strconv.ParseFloat(strings.TrimPrefix(raw[:idx], "+"), bitSize64)
	if err != nil {
		return 0, false, nil //nolint:nilerr // Not a day-prefixed value; let the caller try other forms.
	}
	total, err := secondsToDuration(days * secondsPerDay)
	if err != nil {
		return 0, true, err
	}
	rest := raw[idx+1:]
	if rest == "" {
		return total, true, nil
	}
	tail, err := time.ParseDuration(rest)
	if err != nil {
		return 0, true, fmt.Errorf("%w: %w", ErrInvalid, err)
	}
	if (tail > 0 && total > math.MaxInt64-tail) || (tail < 0 && total < math.MinInt64-tail) {
		return 0, true, fmt.Errorf(errFormat, ErrOverflow, raw)
	}
	return total + tail, true, nil
}

// IsZeroTTL reports whether the TTL string represents a zero duration.
//
// Parse rejects zero (a zero duration is not a valid period for cleanup scheduling), but for
// cache TTLs a zero value is a legitimate, common case meaning "always expired / never reuse".
// Callers detect it with this helper before parsing.
func IsZeroTTL(ttl string) bool {
	d, err := ParseString(ttl)
	return err == nil && d == 0
}

// IsExpired reports whether a cache entry last refreshed at updatedAt has exceeded ttl.
//
// A zero TTL (see IsZeroTTL) is always expired. A non-zero TTL is parsed with ParseDuration; on
// a parse error the error is returned and callers should fail safe by treating the entry as
// expired to avoid serving stale data. This helper makes no decision for an empty ttl (""). The
// meaning of "no TTL configured" differs between subsystems (reuse-forever versus
// refresh-each-run), so callers handle "" explicitly.
func IsExpired(updatedAt time.Time, ttl string) (bool, error) {
	if IsZeroTTL(ttl) {
		return true, nil
	}
	ttlDuration, err := ParseDuration(ttl)
	if err != nil {
		return false, err
	}
	return time.Since(updatedAt) > ttlDuration, nil
}
