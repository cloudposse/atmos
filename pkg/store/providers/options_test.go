package providers

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFirstNonEmptyStringPtr(t *testing.T) {
	ptr := func(s string) *string { return &s }
	empty := ""

	tests := []struct {
		name   string
		values []*string
		want   string
	}{
		{
			name:   "no arguments",
			values: nil,
			want:   "",
		},
		{
			name:   "all nil pointers",
			values: []*string{nil, nil},
			want:   "",
		},
		{
			name:   "single empty string pointer",
			values: []*string{&empty},
			want:   "",
		},
		{
			name:   "nil then empty then value",
			values: []*string{nil, &empty, ptr("third")},
			want:   "third",
		},
		{
			name:   "first non-empty wins over later values",
			values: []*string{ptr("first"), ptr("second")},
			want:   "first",
		},
		{
			name:   "empty first falls through to second",
			values: []*string{ptr(""), ptr("second")},
			want:   "second",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, firstNonEmptyStringPtr(tt.values...))
		})
	}
}
