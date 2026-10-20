package duration

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFormatISO8601(t *testing.T) {
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
		{"-5m", ""},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			d, err := time.ParseDuration(tt.in)
			require.NoError(t, err)
			assert.Equal(t, tt.want, FormatISO8601(d))
		})
	}
}

func TestParseISO8601(t *testing.T) {
	validTests := []struct {
		in   string
		want time.Duration
	}{
		{"PT8H", 8 * time.Hour},
		{"PT1H30M", 90 * time.Minute},
		{"PT30M", 30 * time.Minute},
		{"PT45S", 45 * time.Second},
		{"PT1H30M15S", 1*time.Hour + 30*time.Minute + 15*time.Second},
		{"P1D", 24 * time.Hour},
		{"P1DT8H", 32 * time.Hour},
		{"P2W", 14 * 24 * time.Hour},
		{"P1Y", 365 * 24 * time.Hour},
		{"p1y", 365 * 24 * time.Hour},
		{"P1M", 30 * 24 * time.Hour},
		{"P6M", 6 * 30 * 24 * time.Hour},
		{"P1Y2M3W4DT5H6M7S", time.Duration(1*365*24+2*30*24+3*7*24+4*24+5)*time.Hour + 6*time.Minute + 7*time.Second},
		{"PT0.5H", 30 * time.Minute},
		{"PT0.5M", 30 * time.Second},
		{"PT0.5S", 500 * time.Millisecond},
		{"PT0S", 0},
		{"  PT1H  ", time.Hour},
	}
	for _, tt := range validTests {
		t.Run(tt.in, func(t *testing.T) {
			got, err := ParseISO8601(tt.in)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}

	invalidTests := []struct {
		in      string
		wantErr error
	}{
		{"", ErrISO8601Empty},
		{"8H", ErrISO8601Prefix},
		{"P", ErrISO8601Designator},
		{"PT", ErrISO8601Designator},
		{"PT8X", ErrISO8601Designator},
		{"PTH", ErrISO8601Designator},
		{"PT8", ErrISO8601Designator},
		{"PT99999999999999999H", ErrOverflow},
	}
	for _, tt := range invalidTests {
		t.Run("invalid "+tt.in, func(t *testing.T) {
			_, err := ParseISO8601(tt.in)
			require.ErrorIs(t, err, tt.wantErr)
			if !errors.Is(tt.wantErr, ErrOverflow) {
				require.ErrorIs(t, err, ErrInvalid)
			}
		})
	}
}

func TestParseISO8601RoundTrip(t *testing.T) {
	for _, in := range []time.Duration{8 * time.Hour, 90 * time.Minute, 45 * time.Second, 1*time.Hour + 30*time.Minute + 15*time.Second} {
		got, err := ParseISO8601(FormatISO8601(in))
		require.NoError(t, err)
		assert.Equal(t, in, got)
	}
}
