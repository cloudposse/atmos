package duration

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseString(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    time.Duration
		wantErr error
	}{
		// Go durations.
		{name: "seconds", input: "30s", want: 30 * time.Second},
		{name: "milliseconds", input: "500ms", want: 500 * time.Millisecond},
		{name: "compound", input: "1h30m", want: 90 * time.Minute},
		{name: "decimal seconds", input: "1.5s", want: 1500 * time.Millisecond},
		{name: "negative go duration", input: "-1h", want: -time.Hour},
		{name: "zero", input: "0", want: 0},
		{name: "zero with unit", input: "0s", want: 0},
		// Bare numbers are seconds.
		{name: "bare integer", input: "10", want: 10 * time.Second},
		{name: "bare large integer", input: "3600", want: time.Hour},
		{name: "bare decimal", input: "1.5", want: 1500 * time.Millisecond},
		{name: "bare negative", input: "-100", want: -100 * time.Second},
		// Days.
		{name: "days", input: "7d", want: 7 * 24 * time.Hour},
		{name: "one day", input: "1d", want: 24 * time.Hour},
		{name: "days and hours", input: "1d12h", want: 36 * time.Hour},
		{name: "decimal days", input: "0.5d", want: 12 * time.Hour},
		// Keywords.
		{name: "minute keyword", input: "minute", want: time.Minute},
		{name: "hourly keyword", input: "hourly", want: time.Hour},
		{name: "daily keyword", input: "daily", want: 24 * time.Hour},
		{name: "weekly keyword", input: "weekly", want: 7 * 24 * time.Hour},
		{name: "monthly keyword", input: "monthly", want: 30 * 24 * time.Hour},
		{name: "yearly keyword", input: "yearly", want: 365 * 24 * time.Hour},
		{name: "keyword case insensitive", input: "Daily", want: 24 * time.Hour},
		// ISO-8601.
		{name: "iso hours", input: "PT8H", want: 8 * time.Hour},
		{name: "iso day and hours", input: "P1DT12H", want: 36 * time.Hour},
		// Whitespace.
		{name: "whitespace", input: "  1h  ", want: time.Hour},
		// Errors.
		{name: "empty", input: "", wantErr: ErrInvalid},
		{name: "blank", input: "   ", wantErr: ErrInvalid},
		{name: "garbage", input: "invalid", wantErr: ErrInvalid},
		{name: "unknown unit", input: "5x", wantErr: ErrInvalid},
		{name: "days with bad tail", input: "1dxyz", wantErr: ErrInvalid},
		{name: "iso garbage", input: "PX", wantErr: ErrInvalid},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseString(tt.input)
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestParseAny(t *testing.T) {
	tests := []struct {
		name    string
		input   any
		want    time.Duration
		wantErr error
	}{
		{name: "int is seconds", input: 10, want: 10 * time.Second},
		{name: "int64 is seconds", input: int64(90), want: 90 * time.Second},
		{name: "uint is seconds", input: uint(5), want: 5 * time.Second},
		{name: "float is seconds", input: 1.5, want: 1500 * time.Millisecond},
		{name: "negative int", input: -3, want: -3 * time.Second},
		{name: "string", input: "500ms", want: 500 * time.Millisecond},
		{name: "duration passthrough", input: 2 * time.Minute, want: 2 * time.Minute},
		{name: "nil", input: nil, wantErr: ErrInvalid},
		{name: "bool", input: true, wantErr: ErrInvalid},
		{name: "map", input: map[string]any{}, wantErr: ErrInvalid},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseAny(tt.input)
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestParseDurationRequiresPositive(t *testing.T) {
	for _, in := range []string{"0", "0s", "-1h", "-5"} {
		_, err := ParseDuration(in)
		require.ErrorIs(t, err, ErrNotPositive, in)
	}
	d, err := ParseDuration("1h")
	require.NoError(t, err)
	assert.Equal(t, time.Hour, d)
	_, err = ParseDuration("bogus")
	require.ErrorIs(t, err, ErrInvalid)
}

func TestParseNonNegativeAllowsZero(t *testing.T) {
	d, err := ParseNonNegative("0")
	require.NoError(t, err)
	assert.Equal(t, time.Duration(0), d)
	_, err = ParseNonNegative("-1s")
	require.ErrorIs(t, err, ErrNegative)
}

func TestParse(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    int64
		wantErr error
	}{
		{name: "integer seconds", input: "3600", want: 3600},
		{name: "small integer", input: "60", want: 60},
		{name: "seconds suffix", input: "30s", want: 30},
		{name: "minutes suffix", input: "5m", want: 300},
		{name: "hours suffix", input: "1h", want: 3600},
		{name: "compound", input: "1h30m", want: 5400},
		{name: "days suffix", input: "7d", want: 604800},
		{name: "daily keyword", input: "daily", want: 86400},
		{name: "weekly keyword", input: "weekly", want: 604800},
		{name: "monthly keyword", input: "monthly", want: 2592000},
		{name: "yearly keyword", input: "yearly", want: 31536000},
		{name: "whitespace", input: "  1h  ", want: 3600},
		{name: "truncates sub-second part", input: "1.9s", want: 1},
		{name: "invalid string", input: "invalid", wantErr: ErrInvalid},
		{name: "empty string", input: "", wantErr: ErrInvalid},
		{name: "negative integer", input: "-100", wantErr: ErrNotPositive},
		{name: "zero", input: "0", wantErr: ErrNotPositive},
		{name: "sub-second only", input: "500ms", wantErr: ErrNotPositive},
		{name: "unknown unit", input: "5x", wantErr: ErrInvalid},
		{name: "negative with unit", input: "-1h", wantErr: ErrNotPositive},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Parse(tt.input)
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestOverflowProtection(t *testing.T) {
	for _, in := range []string{"9223372036854775807d", "9223372036854775807", "99999999999d", "1e300"} {
		_, err := ParseString(in)
		require.Error(t, err, in)
		require.True(t, errors.Is(err, ErrOverflow) || errors.Is(err, ErrInvalid), "%s: %v", in, err)
	}
	// A large but representable value still parses.
	d, err := ParseString("100000d")
	require.NoError(t, err)
	assert.Equal(t, 100000*24*time.Hour, d)
}

func TestIsZeroTTL(t *testing.T) {
	for _, in := range []string{"0", "0s", "0m", "0h", "0d", " 0 "} {
		assert.True(t, IsZeroTTL(in), in)
	}
	for _, in := range []string{"", "1s", "abc", "-1s"} {
		assert.False(t, IsZeroTTL(in), in)
	}
}

func TestIsExpired(t *testing.T) {
	now := time.Now()
	expired, err := IsExpired(now, "0")
	require.NoError(t, err)
	assert.True(t, expired)

	expired, err = IsExpired(now.Add(-2*time.Hour), "1h")
	require.NoError(t, err)
	assert.True(t, expired)

	expired, err = IsExpired(now, "1h")
	require.NoError(t, err)
	assert.False(t, expired)

	_, err = IsExpired(now, "bogus")
	require.ErrorIs(t, err, ErrInvalid)
}

func TestIs(t *testing.T) {
	_, err := ParseDuration("0")
	assert.True(t, Is(err))
	_, err = ParseString("x")
	assert.True(t, Is(err))
	assert.False(t, Is(errors.New("other")))
	assert.False(t, Is(nil))
}
