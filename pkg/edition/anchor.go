package edition

import (
	"fmt"
	"regexp"
	"time"

	"github.com/cloudposse/atmos/pkg/perf"
)

// Granularity indicates how much of the date the user specified in an edition pin.
type Granularity string

const (
	// GranularityYear means the pin was "YYYY".
	GranularityYear Granularity = "year"
	// GranularityMonth means the pin was "YYYY-MM".
	GranularityMonth Granularity = "month"
	// GranularityDay means the pin was "YYYY-MM-DD".
	GranularityDay Granularity = "day"
)

const (
	// DateLayout is the layout for fully-specified edition dates.
	dateLayout = "2006-01-02"
	// InvalidEditionFormat wraps ErrInvalidEdition with the offending input.
	invalidEditionFormat = "%w: %q"
	// LastDayOfDecember closes out a year-granularity anchor.
	lastDayOfDecember = 31
)

var anchorPattern = regexp.MustCompile(`^(\d{4})(?:-(\d{2})(?:-(\d{2}))?)?$`)

// Anchor is a resolved edition pin.
//
// Partial dates round to the END of the period they name — "2026" resolves to
// 2026-12-31 and "2026-07" to 2026-07-31 — so "the 2026 edition" includes every
// default change shipped during 2026, matching Rust's edition semantics.
type Anchor struct {
	// Date is the fully resolved anchor date (UTC, midnight).
	Date time.Time
	// Raw is the string the user wrote, e.g. "2026-07".
	Raw string
	// Granularity records how much of the date the user specified.
	Granularity Granularity
}

// ParseAnchor parses an edition string ("YYYY", "YYYY-MM", or "YYYY-MM-DD") into
// an Anchor, rounding partial dates to the end of the period they name.
func ParseAnchor(s string) (Anchor, error) {
	defer perf.Track(nil, "edition.ParseAnchor")()

	match := anchorPattern.FindStringSubmatch(s)
	if match == nil {
		return Anchor{}, fmt.Errorf(invalidEditionFormat, ErrInvalidEdition, s)
	}

	switch {
	case match[3] != "":
		date, err := time.ParseInLocation(dateLayout, s, time.UTC)
		if err != nil {
			return Anchor{}, fmt.Errorf(invalidEditionFormat, ErrInvalidEdition, s)
		}
		return Anchor{Date: date, Raw: s, Granularity: GranularityDay}, nil
	case match[2] != "":
		firstOfMonth, err := time.ParseInLocation("2006-01", s, time.UTC)
		if err != nil {
			return Anchor{}, fmt.Errorf(invalidEditionFormat, ErrInvalidEdition, s)
		}
		// Day 0 of the following month is the last day of this month.
		lastOfMonth := time.Date(firstOfMonth.Year(), firstOfMonth.Month()+1, 0, 0, 0, 0, 0, time.UTC)
		return Anchor{Date: lastOfMonth, Raw: s, Granularity: GranularityMonth}, nil
	default:
		firstOfYear, err := time.ParseInLocation("2006", s, time.UTC)
		if err != nil {
			return Anchor{}, fmt.Errorf(invalidEditionFormat, ErrInvalidEdition, s)
		}
		endOfYear := time.Date(firstOfYear.Year(), time.December, lastDayOfDecember, 0, 0, 0, 0, time.UTC)
		return Anchor{Date: endOfYear, Raw: s, Granularity: GranularityYear}, nil
	}
}
