package lifecycle

import (
	"testing"

	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
	plan, err := BuildPlan(stacks, commands, Selection{Affected: affected(a)}, "{{")
	require.NoError(t, err)
	assert.Equal(t, 1, plan.Graph.Size())
	_, err = BuildPlan(stacks, commands, Selection{Affected: affected(a), Dependencies: -1}, "{{")
	require.ErrorContains(t, err, "requires operation")
	depends(stacks, a, b, false)
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
	_, err = BuildPlan(stacks, commands, Selection{Affected: affected(a)}, "{{")
	require.ErrorContains(t, err, "locked")
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
