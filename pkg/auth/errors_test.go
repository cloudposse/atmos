package auth

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestFormatProfile(t *testing.T) {
	tests := []struct {
		name     string
		profiles []string
		expected string
	}{
		{
			name:     "nil profiles",
			profiles: nil,
			expected: "(not set)",
		},
		{
			name:     "empty profiles",
			profiles: []string{},
			expected: "(not set)",
		},
		{
			name:     "single profile",
			profiles: []string{"devops"},
			expected: "devops",
		},
		{
			name:     "multiple profiles",
			profiles: []string{"ci", "developer"},
			expected: "ci, developer",
		},
		{
			name:     "three profiles",
			profiles: []string{"admin", "devops", "ci"},
			expected: "admin, devops, ci",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := FormatProfile(tt.profiles)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestFormatExpiration(t *testing.T) {
	testTime := time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC)

	tests := []struct {
		name       string
		expiration *time.Time
		expected   string
	}{
		{
			name:       "nil expiration",
			expiration: nil,
			expected:   "",
		},
		{
			name:       "valid expiration",
			expiration: &testTime,
			expected:   "2024-01-15T10:30:00Z",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := FormatExpiration(tt.expiration)
			assert.Equal(t, tt.expected, result)
		})
	}
}
