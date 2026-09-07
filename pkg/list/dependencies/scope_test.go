package dependencies

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// errFakeDescribeStack is returned by fakeDescribe when errOnStack matches.
var errFakeDescribeStack = errors.New("fake describe: per-stack pass blew up")

// describeCall records one DescribeFunc invocation.
type describeCall struct {
	stack            string
	components       []string
	processTemplates bool
	processFunctions bool
}

// fakeDescribe serves per-stack views of a full stacks map and records calls,
// standing in for ExecuteDescribeStacks in scoped-evaluation tests.
type fakeDescribe struct {
	full     map[string]any
	resolved map[string]any
	calls    []describeCall
	err      error
	// errOnStack, when non-empty, fails only the per-stack (Phase C) describe
	// call for that stack name, leaving the initial lightweight full-repo pass
	// (stack == "") to succeed — used to test error propagation from inside
	// resolveClosureStacks' evaluation loop rather than from the first call.
	errOnStack string
}

func (f *fakeDescribe) describe(stack string, components []string, processTemplates, processFunctions bool) (map[string]any, error) {
	f.calls = append(f.calls, describeCall{stack: stack, components: components, processTemplates: processTemplates, processFunctions: processFunctions})
	if f.err != nil {
		return nil, f.err
	}
	if f.errOnStack != "" && stack == f.errOnStack {
		return nil, errFakeDescribeStack
	}
	if stack == "" {
		return f.full, nil
	}
	source := f.full
	if (processTemplates || processFunctions) && f.resolved != nil {
		source = f.resolved
	}
	section, ok := source[stack]
	if !ok {
		return map[string]any{}, nil
	}
	if len(components) == 0 {
		return map[string]any{stack: section}, nil
	}
	// Honor the components narrowing the way the real describe pipeline does.
	allowed := make(map[string]struct{}, len(components))
	for _, name := range components {
		allowed[name] = struct{}{}
	}
	stackMap, _ := section.(map[string]any)
	comps, _ := stackMap["components"].(map[string]any)
	terraform, _ := comps["terraform"].(map[string]any)
	narrowed := make(map[string]any, len(terraform))
	for name, comp := range terraform {
		if _, ok := allowed[name]; ok {
			narrowed[name] = comp
		}
	}
	return map[string]any{stack: map[string]any{"components": map[string]any{"terraform": narrowed}}}, nil
}

// evaluatedStacks returns the stacks described with evaluation on.
func (f *fakeDescribe) evaluatedStacks() []string {
	var stacks []string
	for _, call := range f.calls {
		if call.processTemplates || call.processFunctions {
			stacks = append(stacks, call.stack)
		}
	}
	return stacks
}

// scopedTestStacks builds dev(app -> db -> core/vpc) plus an unrelated stack:
// app and db live in dev, vpc lives in core, and "unrelated" holds a poison
// component that must never be evaluated by a bounded closure request.
func scopedTestStacks() map[string]any {
	return terraformStacks(map[string]map[string]map[string]any{
		"dev": {
			"app": dependsOn(map[string]any{"component": "db"}),
			"db":  dependsOn(map[string]any{"component": "vpc", "stack": "core"}),
		},
		"core": {
			"vpc": {},
		},
		"unrelated": {
			"poison": {},
		},
	})
}

func TestResolveScopedClosureEvaluatesOnlyClosureStacks(t *testing.T) {
	t.Parallel()

	fake := &fakeDescribe{full: scopedTestStacks()}
	result, err := ResolveScopedClosure(fake.describe, &ScopeRequest{
		Stack:            "dev",
		Direction:        DirectionForward,
		ProcessTemplates: true,
		ProcessFunctions: true,
	})
	require.NoError(t, err)

	// The closure spans both stacks the dev components reach.
	assert.ElementsMatch(t, []string{"core", "dev"}, StackNames(result.Closure))
	for _, id := range []string{NodeID("app", "dev"), NodeID("db", "dev"), NodeID("vpc", "core")} {
		_, ok := result.Closure.GetNode(id)
		assert.True(t, ok, "expected closure node %s", id)
	}

	// The merged stacks map holds exactly the closure stacks, fully evaluated.
	assert.ElementsMatch(t, []string{"core", "dev"}, mapKeys(result.Stacks))

	// The unrelated (poison) stack was never evaluated: the only full-repo call
	// is the lightweight structural pass with evaluation off.
	assert.ElementsMatch(t, []string{"core", "dev"}, fake.evaluatedStacks())
	require.NotEmpty(t, fake.calls)
	assert.Equal(t, describeCall{stack: ""}, fake.calls[0], "first call must be the lightweight full-repo pass")
}

// TestResolveScopedClosureEvaluatesOnlyClosureComponents is the regression
// guard for stack-granularity over-evaluation: a component that merely shares
// a stack file with closure members must not be evaluated (its templates/YAML
// functions may require auth to an unrelated account).
func TestResolveScopedClosureEvaluatesOnlyClosureComponents(t *testing.T) {
	t.Parallel()

	stacks := terraformStacks(map[string]map[string]map[string]any{
		"dev": {
			"app":    dependsOn(map[string]any{"component": "db"}),
			"db":     {},
			"poison": {},
		},
	})
	fake := &fakeDescribe{full: stacks}
	result, err := ResolveScopedClosure(fake.describe, &ScopeRequest{
		Components:       []string{"app"},
		Direction:        DirectionForward,
		ProcessTemplates: true,
	})
	require.NoError(t, err)

	for _, call := range fake.calls {
		if !call.processTemplates && !call.processFunctions {
			continue
		}
		assert.Equal(t, []string{"app", "db"}, call.components,
			"evaluated describe passes must be narrowed to the closure's own components")
		assert.NotContains(t, call.components, "poison")
	}
	_, hasPoison := result.Closure.GetNode(NodeID("poison", "dev"))
	assert.False(t, hasPoison)
	// The merged stacks map must not carry the poison component either.
	devComponents := result.Stacks["dev"].(map[string]any)["components"].(map[string]any)["terraform"].(map[string]any)
	assert.NotContains(t, devComponents, "poison")
}

func TestResolveScopedClosureDepthBoundsEvaluation(t *testing.T) {
	t.Parallel()

	fake := &fakeDescribe{full: scopedTestStacks()}
	result, err := ResolveScopedClosure(fake.describe, &ScopeRequest{
		Components:       []string{"app"},
		Stack:            "dev",
		Direction:        DirectionForward,
		Depths:           Depths{Dependencies: 1},
		ProcessTemplates: true,
	})
	require.NoError(t, err)

	// app -> db is one level; vpc (level 2, in core) stays out.
	assert.ElementsMatch(t, []string{"dev"}, StackNames(result.Closure))
	assert.ElementsMatch(t, []string{"dev"}, fake.evaluatedStacks())
	_, hasVpc := result.Closure.GetNode(NodeID("vpc", "core"))
	assert.False(t, hasVpc)
}

func TestResolveScopedClosureExpandsResolvedCrossStackDependency(t *testing.T) {
	t.Parallel()

	lightweight := terraformStacks(map[string]map[string]map[string]any{
		"application": {
			"root": dependsOn(map[string]any{"component": "network", "stack": "{{ .vars.network_stack }}"}),
		},
		"network": {
			"network": {},
		},
	})
	resolved := terraformStacks(map[string]map[string]map[string]any{
		"application": {
			"root": dependsOn(map[string]any{"component": "network", "stack": "network"}),
		},
		"network": {
			"network": {},
		},
	})
	describe := &fakeDescribe{full: lightweight, resolved: resolved}

	result, err := ResolveScopedClosure(describe.describe, &ScopeRequest{
		Components:       []string{"root"},
		Stack:            "application",
		Direction:        DirectionForward,
		Depths:           Depths{Dependencies: 1},
		ProcessTemplates: true,
	})

	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"application", "network"}, StackNames(result.Closure))
	assert.ElementsMatch(t, []string{"application", "network"}, mapKeys(result.Stacks))
	assert.ElementsMatch(t, []string{"application", "network"}, describe.evaluatedStacks())
}

func TestResolveScopedClosureEvaluatesResolvedSameStackDependency(t *testing.T) {
	t.Parallel()

	lightweight := terraformStacks(map[string]map[string]map[string]any{
		"dev": {
			"app":      dependsOn(map[string]any{"component": "{{ .vars.database_component }}"}),
			"database": {},
		},
	})
	resolved := terraformStacks(map[string]map[string]map[string]any{
		"dev": {
			"app":      dependsOn(map[string]any{"component": "database"}),
			"database": {},
		},
	})
	describe := &fakeDescribe{full: lightweight, resolved: resolved}

	result, err := ResolveScopedClosure(describe.describe, &ScopeRequest{
		Components:       []string{"app"},
		Stack:            "dev",
		Direction:        DirectionForward,
		Depths:           Depths{Dependencies: 1},
		ProcessTemplates: true,
	})

	require.NoError(t, err)
	_, ok := result.Closure.GetNode(NodeID("database", "dev"))
	require.True(t, ok, "resolved closure should include the same-stack dependency")
	dev := result.Stacks["dev"].(map[string]any)
	components := dev["components"].(map[string]any)["terraform"].(map[string]any)
	assert.Contains(t, components, "database", "resolved stacks must include every closure component")
}

func TestResolveScopedClosureReverseDirection(t *testing.T) {
	t.Parallel()

	fake := &fakeDescribe{full: scopedTestStacks()}
	result, err := ResolveScopedClosure(fake.describe, &ScopeRequest{
		Components:       []string{"vpc"},
		Stack:            "core",
		Direction:        DirectionReverse,
		ProcessTemplates: true,
	})
	require.NoError(t, err)

	// Everything that depends on core/vpc lives in dev.
	assert.ElementsMatch(t, []string{"core", "dev"}, StackNames(result.Closure))
	for _, id := range []string{NodeID("vpc", "core"), NodeID("db", "dev"), NodeID("app", "dev")} {
		_, ok := result.Closure.GetNode(id)
		assert.True(t, ok, "expected closure node %s", id)
	}
}

func TestResolveScopedClosureEmptyRoots(t *testing.T) {
	t.Parallel()

	fake := &fakeDescribe{full: scopedTestStacks()}
	result, err := ResolveScopedClosure(fake.describe, &ScopeRequest{
		Stack:            "nonexistent",
		Direction:        DirectionForward,
		ProcessTemplates: true,
	})
	require.NoError(t, err)

	assert.Equal(t, 0, result.Closure.Size())
	assert.Empty(t, result.Stacks)
	assert.Empty(t, fake.evaluatedStacks(), "no stack may be evaluated when nothing matches")
}

func TestResolveScopedClosurePropagatesDescribeErrors(t *testing.T) {
	t.Parallel()

	wantErr := errors.New("describe blew up")
	fake := &fakeDescribe{full: scopedTestStacks(), err: wantErr}
	_, err := ResolveScopedClosure(fake.describe, &ScopeRequest{Stack: "dev", Direction: DirectionForward, ProcessTemplates: true})
	require.ErrorIs(t, err, wantErr)
}

// TestResolveScopedClosureEmptyLightweightGraphShortCircuits verifies that
// when the lightweight structural pass produces a graph with zero nodes (no
// concrete terraform components anywhere), ResolveScopedClosure returns
// immediately with the raw lightweight stacks and an empty closure, without
// attempting seed selection or a Phase C evaluation pass.
func TestResolveScopedClosureEmptyLightweightGraphShortCircuits(t *testing.T) {
	t.Parallel()

	fake := &fakeDescribe{full: map[string]any{}}
	result, err := ResolveScopedClosure(fake.describe, &ScopeRequest{
		Stack:     "dev",
		Direction: DirectionForward,
	})
	require.NoError(t, err)

	assert.Equal(t, 0, result.Closure.Size())
	assert.Equal(t, fake.full, result.Stacks)
	// Only the lightweight pass ran; no Phase C evaluation was attempted.
	require.Len(t, fake.calls, 1)
	assert.Equal(t, describeCall{stack: ""}, fake.calls[0])
}

// TestResolveScopedClosurePropagatesPerStackDescribeErrors verifies an error
// from a Phase C (per-stack, inside resolveClosureStacks' evaluation loop)
// describe call propagates too, not just an error from the initial
// lightweight pass (already covered by
// TestResolveScopedClosurePropagatesDescribeErrors).
func TestResolveScopedClosurePropagatesPerStackDescribeErrors(t *testing.T) {
	t.Parallel()

	fake := &fakeDescribe{full: scopedTestStacks(), errOnStack: "dev"}
	_, err := ResolveScopedClosure(fake.describe, &ScopeRequest{
		Stack:            "dev",
		Direction:        DirectionForward,
		ProcessTemplates: true,
	})
	require.ErrorIs(t, err, errFakeDescribeStack)
}

// TestMergeResolvedClosureStacks covers the overlay merge directly, including
// the fallback branches taken when a resolved stack's shape does not match
// the lightweight pass's shape closely enough to merge component-by-component
// (malformed/unexpected data must not panic — the resolved value simply wins).
func TestMergeResolvedClosureStacks(t *testing.T) {
	t.Parallel()

	t.Run("resolved_stack_not_a_map_falls_back_to_resolved_value", func(t *testing.T) {
		t.Parallel()
		lightweight := map[string]any{"dev": map[string]any{"components": map[string]any{}}}
		resolved := map[string]any{"dev": "not-a-map"}
		merged := mergeResolvedClosureStacks(lightweight, resolved)
		assert.Equal(t, "not-a-map", merged["dev"])
	})

	t.Run("resolved_stack_missing_components_section_falls_back_to_resolved_value", func(t *testing.T) {
		t.Parallel()
		lightweight := map[string]any{"dev": map[string]any{"components": map[string]any{"terraform": map[string]any{}}}}
		resolved := map[string]any{"dev": map[string]any{"vars": map[string]any{}}} // no "components" key.
		merged := mergeResolvedClosureStacks(lightweight, resolved)
		assert.Equal(t, resolved["dev"], merged["dev"])
	})

	t.Run("resolved_component_type_not_a_map_falls_back_to_resolved_value_for_that_type", func(t *testing.T) {
		t.Parallel()
		lightweight := map[string]any{
			"dev": map[string]any{"components": map[string]any{"terraform": map[string]any{"app": map[string]any{}}}},
		}
		resolved := map[string]any{
			"dev": map[string]any{"components": map[string]any{"terraform": "not-a-map"}},
		}
		merged := mergeResolvedClosureStacks(lightweight, resolved)
		mergedDev := merged["dev"].(map[string]any)
		mergedComponents := mergedDev["components"].(map[string]any)
		assert.Equal(t, "not-a-map", mergedComponents["terraform"])
	})
}

// TestClosureComponentsByStackNilGraph covers the defensive nil-graph guard:
// a nil closure must yield an empty map, never a nil-pointer dereference.
func TestClosureComponentsByStackNilGraph(t *testing.T) {
	t.Parallel()
	assert.Equal(t, map[string][]string{}, closureComponentsByStack(nil))
}

func TestRootsMultiComponent(t *testing.T) {
	t.Parallel()

	graph, err := BuildGraph(scopedTestStacks())
	require.NoError(t, err)

	t.Run("multiple_components", func(t *testing.T) {
		t.Parallel()
		ids := Roots(graph, &Selector{Components: []string{"app", "vpc"}})
		assert.ElementsMatch(t, []string{NodeID("app", "dev"), NodeID("vpc", "core")}, ids)
	})

	t.Run("component_and_stack", func(t *testing.T) {
		t.Parallel()
		ids := Roots(graph, &Selector{Components: []string{"app", "db"}, Stack: "dev"})
		assert.ElementsMatch(t, []string{NodeID("app", "dev"), NodeID("db", "dev")}, ids)
	})

	t.Run("empty_filters_match_all", func(t *testing.T) {
		t.Parallel()
		ids := Roots(graph, &Selector{})
		assert.Len(t, ids, 4)
	})

	t.Run("nil_graph", func(t *testing.T) {
		t.Parallel()
		assert.Nil(t, Roots(nil, &Selector{}))
	})
}

// mapKeys returns the keys of a map[string]any.
func mapKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

// TestResolveScopedClosureReverseDiscoversTemplatedDependent is the regression
// test for the reverse-direction blind spot: a dependent whose dependency
// declaration is templated produces NO edge in the lightweight graph, so it is
// not in the initial closure and — without the conservative
// UnresolvedDependencySources evaluation — would never be evaluated and never
// discovered. Covers both a same-stack dependent (templated component) and a
// cross-stack dependent (templated stack target).
func TestResolveScopedClosureReverseDiscoversTemplatedDependent(t *testing.T) {
	t.Parallel()

	lightweight := terraformStacks(map[string]map[string]map[string]any{
		"dev": {
			"vpc": {},
			"app": dependsOn(map[string]any{"component": "{{ .vars.dep_component }}"}),
		},
		"ops": {
			"monitor": dependsOn(map[string]any{"component": "vpc", "stack": "{{ .vars.dep_stack }}"}),
		},
		"qa": {
			// Also has a templated dependency, but it resolves to a target
			// OUTSIDE the closure: it must be evaluated (conservatively) yet
			// excluded from the final closure.
			"batch": dependsOn(map[string]any{"component": "{{ .vars.other_component }}"}),
			"other": {},
		},
	})
	resolved := terraformStacks(map[string]map[string]map[string]any{
		"dev": {
			"vpc": {},
			"app": dependsOn(map[string]any{"component": "vpc"}),
		},
		"ops": {
			"monitor": dependsOn(map[string]any{"component": "vpc", "stack": "dev"}),
		},
		"qa": {
			"batch": dependsOn(map[string]any{"component": "other"}),
			"other": {},
		},
	})
	describe := &fakeDescribe{full: lightweight, resolved: resolved}

	result, err := ResolveScopedClosure(describe.describe, &ScopeRequest{
		Components:       []string{"vpc"},
		Stack:            "dev",
		Direction:        DirectionReverse,
		Depths:           Depths{Dependents: 1},
		ProcessTemplates: true,
	})

	require.NoError(t, err)
	_, ok := result.Closure.GetNode(NodeID("app", "dev"))
	require.True(t, ok, "reverse closure must discover a same-stack dependent with a templated component target")
	_, ok = result.Closure.GetNode(NodeID("monitor", "ops"))
	require.True(t, ok, "reverse closure must discover a cross-stack dependent with a templated stack target")

	// Extra evaluation is conservative, not membership: batch resolved to a
	// dependency outside the closure and must be excluded from it.
	_, ok = result.Closure.GetNode(NodeID("batch", "qa"))
	require.False(t, ok, "an evaluated non-dependent must not join the closure")
	_, ok = result.Closure.GetNode(NodeID("other", "qa"))
	require.False(t, ok, "the non-dependent's own dependency must not join the closure")
}

// TestResolveScopedClosureForwardSkipsUnresolvedSourceEvaluation is the
// negative path: the conservative extra evaluation is a reverse/both-direction
// recovery. A forward-only request converges through the closure itself (the
// declaring component is already a member), so unrelated components with
// templated dependencies must NOT be evaluated.
func TestResolveScopedClosureForwardSkipsUnresolvedSourceEvaluation(t *testing.T) {
	t.Parallel()

	lightweight := terraformStacks(map[string]map[string]map[string]any{
		"dev": {
			"vpc": {},
		},
		"ops": {
			"monitor": dependsOn(map[string]any{"component": "vpc", "stack": "{{ .vars.dep_stack }}"}),
		},
	})
	describe := &fakeDescribe{full: lightweight}

	result, err := ResolveScopedClosure(describe.describe, &ScopeRequest{
		Components:       []string{"vpc"},
		Stack:            "dev",
		Direction:        DirectionForward,
		Depths:           Depths{Dependencies: 1},
		ProcessTemplates: true,
	})

	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"dev"}, describe.evaluatedStacks(),
		"forward direction must not evaluate stacks that only hold unresolved-dependency sources")
	_, ok := result.Closure.GetNode(NodeID("monitor", "ops"))
	require.False(t, ok)
}

func TestUnresolvedDependencySources(t *testing.T) {
	t.Parallel()

	stacks := terraformStacks(map[string]map[string]map[string]any{
		"dev": {
			"vpc": {},
			"app": dependsOn(map[string]any{"component": "{{ .vars.dep }}"}),
			"db":  dependsOn(map[string]any{"component": "vpc"}),
		},
		"ops": {
			"monitor": dependsOn(map[string]any{"component": "vpc", "stack": "{{ .vars.stack }}"}),
			"scanner": dependsOn(map[string]any{"component": "!terraform.output meta dep"}),
		},
	})

	sources := UnresolvedDependencySources(stacks, "")

	assert.Equal(t, map[string][]string{
		"dev": {"app"},
		"ops": {"monitor", "scanner"},
	}, sources, "only components with templated or YAML-function dependency values are unresolved sources")
}

// TestResolveScopedClosureRefinesRootsAgainstResolvedSelectors is the
// preview/execution alignment regression test: a root whose templated selector
// conservatively matched in the lightweight pass but resolves to a
// NON-matching value must be dropped from the final closure — the terraform
// adapter re-derives its seed from resolved values, so keeping the root would
// make list previews a superset of the execution set.
func TestResolveScopedClosureRefinesRootsAgainstResolvedSelectors(t *testing.T) {
	t.Parallel()

	lightweight := terraformStacks(map[string]map[string]map[string]any{
		"dev": {
			"app": {
				"metadata": map[string]any{"tags": []any{"{{ .vars.tier }}"}},
				"settings": map[string]any{"depends_on": map[string]any{"1": map[string]any{"component": "vpc"}}},
			},
			"web": {
				"metadata": map[string]any{"tags": []any{"frontend"}},
				"settings": map[string]any{"depends_on": map[string]any{"1": map[string]any{"component": "vpc"}}},
			},
			"vpc": {},
		},
	})
	resolved := terraformStacks(map[string]map[string]map[string]any{
		"dev": {
			"app": {
				// The templated tag resolves OUTSIDE the selection.
				"metadata": map[string]any{"tags": []any{"backend"}},
				"settings": map[string]any{"depends_on": map[string]any{"1": map[string]any{"component": "vpc"}}},
			},
			"web": {
				"metadata": map[string]any{"tags": []any{"frontend"}},
				"settings": map[string]any{"depends_on": map[string]any{"1": map[string]any{"component": "vpc"}}},
			},
			"vpc": {},
		},
	})
	describe := &fakeDescribe{full: lightweight, resolved: resolved}

	result, err := ResolveScopedClosure(describe.describe, &ScopeRequest{
		Tags:             []string{"frontend"},
		Direction:        DirectionForward,
		Depths:           Depths{Dependencies: 1},
		ProcessTemplates: true,
	})

	require.NoError(t, err)
	_, ok := result.Closure.GetNode(NodeID("web", "dev"))
	require.True(t, ok, "the genuinely matching root must stay in the closure")
	_, ok = result.Closure.GetNode(NodeID("vpc", "dev"))
	require.True(t, ok, "the matching root's dependency must stay in the closure")
	_, ok = result.Closure.GetNode(NodeID("app", "dev"))
	require.False(t, ok,
		"a root whose templated selector resolved to a non-matching value must be dropped from the final closure")
}
