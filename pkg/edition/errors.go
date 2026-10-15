package edition

import "errors"

// ErrInvalidEdition is returned when an edition string is not a valid
// year, year-month, or year-month-day date.
var ErrInvalidEdition = errors.New("invalid edition: expected YYYY, YYYY-MM, or YYYY-MM-DD")

// ErrUnknownBehavior is returned when a behavior key has no behavior journal entry.
var ErrUnknownBehavior = errors.New("unknown edition behavior")
