package duration

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParse(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected int64
		wantErr  bool
	}{
		// Integer seconds.
		{name: "integer seconds", input: "3600", expected: 3600},
		{name: "small integer", input: "60", expected: 60},
		{name: "large integer", input: "86400", expected: 86400},

		// Duration with suffix.
		{name: "seconds suffix", input: "30s", expected: 30},
		{name: "minutes suffix", input: "5m", expected: 300},
		{name: "hours suffix", input: "1h", expected: 3600},
		{name: "hours suffix 2h", input: "2h", expected: 7200},
		{name: "days suffix", input: "7d", expected: 604800},
		{name: "days suffix 1d", input: "1d", expected: 86400},

		// Keywords.
		{name: "minute keyword", input: "minute", expected: 60},
		{name: "hourly keyword", input: "hourly", expected: 3600},
		{name: "daily keyword", input: "daily", expected: 86400},
		{name: "weekly keyword", input: "weekly", expected: 604800},
		{name: "monthly keyword", input: "monthly", expected: 2592000},
		{name: "yearly keyword", input: "yearly", expected: 31536000},

		// Whitespace handling.
		{name: "whitespace prefix", input: "  1h", expected: 3600},
		{name: "whitespace suffix", input: "1h  ", expected: 3600},
		{name: "whitespace both", input: "  daily  ", expected: 86400},

		// Invalid inputs.
		{name: "invalid string", input: "invalid", wantErr: true},
		{name: "empty string", input: "", wantErr: true},
		{name: "negative integer", input: "-100", wantErr: true},
		{name: "zero", input: "0", wantErr: true},
		{name: "unknown unit", input: "5x", wantErr: true},
		{name: "negative with unit", input: "-1h", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := Parse(tt.input)
			if tt.wantErr {
				assert.Error(t, err, "expected error for input %q", tt.input)
			} else {
				require.NoError(t, err, "unexpected error for input %q", tt.input)
				assert.Equal(t, tt.expected, result, "unexpected result for input %q", tt.input)
			}
		})
	}
}

func TestParseDuration(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected time.Duration
		wantErr  bool
	}{
		{name: "1 hour", input: "1h", expected: time.Hour},
		{name: "2 hours", input: "2h", expected: 2 * time.Hour},
		{name: "30 minutes", input: "30m", expected: 30 * time.Minute},
		{name: "7 days", input: "7d", expected: 7 * 24 * time.Hour},
		{name: "daily keyword", input: "daily", expected: 24 * time.Hour},
		{name: "weekly keyword", input: "weekly", expected: 7 * 24 * time.Hour},
		{name: "integer seconds", input: "3600", expected: time.Hour},
		{name: "invalid", input: "invalid", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ParseDuration(tt.input)
			if tt.wantErr {
				assert.Error(t, err, "expected error for input %q", tt.input)
			} else {
				require.NoError(t, err, "unexpected error for input %q", tt.input)
				assert.Equal(t, tt.expected, result, "unexpected result for input %q", tt.input)
			}
		})
	}
}

// Test overflow protection in parseWithSuffix.

func TestParse_OverflowProtection(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		// Value that would overflow when multiplied by day multiplier.
		{name: "overflow days", input: "9223372036854775807d", wantErr: true},
		// Value that would overflow when multiplied by hour multiplier.
		{name: "overflow hours", input: "9223372036854775807h", wantErr: true},
		// Large but valid value.
		{name: "large valid days", input: "100000000d", wantErr: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Parse(tt.input)
			if tt.wantErr {
				assert.Error(t, err, "expected error for input %q", tt.input)
			} else {
				assert.NoError(t, err, "unexpected error for input %q", tt.input)
			}
		})
	}
}

// Test ParseDuration overflow when seconds exceed maxDurationSeconds.

func TestParseDuration_OverflowProtection(t *testing.T) {
	// maxDurationSeconds is approximately 292 years.
	// Test with a value that exceeds this.
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		// ~300 years in seconds (exceeds maxDurationSeconds).
		{name: "exceeds max duration", input: "9467280000", wantErr: true},
		// ~100 years in seconds (valid).
		{name: "large valid seconds", input: "3153600000", wantErr: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseDuration(tt.input)
			if tt.wantErr {
				assert.Error(t, err, "expected error for input %q", tt.input)
			} else {
				assert.NoError(t, err, "unexpected error for input %q", tt.input)
			}
		})
	}
}
