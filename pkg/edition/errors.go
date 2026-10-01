package edition

import "errors"

// ErrInvalidEdition is returned when an edition string is not a valid
// year, year-month, or year-month-day date.
var ErrInvalidEdition = errors.New("invalid edition: expected YYYY, YYYY-MM, or YYYY-MM-DD")
