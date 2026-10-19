package azure

import (
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGoDurationToISO8601(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"8h", "PT8H"},
		{"90m", "PT1H30M"},
		{"30m", "PT30M"},
		{"45s", "PT45S"},
		{"1h30m15s", "PT1H30M15S"},
		{"2h0m0s", "PT2H"},
		{"0s", ""},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			d, err := time.ParseDuration(tt.in)
			require.NoError(t, err)
			assert.Equal(t, tt.want, goDurationToISO8601(d))
		})
	}
}

func TestParseISO8601Duration(t *testing.T) {
	validTests := []struct {
		in   string
		want time.Duration
	}{
		{"PT8H", 8 * time.Hour},
		{"PT7H", 7 * time.Hour},
		{"PT1H30M", 90 * time.Minute},
		{"PT30M", 30 * time.Minute},
		{"PT45S", 45 * time.Second},
		{"PT1H30M15S", 1*time.Hour + 30*time.Minute + 15*time.Second},
		{"P1D", 24 * time.Hour},
		{"P1DT8H", 32 * time.Hour},
		{"P1DT1H", 25 * time.Hour},
		{"P2W", 14 * 24 * time.Hour},
		{"P1Y", 365 * 24 * time.Hour},
		{"P2Y", 2 * 365 * 24 * time.Hour},
		{"p1y", 365 * 24 * time.Hour},
		{"P1M", 30 * 24 * time.Hour},
		{"P6M", 6 * 30 * 24 * time.Hour},
		{"p1m", 30 * 24 * time.Hour},
		{"P1Y2M3W4DT5H6M7S", time.Duration(1*365*24+2*30*24+3*7*24+4*24+5)*time.Hour + 6*time.Minute + 7*time.Second},
		{"PT0.5H", 30 * time.Minute},
		{"PT0.5M", 30 * time.Second},
		{"PT0.5S", 500 * time.Millisecond},
		{"PT0.1S", 100 * time.Millisecond},
		{"PT0S", 0},
		{"P0D", 0},
		{"pt8h", 8 * time.Hour},
		{"P90D", 90 * 24 * time.Hour},
		{"P292Y", 292 * 365 * 24 * time.Hour},
	}
	for _, tt := range validTests {
		t.Run(tt.in, func(t *testing.T) {
			got, err := parseISO8601Duration(tt.in)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}

	invalidTests := []string{
		"",
		"   ",
		"8h",
		"P",
		"PT",
		"P.",
		"PT.",
		"P1X",
		"PT1",
		"invalid",
		"P1D1H",
		"P.Y",
		"P.M",
		"P.W",
		"P.D",
		"P1D2",
		"P1D!",
		"PT.H",
		"PT.M",
		"PT.S",
		"PT1H2",
		"PT1X",
		"PT!",
		"P300Y",
		"P293Y",
		"P1000Y",
		"P1000000000000Y",
		"P4000M",
		"P20000W",
		"P110000D",
		"PT3000000H",
		"PT160000000M",
		"PT10000000000S",
		"PT9223372036854775808S",
		"P292YT10000H",
	}
	for _, in := range invalidTests {
		t.Run("invalid_"+in, func(t *testing.T) {
			_, err := parseISO8601Duration(in)
			assert.Error(t, err)
		})
	}
}

func TestAddUnit(t *testing.T) {
	// Normal additions.
	d, err := addUnit(0, 5, time.Hour)
	require.NoError(t, err)
	assert.Equal(t, 5*time.Hour, d)

	d, err = addUnit(5*time.Hour, 30, time.Minute)
	require.NoError(t, err)
	assert.Equal(t, 5*time.Hour+30*time.Minute, d)

	// Negative, NaN, Inf values.
	_, err = addUnit(0, -1, time.Hour)
	assert.Error(t, err)

	_, err = addUnit(0, math.NaN(), time.Hour)
	assert.Error(t, err)

	_, err = addUnit(0, math.Inf(1), time.Hour)
	assert.Error(t, err)

	_, err = addUnit(0, math.Inf(-1), time.Hour)
	assert.Error(t, err)

	// Single unit overflow (e.g. 300 years).
	_, err = addUnit(0, 300, 365*24*time.Hour)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "overflows")

	// Floating point overflow beyond float64(math.MaxInt64).
	_, err = addUnit(0, float64(math.MaxInt64)*2, time.Nanosecond)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "overflows")

	// Accumulation overflow: initial total + new unit exceeds math.MaxInt64.
	total := 292 * 365 * 24 * time.Hour
	_, err = addUnit(total, 10000, time.Hour)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "overflows")
}
