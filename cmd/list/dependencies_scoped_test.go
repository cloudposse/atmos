package list

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/tests"
)

// dependenciesScopedFixturePath is a two-stack fixture (app-a, app-b) with no
// dependency relationship between the stacks. The `broken` component in app-b
// unconditionally fails template rendering the moment it is evaluated at all —
// standing in for "an unrelated stack whose backend is unreachable" (the
// reported cross-account customer scenario) without requiring real cloud
// credentials in CI.
var dependenciesScopedFixturePath = filepath.Join("..", "..", "tests", "fixtures", "scenarios", "dependencies-scoped-evaluation")

func chdirToDependenciesScopedFixture(t *testing.T) {
	t.Helper()
	tests.RequireFilePath(t, dependenciesScopedFixturePath, "test fixture directory")
	t.Chdir(dependenciesScopedFixturePath)
}

// TestExecuteListDependenciesCmd_ScopedEvaluationAvoidsUnrelatedStack proves the
// core fix: `list dependencies --stack app-a` with full evaluation enabled
// completes successfully even though the unrelated app-b stack contains a
// component that always errors when evaluated. Before the scope-before-evaluate
// fix, describeStacksForDependencies always fully evaluated every stack
// (hardcoded filterByStack=""), so this exact scenario would fail with
// app-b's error before ever reaching app-a's dependency tree. See
// docs/fixes/2026-07-25-scope-before-evaluate-labels-tags-list-dependencies.md.
func TestExecuteListDependenciesCmd_ScopedEvaluationAvoidsUnrelatedStack(t *testing.T) {
	initExecutorTestIO(t)
	chdirToDependenciesScopedFixture(t)

	cmd := newCmdWithListParser("dependencies", dependenciesParser.RegisterFlags)
	require.NoError(t, cmd.Flags().Set("identity", "false"))
	opts := &DependenciesOptions{
		Format:           "json",
		Direction:        "both",
		Stack:            "app-a",
		Component:        "child",
		ProcessTemplates: true,
		ProcessFunctions: true,
	}

	err := executeListDependenciesCmd(cmd, []string{"child"}, opts)
	require.NoError(t, err,
		"bounded --stack app-a must never evaluate app-b's always-erroring component")
}

// Even a selected component's unused values must not be evaluated for a graph.
func TestExecuteListDependenciesCmd_ScopedEvaluationSkipsUnusedValue(t *testing.T) {
	initExecutorTestIO(t)
	chdirToDependenciesScopedFixture(t)

	cmd := newCmdWithListParser("dependencies", dependenciesParser.RegisterFlags)
	require.NoError(t, cmd.Flags().Set("identity", "false"))
	opts := &DependenciesOptions{
		Format:           "json",
		Direction:        "both",
		Stack:            "app-b",
		ProcessTemplates: true,
		ProcessFunctions: true,
	}

	err := executeListDependenciesCmd(cmd, []string{}, opts)
	require.NoError(t, err, "the graph does not consume the broken vars value")

	// A failure in a value required by evaluation must still propagate through
	// the scoped closure engine. This explicitly adds that value to its inputs.
	describe, err := newDependenciesDescribeContext(cmd, nil, opts)
	require.NoError(t, err)
	describe.atmosConfig.ListEvaluationPaths = append(describe.atmosConfig.ListEvaluationPaths, []string{"vars", "upstream_value"})
	_, err = buildScopedDependencyGraph(describe, opts)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "app-b must never be evaluated")
}

// An unbounded graph needs every stack's edges, not every stack's values.
func TestExecuteListDependenciesCmd_UnboundedSkipsUnusedValues(t *testing.T) {
	initExecutorTestIO(t)
	chdirToDependenciesScopedFixture(t)

	cmd := newCmdWithListParser("dependencies", dependenciesParser.RegisterFlags)
	require.NoError(t, cmd.Flags().Set("identity", "false"))
	opts := &DependenciesOptions{
		Format:           "json",
		Direction:        "both",
		ProcessTemplates: true,
		ProcessFunctions: true,
	}

	err := executeListDependenciesCmd(cmd, []string{}, opts)
	require.NoError(t, err, "unused values remain unevaluated even without a stack filter")
}
