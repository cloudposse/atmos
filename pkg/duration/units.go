package duration

import "time"

// Units expresses the same elapsed duration as whole totals in each unit.
// Months are 30 days and years are 365 days, matching Parse and FormatApproximate.
// These are independent totals, not the components of a calendar duration.
type Units struct {
	Seconds int64 `json:"seconds"`
	Minutes int64 `json:"minutes"`
	Hours   int64 `json:"hours"`
	Days    int64 `json:"days"`
	Weeks   int64 `json:"weeks"`
	Months  int64 `json:"months"`
	Years   int64 `json:"years"`
}

// InUnits returns whole elapsed totals, rounded down. Negative durations yield zeroes.
func InUnits(elapsed time.Duration) Units {
	seconds := max(int64(elapsed/time.Second), 0)
	return Units{
		Seconds: seconds,
		Minutes: seconds / secondsPerMinute,
		Hours:   seconds / secondsPerHour,
		Days:    seconds / secondsPerDay,
		Weeks:   seconds / secondsPerWeek,
		Months:  seconds / secondsPerMonth,
		Years:   seconds / secondsPerYear,
	}
}
