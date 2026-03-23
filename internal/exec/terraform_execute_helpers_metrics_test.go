package exec

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestFormatDuration(t *testing.T) {
	tests := []struct {
		name     string
		duration time.Duration
		expected string
	}{
		{"zero", 0, "0ms"},
		{"milliseconds", 500 * time.Millisecond, "500ms"},
		{"sub-second", 999 * time.Millisecond, "999ms"},
		{"one second", time.Second, "1.0s"},
		{"seconds with fraction", 2500 * time.Millisecond, "2.5s"},
		{"minutes", 90 * time.Second, "90.0s"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.expected, formatDuration(tc.duration))
		})
	}
}

func TestFormatBytes(t *testing.T) {
	tests := []struct {
		name     string
		bytes    int64
		expected string
	}{
		{"zero bytes", 0, "0 B"},
		{"small bytes", 512, "512 B"},
		{"kilobytes", 1024, "1.0 KB"},
		{"megabytes", 1024 * 1024, "1.0 MB"},
		{"512 MB", 512 * 1024 * 1024, "512.0 MB"},
		{"gigabytes", 1024 * 1024 * 1024, "1.0 GB"},
		{"1.5 GB", 1536 * 1024 * 1024, "1.5 GB"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.expected, formatBytes(tc.bytes))
		})
	}
}
