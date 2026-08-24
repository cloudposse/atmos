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

// TestConfigureTerraformRC_CloneDoesNotMutateSourceHosts reproduces the leak where a
// shallow copy of the user's RC config let mergeCacheContribution mutate the shared
// "host" map in place, leaking cache loopback overrides back into atmosConfig for later
// components. The clone path must isolate the source map (result -> src isolation).
func TestConfigureTerraformRC_CloneDoesNotMutateSourceHosts(t *testing.T) {
	// The user's RC config, as it lives on atmosConfig.Components.Terraform.RC.Config.
	sourceConfig := map[string]any{
		"host": map[string]any{
			"private.example.com": map[string]any{
				"services": map[string]any{"modules.v1": "https://private.example.com/v1/modules/"},
			},
		},
	}

	// Mirror the copy loop in configureTerraformRC.
	rcMap := map[string]any{}
	for k, v := range sourceConfig {
		rcMap[k] = cloneRCValue(v)
	}

	// The cache contributes a new host; mergeCacheContribution mutates rcMap["host"] in place.
	mergeCacheContribution(rcMap, map[string]any{
		"host": map[string]any{
			"registry.terraform.io": map[string]any{
				"services": map[string]any{"modules.v1": "http://127.0.0.1:5000/modules/registry.terraform.io/"},
			},
		},
	})

	// Result has both hosts.
	resultHosts := rcMap["host"].(map[string]any)
	assert.Contains(t, resultHosts, "private.example.com")
	assert.Contains(t, resultHosts, "registry.terraform.io")

	// The SOURCE must be untouched: the cache host must not have leaked into it.
	srcHosts := sourceConfig["host"].(map[string]any)
	assert.NotContains(t, srcHosts, "registry.terraform.io", "cache host leaked into source RC config")
	assert.Len(t, srcHosts, 1, "source host map must still contain only the user's host")
}

// TestCloneRCValue verifies deep isolation in both directions: mutating the clone must
// not affect the source, and mutating the source after cloning must not affect the clone.
func TestCloneRCValue(t *testing.T) {
	src := map[string]any{
		"host": map[string]any{
			"a.example.com": map[string]any{"services": map[string]any{"modules.v1": "https://a/"}},
		},
		"list":   []any{"x", map[string]any{"k": "v"}},
		"scalar": "unchanged",
	}

	clone, ok := cloneRCValue(src).(map[string]any)
	require.True(t, ok)

	// Mutate the clone deeply.
	clone["host"].(map[string]any)["a.example.com"].(map[string]any)["services"].(map[string]any)["modules.v1"] = "MUTATED"
	clone["list"].([]any)[1].(map[string]any)["k"] = "MUTATED"

	// Source must be unaffected (result -> src isolation).
	srcSvc := src["host"].(map[string]any)["a.example.com"].(map[string]any)["services"].(map[string]any)
	assert.Equal(t, "https://a/", srcSvc["modules.v1"])
	assert.Equal(t, "v", src["list"].([]any)[1].(map[string]any)["k"])

	// Mutate the source after cloning; the clone must be unaffected (src -> result isolation).
	src["host"].(map[string]any)["a.example.com"].(map[string]any)["services"].(map[string]any)["modules.v1"] = "SRC2"
	assert.Equal(t, "MUTATED", clone["host"].(map[string]any)["a.example.com"].(map[string]any)["services"].(map[string]any)["modules.v1"])
}

// TestSortComponentStacks verifies deterministic stack-then-component ordering. Assert
// element contents (first and last), not just length, per the slice-result test rule.
func TestSortComponentStacks(t *testing.T) {
	targets := []ComponentStack{
		{Component: "vpc", Stack: "plat-ue2-prod"},
		{Component: "eks", Stack: "plat-ue2-prod"},
		{Component: "vpc", Stack: "core-ue1-dev"},
	}

	sortComponentStacks(targets)

	require.Len(t, targets, 3)
	assert.Equal(t, ComponentStack{Component: "vpc", Stack: "core-ue1-dev"}, targets[0])
	assert.Equal(t, ComponentStack{Component: "eks", Stack: "plat-ue2-prod"}, targets[1])
	assert.Equal(t, ComponentStack{Component: "vpc", Stack: "plat-ue2-prod"}, targets[2])
}
