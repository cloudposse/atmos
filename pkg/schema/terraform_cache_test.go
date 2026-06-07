package schema

import (
	"testing"

	"github.com/go-viper/mapstructure/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Compile-time sentinel: a rename of these fields fails the build immediately.
var _ = TerraformCacheMirror{Enabled: true, Platforms: nil}

// TestTerraformCacheMirrorDecode verifies the cache.mirror config decodes through
// the same mapstructure path the config loader uses (pkg/config/load.go).
func TestTerraformCacheMirrorDecode(t *testing.T) {
	raw := map[string]any{
		"enabled": true,
		"mirror": map[string]any{
			"enabled":   true,
			"platforms": []any{"linux_amd64", "darwin_arm64", "windows_amd64"},
		},
	}

	var cfg TerraformCacheConfig
	require.NoError(t, mapstructure.Decode(raw, &cfg))

	require.NotNil(t, cfg.Mirror)
	assert.True(t, cfg.Mirror.Enabled)
	assert.Equal(t, []string{"linux_amd64", "darwin_arm64", "windows_amd64"}, cfg.Mirror.Platforms)
}

// TestTerraformCacheMirrorOmitted verifies Mirror is nil when not configured.
func TestTerraformCacheMirrorOmitted(t *testing.T) {
	raw := map[string]any{"enabled": true}

	var cfg TerraformCacheConfig
	require.NoError(t, mapstructure.Decode(raw, &cfg))

	assert.Nil(t, cfg.Mirror)
}
