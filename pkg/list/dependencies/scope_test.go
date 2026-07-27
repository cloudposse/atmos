package dependencies

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// describeCall records one DescribeFunc invocation.
type describeCall struct {
	stack            string
	processTemplates bool
	processFunctions bool
}

// fakeDescribe serves per-stack views of a full stacks map and records calls,
// standing in for ExecuteDescribeStacks in scoped-evaluation tests.
type fakeDescribe struct {
	full  map[string]any
	calls []describeCall
	err   error
}

func (f *fakeDescribe) describe(stack string, processTemplates, processFunctions bool) (map[string]any, error) {
	f.calls = append(f.calls, describeCall{stack: stack, processTemplates: processTemplates, processFunctions: processFunctions})
	if f.err != nil {
		return nil, f.err
	}
	if stack == "" {
		return f.full, nil
	}
	if section, ok := f.full[stack]; ok {
		return map[string]any{stack: section}, nil
	}
	return map[string]any{}, nil
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
