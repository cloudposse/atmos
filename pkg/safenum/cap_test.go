package safenum

import (
	"math"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCap(t *testing.T) {
	tests := []struct {
		name string
		a, b int
		max  int
		want int
	}{
		{name: "simple sum under max", a: 3, b: 4, max: 100, want: 7},
		{name: "zero values", a: 0, b: 0, max: 100, want: 0},
		{name: "sum exactly at max", a: 40, b: 60, max: 100, want: 100},
		{name: "sum exceeds max", a: 60, b: 60, max: 100, want: 100},
		{name: "a alone exceeds max", a: 200, b: 5, max: 100, want: 100},
		{name: "would overflow int addition", a: math.MaxInt - 1, b: 5, max: 1 << 24, want: 1 << 24},
		{name: "doubling shape via a==b", a: 10, b: 10, max: 100, want: 20},
		{name: "doubling shape clamped", a: 80, b: 80, max: 100, want: 100},
		{name: "negative inputs treated as zero", a: -5, b: -1, max: 100, want: 0},
		{name: "negative max treated as zero", a: 5, b: 5, max: -1, want: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Cap(tt.a, tt.b, tt.max)
			require.Equal(t, tt.want, got)
			require.LessOrEqual(t, got, max(tt.max, 0))
		})
	}
}
