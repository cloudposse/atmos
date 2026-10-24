package duration

import (
	"fmt"
	"strings"
	"time"
)

// Format renders a duration for human display (e.g. "800ms", "45.2s", "34m 47s", "1h 2m 3s").
// Durations under a minute keep sub-second precision; longer ones are rounded to whole seconds
// and split into hours, minutes and seconds, omitting zero units.
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
	minutes := int(d%time.Hour) / int(time.Minute)
	seconds := int(d%time.Minute) / int(time.Second)

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
