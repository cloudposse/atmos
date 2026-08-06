package yaml

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestLooksNonString(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want bool
	}{
		{"bool true", "true", true},
		{"bool false", "false", true},
		{"bool mixed case", "True", true},
		{"integer", "42", true},
		{"negative integer", "-5", true},
		{"float", "3.14", true},
		{"region string", "us-east-1", false},
		{"plain word", "production", false},
		{"empty string", "", false},
		{"string containing a number", "v1.2.3", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, LooksNonString(tt.raw))
		})
	}
}
