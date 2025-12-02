package resolution

import "errors"

// Static errors for cycle detection.
var (
	// ErrCycleDetected is returned when a circular dependency is detected.
	ErrCycleDetected = errors.New("cycle detected")
)
