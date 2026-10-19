package lifecycle

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

func inventory(nodes ...[]string) map[string]any {
	result := map[string]any{}
	for _, n := range nodes {
		if result[n[1]] == nil {
			result[n[1]] = map[string]any{"components": map[string]any{}}
		}
		kinds := result[n[1]].(map[string]any)["components"].(map[string]any)
		if kinds[n[0]] == nil {
			kinds[n[0]] = map[string]any{}
		}
		kinds[n[0]].(map[string]any)[n[2]] = map[string]any{}
	}
	return result
}

func section(stacks map[string]any, kind, stack, name string) map[string]any {
	return stacks[stack].(map[string]any)["components"].(map[string]any)[kind].(map[string]any)[name].(map[string]any)
}

func depends(stacks map[string]any, from, to []string, required bool) {
	s := section(stacks, from[0], from[1], from[2])
	s["dependencies"] = map[string]any{"components": []any{map[string]any{"kind": to[0], "stack": to[1], "component": to[2], "required": required}}}
}

func capabilities(kinds ...string) map[string]*Handler {
	c := map[string]*Handler{}
	for _, k := range kinds {
		c[k] = NativeHandler(k, "deploy")
	}
	return c
}

func affected(nodes ...[]string) []schema.Affected {
	r := []schema.Affected{}
	for _, n := range nodes {
		r = append(r, schema.Affected{ComponentType: n[0], Stack: n[1], Component: n[2]})
	}
	return r
}

func TestMixedGraphSelectionAndOrdering(t *testing.T) {
	tf := []string{"terraform", "shared", "service"}
	helm := []string{"helm", "dev", "service"}
	kube := []string{"kubernetes", "dev", "service"}
	stacks := inventory(tf, helm, kube)
	depends(stacks, helm, tf, true)
	depends(stacks, kube, helm, true)
	commands := capabilities("terraform", "helm", "kubernetes")
	cases := []struct {
		name      string
		selection Selection
		want      []string
	}{
		{"all", Selection{}, []string{"terraform/shared/service", "helm/dev/service", "kubernetes/dev/service"}},
		{"stack intersection", Selection{Stack: "dev"}, []string{"helm/dev/service", "kubernetes/dev/service"}},
		{"unchanged prerequisites omitted", Selection{Affected: affected(kube)}, []string{"kubernetes/dev/service"}},
		{"bounded cross stack expansion", Selection{Stack: "dev", Affected: affected(helm), Dependencies: 1}, []string{"terraform/shared/service", "helm/dev/service"}},
		{"depth one", Selection{Affected: affected(kube), Dependencies: 1}, []string{"helm/dev/service", "kubernetes/dev/service"}},
		{"unlimited", Selection{Affected: affected(kube), Dependencies: -1}, []string{"terraform/shared/service", "helm/dev/service", "kubernetes/dev/service"}},
		{"dependents", Selection{Stack: "shared", Affected: affected(tf), Dependents: -1}, []string{"terraform/shared/service", "helm/dev/service", "kubernetes/dev/service"}},
		{"intermediate contraction", Selection{Affected: affected(tf, kube)}, []string{"terraform/shared/service", "kubernetes/dev/service"}},
		{"destroy reversed", Selection{Reverse: true}, []string{"kubernetes/dev/service", "helm/dev/service", "terraform/shared/service"}},
		{"empty affected", Selection{Affected: affected()}, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			plan, err := BuildPlan(stacks, commands, tc.selection, "{{")
			require.NoError(t, err)
			order, err := plan.Graph.TopologicalSort()
			require.NoError(t, err)
			var labels []string
			for i := range order {
				labels = append(labels, Label(&order[i]))
			}
			assert.Equal(t, tc.want, labels)
		})
	}
}

func TestGraphPreflightAndExclusions(t *testing.T) {
	a := []string{"helm", "dev", "app"}
	b := []string{"packer", "shared", "image"}
	bad := []string{"terraform", "unused", "bad"}
	stacks := inventory(a, b, bad)
	commands := capabilities("helm", "terraform")
	depends(stacks, a, b, true)
	depends(stacks, bad, []string{"terraform", "unused", "missing"}, true)
	// A required prerequisite that cannot run the operation fails the plan with or without expansion.
	_, err := BuildPlan(stacks, commands, Selection{Affected: affected(a)}, "{{")
	require.ErrorIs(t, err, errUtils.ErrUnsupportedComponentType)
	require.ErrorContains(t, err, "helm/dev/app requires operation on packer/shared/image")
	_, err = BuildPlan(stacks, commands, Selection{Affected: affected(a), Dependencies: -1}, "{{")
	require.ErrorContains(t, err, "requires operation")
	depends(stacks, a, b, false)
	plan, err := BuildPlan(stacks, commands, Selection{Affected: affected(a)}, "{{")
	require.NoError(t, err)
	assert.Equal(t, 1, plan.Graph.Size())
	plan, err = BuildPlan(stacks, commands, Selection{Affected: affected(a), Dependencies: -1}, "{{")
	require.NoError(t, err)
	require.Len(t, plan.Notices, 1)
	_, err = BuildPlan(stacks, commands, Selection{Affected: affected(bad)}, "{{")
	require.ErrorContains(t, err, "requires")
	depends(stacks, a, a, true)
	_, err = BuildPlan(stacks, commands, Selection{Affected: affected(a)}, "{{")
	require.Error(t, err)
	section(stacks, a[0], a[1], a[2])["dependencies"] = map[string]any{"components": []any{}}
	section(stacks, a[0], a[1], a[2])["metadata"] = map[string]any{"locked": true}
	plan, err = BuildPlan(stacks, commands, Selection{Affected: affected(a)}, "{{")
	require.NoError(t, err)
	assert.Zero(t, plan.Graph.Size())
	require.Len(t, plan.Notices, 1)
	assert.Equal(t, "locked; excluded", plan.Notices[0].Reason)
	section(stacks, a[0], a[1], a[2])["metadata"] = map[string]any{"enabled": false}
	plan, err = BuildPlan(stacks, commands, Selection{Affected: affected(a)}, "{{")
	require.NoError(t, err)
	assert.Zero(t, plan.Graph.Size())
	deleted := affected(a)
	deleted[0].Deleted = true
	plan, err = BuildPlan(stacks, commands, Selection{Affected: deleted}, "{{")
	require.NoError(t, err)
	require.Len(t, plan.Notices, 1)
	assert.Contains(t, plan.Notices[0].Reason, "deleted")
}

func TestRelevantCycleThroughUnselectedNodes(t *testing.T) {
	a := []string{"terraform", "dev", "a"}
	b := []string{"helm", "dev", "b"}
	c := []string{"kubernetes", "dev", "c"}
	stacks := inventory(a, b, c)
	depends(stacks, a, b, true)
	depends(stacks, b, a, true)
	commands := capabilities("terraform", "helm", "kubernetes")
	_, err := BuildPlan(stacks, commands, Selection{Affected: affected(c)}, "{{")
	require.NoError(t, err)
	_, err = BuildPlan(stacks, commands, Selection{Affected: affected(a)}, "{{")
	require.Error(t, err)
	assert.NotEqual(t, NodeID("a", "b|c", "d"), NodeID("a|b", "c", "d"))
}

// setDependencies replaces a component's dependency declaration with raw entries.
func setDependencies(stacks map[string]any, node []string, entries ...any) {
	section(stacks, node[0], node[1], node[2])["dependencies"] = map[string]any{"components": entries}
}

func TestDependencyDefaultsToOwnKindAndStack(t *testing.T) {
	network := []string{"terraform", "dev", "network"}
	app := []string{"terraform", "dev", "app"}
	other := []string{"terraform", "prod", "network"}
	stacks := inventory(network, app, other)
	// Only the component is named: the kind and stack come from the declaring component.
	setDependencies(stacks, app, map[string]any{"component": "network"})

	plan, err := BuildPlan(stacks, capabilities("terraform"), Selection{Stack: "dev"}, "{{")
	require.NoError(t, err)
	assert.Equal(t, []string{"terraform/dev/network", "terraform/dev/app"}, planLabels(t, plan))
	assert.Equal(t, []string{NodeID("terraform", "dev", "network")}, plan.Graph.Nodes[NodeID("terraform", "dev", "app")].Dependencies)
}

func TestUnresolvedDependencySelectorFailsThePlan(t *testing.T) {
	app := []string{"terraform", "dev", "app"}
	stacks := inventory(app)
	setDependencies(stacks, app, map[string]any{"component": "{{ .vars.target }}"})

	_, err := BuildPlan(stacks, capabilities("terraform"), Selection{}, "{{")
	require.ErrorIs(t, err, errUtils.ErrDependencyResolution)
	assert.Contains(t, err.Error(), "terraform/dev/app")
}

func TestMalformedDependenciesSectionFailsThePlan(t *testing.T) {
	app := []string{"terraform", "dev", "app"}
	stacks := inventory(app)
	section(stacks, app[0], app[1], app[2])["dependencies"] = "not-a-map"

	_, err := BuildPlan(stacks, capabilities("terraform"), Selection{}, "{{")
	require.ErrorIs(t, err, errUtils.ErrDependencyResolution)
	assert.Contains(t, err.Error(), "terraform/dev/app")
}

func TestOptionalMissingTargetIsIgnored(t *testing.T) {
	app := []string{"terraform", "dev", "app"}
	stacks := inventory(app)
	depends(stacks, app, []string{"terraform", "dev", "missing"}, false)

	plan, err := BuildPlan(stacks, capabilities("terraform"), Selection{}, "{{")
	require.NoError(t, err)
	assert.Equal(t, []string{"terraform/dev/app"}, planLabels(t, plan))
	assert.Empty(t, plan.Graph.Nodes[NodeID("terraform", "dev", "app")].Dependencies)

	// Negative path: the same missing target is fatal when required.
	depends(stacks, app, []string{"terraform", "dev", "missing"}, true)
	_, err = BuildPlan(stacks, capabilities("terraform"), Selection{}, "{{")
	require.ErrorIs(t, err, errUtils.ErrDependencyTargetNotFound)
}

func TestHandlerErrorFailsOnlyPlansThatSelectItsKind(t *testing.T) {
	app := []string{"terraform", "dev", "app"}
	widget := []string{"widget", "dev", "w"}
	stacks := inventory(app, widget)
	broken := errors.New("broken declaration")
	commands := capabilities("terraform")
	commands["widget"] = &Handler{Path: []string{"widget", "deploy"}, Err: broken}

	_, err := BuildPlan(stacks, commands, Selection{}, "{{")
	require.ErrorIs(t, err, broken)

	// An unrelated kind never trips over the broken handler.
	plan, err := BuildPlan(stacks, commands, Selection{Affected: affected(app)}, "{{")
	require.NoError(t, err)
	assert.Equal(t, []string{"terraform/dev/app"}, planLabels(t, plan))
}

func TestAffectedOutsideSelectedStackIsNotEligible(t *testing.T) {
	devApp := []string{"terraform", "dev", "app"}
	prodApp := []string{"terraform", "prod", "app"}
	stacks := inventory(devApp, prodApp)

	plan, err := BuildPlan(stacks, capabilities("terraform"), Selection{Stack: "dev", Affected: affected(prodApp)}, "{{")
	require.NoError(t, err)
	assert.Zero(t, plan.Graph.Size())
	assert.Empty(t, plan.Notices)
}

func TestExpansionVisitsSharedPrerequisiteOnce(t *testing.T) {
	app := []string{"terraform", "dev", "app"}
	left := []string{"terraform", "dev", "left"}
	right := []string{"terraform", "dev", "right"}
	base := []string{"terraform", "dev", "base"}
	stacks := inventory(app, left, right, base)
	setDependencies(stacks, app, map[string]any{"component": "left"}, map[string]any{"component": "right"})
	depends(stacks, left, base, true)
	depends(stacks, right, base, true)

	plan, err := BuildPlan(stacks, capabilities("terraform"), Selection{Affected: affected(app), Dependencies: -1}, "{{")
	require.NoError(t, err)
	labels := planLabels(t, plan)
	assert.Len(t, labels, 4)
	assert.Equal(t, "terraform/dev/base", labels[0])
	assert.Equal(t, "terraform/dev/app", labels[3])
}

func TestDependentExpansionTraversesUnsupportedKinds(t *testing.T) {
	network := []string{"terraform", "dev", "network"}
	image := []string{"packer", "dev", "image"}
	app := []string{"terraform", "dev", "app"}
	stacks := inventory(network, image, app)
	depends(stacks, image, network, false)
	depends(stacks, app, image, false)

	plan, err := BuildPlan(stacks, capabilities("terraform"), Selection{Affected: affected(network), Dependents: -1}, "{{")
	require.NoError(t, err)
	// The unsupported consumer is reported and skipped, yet its own consumer is still reached.
	assert.ElementsMatch(t, []string{"terraform/dev/network", "terraform/dev/app"}, planLabels(t, plan))
	assert.Contains(t, plan.Notices, Notice{"packer", "dev", "image", "expanded component does not define this operation"})
}

// selectorMetadata sets a component's metadata tags and labels.
func selectorMetadata(stacks map[string]any, node []string, componentTags []any, labels map[string]any) {
	section(stacks, node[0], node[1], node[2])["metadata"] = map[string]any{"tags": componentTags, "labels": labels}
}

// plannedLabels returns the selected components in execution order.
func plannedLabels(t *testing.T, plan *Plan) []string {
	t.Helper()
	order, err := plan.Graph.TopologicalSort()
	require.NoError(t, err)
	labels := make([]string, 0, len(order))
	for i := range order {
		labels = append(labels, Label(&order[i]))
	}
	return labels
}

func TestTagAndLabelSelectors(t *testing.T) {
	network := []string{"terraform", "dev", "network"}
	database := []string{"terraform", "dev", "database"}
	cache := []string{"terraform", "dev", "cache"}
	app := []string{"terraform", "dev", "app"}
	stacks := inventory(network, database, cache, app)
	selectorMetadata(stacks, database, []any{"data", "stateful"}, map[string]any{"team": "platform", "tier": "1"})
	selectorMetadata(stacks, cache, []any{"data"}, map[string]any{"team": "platform", "tier": "2"})
	selectorMetadata(stacks, app, []any{"web"}, map[string]any{"team": "product"})
	depends(stacks, database, network, true)
	commands := capabilities("terraform")

	plan := func(s Selection) []string {
		p, err := BuildPlan(stacks, commands, s, "{{")
		require.NoError(t, err)
		return plannedLabels(t, p)
	}

	// Tags match any; labels must all match; both together must both match.
	assert.ElementsMatch(t, []string{"terraform/dev/database", "terraform/dev/cache"}, plan(Selection{Filter: &Filter{Tags: []string{"data"}}}))
	assert.ElementsMatch(t, []string{"terraform/dev/database", "terraform/dev/app"}, plan(Selection{Filter: &Filter{Tags: []string{"stateful", "web"}}}))
	assert.Equal(t, []string{"terraform/dev/database"}, plan(Selection{Filter: &Filter{Labels: map[string]string{"team": "platform", "tier": "1"}}}))
	assert.Equal(t, []string{"terraform/dev/cache"}, plan(Selection{Filter: &Filter{Tags: []string{"data"}, Labels: map[string]string{"tier": "2"}}}))
	assert.Empty(t, plan(Selection{Filter: &Filter{Tags: []string{"missing"}}}))

	// Filters narrow the initial selection only; dependency expansion still adds
	// the untagged prerequisite, ahead of the tagged component.
	assert.Equal(t, []string{"terraform/dev/network", "terraform/dev/database"}, plan(Selection{Filter: &Filter{Tags: []string{"stateful"}}, Dependencies: -1}))

	// Combined with an affected selection, the filter narrows the affected set.
	assert.Equal(t, []string{"terraform/dev/cache"}, plan(Selection{Filter: &Filter{Tags: []string{"data"}}, Affected: affected(cache, app)}))
}
