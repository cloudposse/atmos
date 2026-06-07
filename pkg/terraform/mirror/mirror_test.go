package mirror

import (
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/cloudposse/atmos/pkg/schema"
)

func TestResolvePlatforms(t *testing.T) {
	host := runtime.GOOS + "_" + runtime.GOARCH

	tests := []struct {
		name     string
		flag     []string
		cache    *schema.TerraformCacheConfig
		expected []string
	}{
		{
			name:     "flag overrides config and host",
			flag:     []string{"linux_amd64", "darwin_arm64"},
			cache:    &schema.TerraformCacheConfig{Mirror: &schema.TerraformCacheMirror{Platforms: []string{"windows_amd64"}}},
			expected: []string{"linux_amd64", "darwin_arm64"},
		},
		{
			name:     "config used when no flag",
			flag:     nil,
			cache:    &schema.TerraformCacheConfig{Mirror: &schema.TerraformCacheMirror{Platforms: []string{"linux_amd64", "windows_amd64"}}},
			expected: []string{"linux_amd64", "windows_amd64"},
		},
		{
			name:     "host fallback when flag and config empty",
			flag:     nil,
			cache:    &schema.TerraformCacheConfig{Mirror: &schema.TerraformCacheMirror{}},
			expected: []string{host},
		},
		{
			name:     "host fallback when mirror is nil",
			flag:     nil,
			cache:    &schema.TerraformCacheConfig{},
			expected: []string{host},
		},
		{
			name:     "host fallback when cache is nil",
			flag:     nil,
			cache:    nil,
			expected: []string{host},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ResolvePlatforms(tt.flag, tt.cache)
			assert.Equal(t, tt.expected, got)
		})
	}
}
