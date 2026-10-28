package duration

import (
	"fmt"
	"strings"
	"time"
)

// Format renders a duration for human display (e.g. "800ms", "45.2s", "34m 47s", "1h 2m 3s").
// Durations under a minute keep sub-second precision; longer ones are rounded to whole seconds
// and split into hours, minutes and seconds, omitting zero units.
// It deliberately does not call perf.Track: pkg/perf imports this package, so doing so would create an import cycle.
func Format(d time.Duration) string {
	if d < time.Second {
		return fmt.Sprintf("%dms", d.Milliseconds())
	}
	// Check the rounded value so 59.96s renders as "1m", not "60.0s".
	if d.Round(100*time.Millisecond) < time.Minute {
		return fmt.Sprintf("%.1fs", d.Seconds())
	}

	d = d.Round(time.Second)
	hours := int(d / time.Hour)
	minutes := int((d % time.Hour) / time.Minute)
	seconds := int((d % time.Minute) / time.Second)

	var parts []string
	if hours > 0 {
		parts = append(parts, fmt.Sprintf("%dh", hours))
	}
	if minutes > 0 {
		parts = append(parts, fmt.Sprintf("%dm", minutes))
	}
	if seconds > 0 {
		parts = append(parts, fmt.Sprintf("%ds", seconds))
	}
	return strings.Join(parts, " ")
}

// FormatApproximate renders the largest whole unit, such as "4 months" or "1 day".
// Months and years follow Parse's fixed 30-day and 365-day conventions rather than
// calendar arithmetic. Nonpositive durations render as "0 seconds".
// Like Format, this pure formatter cannot import perf without creating a cycle.
func FormatApproximate(d time.Duration) string {
	units := []struct {
		seconds int64
		name    string
	}{
		{secondsPerYear, "year"},
		{secondsPerMonth, "month"},
		{secondsPerWeek, "week"},
		{secondsPerDay, "day"},
		{secondsPerHour, "hour"},
		{secondsPerMinute, "minute"},
		{1, "second"},
	}
	for _, unit := range units {
		count := int64(d / (time.Duration(unit.seconds) * time.Second))
		if count < 1 {
			continue
		}
		suffix := "s"
		if count == 1 {
			suffix = ""
		}
		return fmt.Sprintf("%d %s%s", count, unit.name, suffix)
	}
	return "0 seconds"
}
