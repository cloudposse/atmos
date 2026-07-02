package composition

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	cfg "github.com/cloudposse/atmos/pkg/config"
)

// stackWithComponents builds a describe-stacks stack entry whose container
// components each carry the given composition claim (empty = no claim).
func stackWithComponents(claims map[string]string) map[string]any {
	typeMap := map[string]any{}
	for component, composition := range claims {
		comp := map[string]any{}
		if composition != "" {
			comp["composition"] = composition
		}
		typeMap[component] = comp
	}
	return map[string]any{
		cfg.ComponentsSectionName: map[string]any{
			cfg.ContainerComponentType: typeMap,
		},
	}
}

func TestClaimsComposition(t *testing.T) {
	cases := []struct {
		name     string
		compData any
		want     bool
	}{
		{"matching claim", map[string]any{"composition": "storefront"}, true},
		{"different composition", map[string]any{"composition": "other"}, false},
		{"no composition field", map[string]any{"image": "alpine"}, false},
		{"non-string composition", map[string]any{"composition": 42}, false},
		{"not a map", "storefront", false},
		{"nil", nil, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, claimsComposition(tc.compData, "storefront"))
		})
	}
}

func TestCollectStackMembers(t *testing.T) {
	t.Run("collects matching members", func(t *testing.T) {
		seen := map[string]bool{}
		collectStackMembers(stackWithComponents(map[string]string{
			"api":       "storefront",
			"worker":    "storefront",
			"unrelated": "other",
		}), "storefront", seen)
		assert.True(t, seen["api"])
		assert.True(t, seen["worker"])
		assert.False(t, seen["unrelated"])
	})

	t.Run("type-assertion safety", func(t *testing.T) {
		seen := map[string]bool{}
		// None of these malformed inputs should panic or record members.
		collectStackMembers("not a map", "storefront", seen)
		collectStackMembers(map[string]any{}, "storefront", seen)                                                              // missing components section
		collectStackMembers(map[string]any{cfg.ComponentsSectionName: "bad"}, "storefront", seen)                              // components not a map
		collectStackMembers(map[string]any{cfg.ComponentsSectionName: map[string]any{"container": "bad"}}, "storefront", seen) // type section not a map
		assert.Empty(t, seen)
	})
}

func TestCollectMembers_DedupesAndSorts(t *testing.T) {
	stacksMap := map[string]any{
		"dev": stackWithComponents(map[string]string{"worker": "storefront", "api": "storefront"}),
		// "api" appears in two stacks and must be de-duplicated.
		"staging": stackWithComponents(map[string]string{"api": "storefront", "database": "other"}),
		"prod":    "not-a-map", // ignored, no panic
	}
	members := collectMembers(stacksMap, "storefront")
	assert.Equal(t, []string{"api", "worker"}, members) // sorted + deduped
}

func TestReportForStacks(t *testing.T) {
	stacksMap := map[string]any{
		"dev": stackWithComponents(map[string]string{"api": "storefront", "worker": "storefront"}),
	}
	report, err := reportForStacks(stacksMap, "storefront", compositions())
	require.NoError(t, err)
	assert.Equal(t, "storefront", report.Composition)
	assert.Equal(t, []string{"api", "worker"}, report.Fulfilled)
	assert.Equal(t, []string{"database"}, report.NotProvided)
	assert.Empty(t, report.Unknown)
}

func TestReportForStacks_UnknownComposition(t *testing.T) {
	_, err := reportForStacks(map[string]any{}, "missing", compositions())
	require.Error(t, err)
}

// TestRenderReport exercises every output branch. UI output is a no-op when the
// formatter is uninitialized, so this asserts the branch logic does not panic on
// empty/populated slices rather than the formatted strings.
func TestRenderReport(t *testing.T) {
	reports := []Report{
		{Composition: "storefront", Description: "desc", Fulfilled: []string{"api"}, NotProvided: []string{"database"}, Unknown: []string{"frontend"}},
		{Composition: "empty"}, // no services declared branch
		{Composition: "only-fulfilled", Fulfilled: []string{"api"}},
	}
	for _, r := range reports {
		r := r
		assert.NotPanics(t, func() { renderReport(&r) })
	}
}
