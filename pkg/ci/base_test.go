package ci

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsCommitSHA(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		{name: "full SHA lowercase", input: "3a5eafeab90426bd82bf5899896b28cc0bab3073", expected: true},
		{name: "full SHA uppercase", input: "3A5EAFEAB90426BD82BF5899896B28CC0BAB3073", expected: true},
		{name: "short SHA 7 chars", input: "abc1234", expected: true},
		{name: "short SHA 8 chars", input: "abcd1234", expected: true},
		{name: "too short 6 chars", input: "abc123", expected: false},
		{name: "too long 41 chars", input: "3a5eafeab90426bd82bf5899896b28cc0bab30731", expected: false},
		{name: "empty string", input: "", expected: false},
		{name: "branch name", input: "main", expected: false},
		{name: "ref path", input: "refs/heads/main", expected: false},
		{name: "mixed with non-hex", input: "abcdefg", expected: false},
		{name: "contains slash", input: "abc123/def", expected: false},
		{name: "valid 10 char hex", input: "0123456789", expected: true},
		{name: "all zeros", input: "0000000", expected: true},
		{name: "mixed case hex", input: "aAbBcCdDeEfF01234567890123456789012345678", expected: false}, // 41 chars.
		{name: "40 char mixed case", input: "aAbBcCdDeEfF0123456789012345678901234567", expected: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, IsCommitSHA(tt.input))
		})
	}
}
