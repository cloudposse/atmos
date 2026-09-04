package ui

import "time"

// Clock provides time-related operations for testability.
type Clock interface {
	Now() time.Time
	Since(t time.Time) time.Duration
}

// realClock implements Clock using the standard time package.
type realClock struct{}

// Now returns the current time.
func (realClock) Now() time.Time {
	return time.Now()
}

// Since returns the duration since the given time.
func (realClock) Since(t time.Time) time.Duration {
	return time.Since(t)
}

// defaultClock returns the default clock implementation.
func defaultClock() Clock {
	return realClock{}
}
