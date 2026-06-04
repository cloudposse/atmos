package hooks

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewSkipPredicate(t *testing.T) {
	tests := []struct {
		name     string
		raw      string
		hookName string
		wantSkip bool
	}{
		{name: "empty runs everything", raw: "", hookName: "cost", wantSkip: false},
		{name: "explicit false runs everything", raw: "false", hookName: "cost", wantSkip: false},
		{name: "False case-insensitive", raw: "False", hookName: "cost", wantSkip: false},

		{name: "star skips all", raw: "*", hookName: "anything", wantSkip: true},
		{name: "true skips all", raw: "true", hookName: "anything", wantSkip: true},
		{name: "True case-insensitive", raw: "True", hookName: "anything", wantSkip: true},

		{name: "single name matches", raw: "cost", hookName: "cost", wantSkip: true},
		{name: "single name doesn't match other", raw: "cost", hookName: "security", wantSkip: false},

		{name: "comma list matches first", raw: "cost,security", hookName: "cost", wantSkip: true},
		{name: "comma list matches second", raw: "cost,security", hookName: "security", wantSkip: true},
		{name: "comma list misses absent name", raw: "cost,security", hookName: "audit", wantSkip: false},

		{name: "tolerates whitespace around names", raw: "  cost ,  security ", hookName: "security", wantSkip: true},
		{name: "tolerates trailing comma", raw: "cost,", hookName: "cost", wantSkip: true},
		{name: "tolerates empty list element", raw: ",,cost", hookName: "cost", wantSkip: true},

		// Hook name is case-sensitive — matching by exact name is the contract
		// users see, mirroring how stack YAML keys are matched.
		{name: "case sensitive miss", raw: "Cost", hookName: "cost", wantSkip: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pred := newSkipPredicate(tt.raw)
			assert.Equal(t, tt.wantSkip, pred(tt.hookName))
		})
	}
}
