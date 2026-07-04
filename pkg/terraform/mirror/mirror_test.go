package mirror

import (
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestResolvePlatforms(t *testing.T) {
	host := runtime.GOOS + "_" + runtime.GOARCH

	tests := []struct {
		name       string
		flag       []string
		configured []string
		expected   []string
	}{
		{
			name:       "flag overrides config and host",
			flag:       []string{"linux_amd64", "darwin_arm64"},
			configured: []string{"windows_amd64"},
			expected:   []string{"linux_amd64", "darwin_arm64"},
		},
		{
			name:       "config used when no flag",
			flag:       nil,
			configured: []string{"linux_amd64", "windows_amd64"},
			expected:   []string{"linux_amd64", "windows_amd64"},
		},
		{
			name:       "host fallback when flag and config empty",
			flag:       nil,
			configured: nil,
			expected:   []string{host},
		},
		{
			name:       "host fallback when config is empty slice",
			flag:       nil,
			configured: []string{},
			expected:   []string{host},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ResolvePlatforms(tt.flag, tt.configured)
			assert.Equal(t, tt.expected, got)
		})
	}
}
