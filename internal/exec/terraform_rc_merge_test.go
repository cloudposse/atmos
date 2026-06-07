package exec

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMergeCacheContribution_CacheOwnsProviderInstallation(t *testing.T) {
	rcMap := map[string]any{
		"provider_installation": []any{
			map[string]any{"direct": map[string]any{}},
		},
	}
	contribution := map[string]any{
		"provider_installation": []any{
			map[string]any{"network_mirror": map[string]any{"url": "http://127.0.0.1:5000/providers/"}},
			map[string]any{"direct": map[string]any{}},
		},
	}

	mergeCacheContribution(rcMap, contribution)

	pi, ok := rcMap["provider_installation"].([]any)
	require.True(t, ok)
	require.Len(t, pi, 2, "cache provider_installation must replace the user's")
	nm := pi[0].(map[string]any)["network_mirror"].(map[string]any)
	assert.Equal(t, "http://127.0.0.1:5000/providers/", nm["url"])
}

func TestMergeCacheContribution_MergesHostsPreservingUserHosts(t *testing.T) {
	rcMap := map[string]any{
		"host": map[string]any{
			"private.example.com": map[string]any{
				"services": map[string]any{"modules.v1": "https://private.example.com/v1/modules/"},
			},
		},
	}
	contribution := map[string]any{
		"host": map[string]any{
			"registry.terraform.io": map[string]any{
				"services": map[string]any{"modules.v1": "http://127.0.0.1:5000/modules/registry.terraform.io/"},
			},
		},
	}

	mergeCacheContribution(rcMap, contribution)

	hosts := rcMap["host"].(map[string]any)
	assert.Contains(t, hosts, "private.example.com", "user host must be preserved")
	assert.Contains(t, hosts, "registry.terraform.io", "cache host must be added")
}
