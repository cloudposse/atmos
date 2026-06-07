package rc

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRender_ProviderInstallationOrderedMethods(t *testing.T) {
	// provider_installation is a list of single-key method maps. List order is
	// precedence and must be preserved: network_mirror then direct.
	rc := map[string]any{
		"provider_installation": []any{
			map[string]any{"network_mirror": map[string]any{"url": "https://mirror.example.com/"}},
			map[string]any{"direct": map[string]any{"exclude": []any{"registry.terraform.io/hashicorp/*"}}},
		},
	}

	out, err := Render(rc)
	require.NoError(t, err)
	s := string(out)

	assert.Contains(t, s, "provider_installation {")
	assert.Contains(t, s, "network_mirror {")
	assert.Contains(t, s, `url = "https://mirror.example.com/"`)
	assert.Contains(t, s, "direct {")
	assert.Contains(t, s, `exclude = ["registry.terraform.io/hashicorp/*"]`)

	// network_mirror must come before direct (precedence ordering preserved).
	assert.Less(t, strings.Index(s, "network_mirror {"), strings.Index(s, "direct {"))
}

func TestRender_EmptyMethodBlock(t *testing.T) {
	// `direct: {}` (or null) renders an empty block, not an attribute.
	rc := map[string]any{
		"provider_installation": []any{
			map[string]any{"network_mirror": map[string]any{"url": "http://127.0.0.1:5000/"}},
			map[string]any{"direct": nil},
		},
	}

	out, err := Render(rc)
	require.NoError(t, err)
	s := string(out)
	assert.Contains(t, s, "direct {")
	assert.NotContains(t, s, "direct =")
}

func TestRender_LabeledHostBlock(t *testing.T) {
	rc := map[string]any{
		"host": map[string]any{
			"registry.terraform.io": map[string]any{
				"services": map[string]any{
					"modules.v1": "https://modules.example.com/v1/modules/",
				},
			},
		},
	}

	out, err := Render(rc)
	require.NoError(t, err)
	s := string(out)
	assert.Contains(t, s, `host "registry.terraform.io" {`)
	assert.Contains(t, s, `"modules.v1" = "https://modules.example.com/v1/modules/"`)
}

func TestRender_CredentialsBlock(t *testing.T) {
	rc := map[string]any{
		"credentials": map[string]any{
			"app.terraform.io": map[string]any{"token": "xxxx"},
		},
	}
	out, err := Render(rc)
	require.NoError(t, err)
	s := string(out)
	assert.Contains(t, s, `credentials "app.terraform.io" {`)
	assert.Contains(t, s, `token = "xxxx"`)
}

func TestRender_TopLevelScalars(t *testing.T) {
	rc := map[string]any{
		"plugin_cache_dir":   "/home/user/.terraform.d/plugin-cache",
		"disable_checkpoint": true,
	}
	out, err := Render(rc)
	require.NoError(t, err)
	// hclwrite aligns '=' with padding when multiple attributes are present, so
	// normalize runs of spaces before asserting.
	s := strings.Join(strings.Fields(string(out)), " ")
	assert.Contains(t, s, `plugin_cache_dir = "/home/user/.terraform.d/plugin-cache"`)
	assert.Contains(t, s, "disable_checkpoint = true")
}

func TestRender_Deterministic(t *testing.T) {
	rc := map[string]any{
		"plugin_cache_dir":   "/cache",
		"disable_checkpoint": true,
		"host": map[string]any{
			"registry.terraform.io": map[string]any{
				"services": map[string]any{"modules.v1": "https://m/"},
			},
		},
	}
	first, err := Render(rc)
	require.NoError(t, err)
	for i := 0; i < 5; i++ {
		again, err := Render(rc)
		require.NoError(t, err)
		assert.Equal(t, string(first), string(again), "Render must be deterministic")
	}
}

func TestRender_Errors(t *testing.T) {
	tests := []struct {
		name string
		rc   map[string]any
	}{
		{
			name: "provider_installation wrong type",
			rc:   map[string]any{"provider_installation": "not-a-list"},
		},
		{
			name: "labeled block value not a map",
			rc:   map[string]any{"host": map[string]any{"x": "not-a-map"}},
		},
		{
			name: "unsupported scalar type",
			rc:   map[string]any{"plugin_cache_dir": struct{}{}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Render(tt.rc)
			assert.Error(t, err)
		})
	}
}
