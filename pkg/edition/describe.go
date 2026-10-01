package edition

import "github.com/cloudposse/atmos/pkg/perf"

// Pin describes the effect of an edition pin for `atmos describe edition`.
type Pin struct {
	// Pinned reports whether an edition is set at all.
	Pinned bool `json:"pinned" yaml:"pinned"`
	// Edition is the raw pin as the user wrote it, e.g. "2026-07".
	Edition string `json:"edition,omitempty" yaml:"edition,omitempty"`
	// ResolvedDate is the fully resolved anchor date (partial dates round to the
	// end of the period they name).
	ResolvedDate string `json:"resolved_date,omitempty" yaml:"resolved_date,omitempty"`
	// Granularity records how much of the date the user specified (year/month/day).
	Granularity Granularity `json:"granularity,omitempty" yaml:"granularity,omitempty"`
	// Source is where the pin came from: "flag", "env", or "config".
	Source string `json:"source,omitempty" yaml:"source,omitempty"`
	// Overrides lists each default the pin rolls back: FromValue is the pinned
	// (effective) value, ToValue the latest default the project would get by unpinning.
	Overrides []Change `json:"overrides" yaml:"overrides"`
}

// DescribePin resolves a raw edition string into its full description. An empty
// raw string yields an unpinned description with no overrides. Source is
// caller-provided provenance ("flag", "env", or "config").
func DescribePin(raw, source string) (Pin, error) {
	defer perf.Track(nil, "edition.DescribePin")()

	if raw == "" {
		return Pin{Pinned: false}, nil
	}

	anchor, err := ParseAnchor(raw)
	if err != nil {
		return Pin{}, err
	}

	return Pin{
		Pinned:       true,
		Edition:      raw,
		ResolvedDate: anchor.Date.Format(dateLayout),
		Granularity:  anchor.Granularity,
		Source:       source,
		Overrides:    Diff(&anchor, nil),
	}, nil
}
