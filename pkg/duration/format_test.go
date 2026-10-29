package duration

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestFormat(t *testing.T) {
	tests := []struct {
		name string
		d    time.Duration
		want string
	}{
		{name: "sub-second", d: 250 * time.Millisecond, want: "250ms"},
		{name: "zero", d: 0, want: "0ms"},
		{name: "whole seconds", d: 3 * time.Second, want: "3.0s"},
		{name: "fractional seconds", d: 1500 * time.Millisecond, want: "1.5s"},
		{name: "just under a minute", d: 59900 * time.Millisecond, want: "59.9s"},
		{name: "rounds up to a minute", d: 59960 * time.Millisecond, want: "1m"},
		{name: "exact minute", d: time.Minute, want: "1m"},
		{name: "minutes and seconds", d: 2086700 * time.Millisecond, want: "34m 47s"},
		{name: "rounds seconds up", d: 89600 * time.Millisecond, want: "1m 30s"},
		{name: "exact hour", d: time.Hour, want: "1h"},
		{name: "hour and seconds only", d: time.Hour + 3*time.Second, want: "1h 3s"},
		{name: "hours minutes seconds", d: time.Hour + 2*time.Minute + 3*time.Second, want: "1h 2m 3s"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, Format(tt.d))
		})
	}
}
