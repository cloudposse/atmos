package schema

import (
	"testing"

	"github.com/go-viper/mapstructure/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Compile-time sentinels: a rename of these fields fails the build immediately.
var (
	_ = TerraformCacheMirror{Enabled: true}
	_ = Terraform{Platforms: nil}
)

// TestTerraformCacheMirrorDecode verifies the cache.mirror config decodes through
// the same mapstructure path the config loader uses (pkg/config/load.go).
func TestTerraformCacheMirrorDecode(t *testing.T) {
	raw := map[string]any{
		"enabled": true,
		"mirror": map[string]any{
			"enabled": true,
		},
	}

	var cfg TerraformCacheConfig
	require.NoError(t, mapstructure.Decode(raw, &cfg))

	require.NotNil(t, cfg.Mirror)
	assert.True(t, cfg.Mirror.Enabled)
}

// TestTerraformPlatformsDecode verifies the project-level components.terraform.platforms
// list decodes through the mapstructure path the config loader uses. This is the single
// source of truth consumed by both `cache mirror` and the post-init `providers lock`.
func TestTerraformPlatformsDecode(t *testing.T) {
	raw := map[string]any{
		"platforms": []any{"linux_amd64", "darwin_arm64", "windows_amd64"},
	}

	var cfg Terraform
	require.NoError(t, mapstructure.Decode(raw, &cfg))

	require.Len(t, cfg.Platforms, 3)
	assert.Equal(t, "linux_amd64", cfg.Platforms[0])
	assert.Equal(t, "windows_amd64", cfg.Platforms[2])
}

// TestTerraformCacheMirrorOmitted verifies Mirror is nil when not configured.
func TestTerraformCacheMirrorOmitted(t *testing.T) {
	raw := map[string]any{"enabled": true}

	var cfg TerraformCacheConfig
	require.NoError(t, mapstructure.Decode(raw, &cfg))

	assert.Nil(t, cfg.Mirror)
}
