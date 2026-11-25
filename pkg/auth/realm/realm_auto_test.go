package realm

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestHasAutoValueFormat(t *testing.T) {
	tests := []struct {
		value string
		want  bool
	}{
		{value: "3ed941021e53d074", want: true},
		{value: "0000000000000000", want: true},
		{value: "3ed941021e53d07", want: false},   // Too short.
		{value: "3ed941021e53d0745", want: false}, // Too long.
		{value: "3ED941021E53D074", want: false},  // Uppercase is never produced.
		{value: "3ed941021e53d07g", want: false},  // Not hexadecimal.
		{value: "3ed941021e53_074", want: false},  // Contains "_".
		{value: "acme", want: false},
		{value: "default", want: false},
		{value: "", want: false},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.want, HasAutoValueFormat(tt.value), "value %q", tt.value)
	}
}
