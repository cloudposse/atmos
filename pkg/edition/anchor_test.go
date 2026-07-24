package edition

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseAnchor(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		wantDate    time.Time
		granularity Granularity
	}{
		{
			name:        "year rounds to December 31",
			input:       "2026",
			wantDate:    time.Date(2026, time.December, 31, 0, 0, 0, 0, time.UTC),
			granularity: GranularityYear,
		},
		{
			name:        "month rounds to last day of month",
			input:       "2026-07",
			wantDate:    time.Date(2026, time.July, 31, 0, 0, 0, 0, time.UTC),
			granularity: GranularityMonth,
		},
		{
			name:        "February rounds to 28 in a non-leap year",
			input:       "2026-02",
			wantDate:    time.Date(2026, time.February, 28, 0, 0, 0, 0, time.UTC),
			granularity: GranularityMonth,
		},
		{
			name:        "February rounds to 29 in a leap year",
			input:       "2024-02",
			wantDate:    time.Date(2024, time.February, 29, 0, 0, 0, 0, time.UTC),
			granularity: GranularityMonth,
		},
		{
			name:        "full date is used as-is",
			input:       "2026-07-16",
			wantDate:    time.Date(2026, time.July, 16, 0, 0, 0, 0, time.UTC),
			granularity: GranularityDay,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			anchor, err := ParseAnchor(tt.input)
			require.NoError(t, err)
			assert.Equal(t, tt.wantDate, anchor.Date)
			assert.Equal(t, tt.input, anchor.Raw)
			assert.Equal(t, tt.granularity, anchor.Granularity)
		})
	}
}

func TestParseAnchorInvalid(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{name: "empty", input: ""},
		{name: "two-digit year", input: "26"},
		{name: "single-digit month", input: "2026-7"},
		{name: "single-digit day", input: "2026-07-1"},
		{name: "month out of range", input: "2026-13"},
		{name: "month zero", input: "2026-00"},
		{name: "day out of range", input: "2026-02-30"},
		{name: "day zero", input: "2026-07-00"},
		{name: "timestamp suffix", input: "2026-07-16T00:00:00Z"},
		{name: "not a date", input: "latest"},
		{name: "trailing dash", input: "2026-"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseAnchor(tt.input)
			require.ErrorIs(t, err, ErrInvalidEdition)
		})
	}
}
