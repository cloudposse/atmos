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
	opts := &DependenciesOptions{
		Format:           "json",
		Direction:        "both",
		Stack:            "app-a",
		Component:        "child",
		ProcessTemplates: true,
		ProcessFunctions: true,
		AuthDisabled:     true,
	}

	err := executeListDependenciesCmd(cmd, []string{"child"}, opts)
	require.NoError(t, err,
		"bounded --stack app-a must never evaluate app-b's always-erroring component")
}

// TestExecuteListDependenciesCmd_ScopedEvaluationPropagatesError proves that a
// bounded request whose own seed stack fails evaluation surfaces the error
// through buildScopedDependencyGraph rather than being swallowed: unlike
// TestExecuteListDependenciesCmd_ScopedEvaluationAvoidsUnrelatedStack (which
// bounds to the healthy app-a), this bounds directly to app-b, whose `broken`
// component always fails template rendering once evaluated.
func TestExecuteListDependenciesCmd_ScopedEvaluationPropagatesError(t *testing.T) {
	initExecutorTestIO(t)
	chdirToDependenciesScopedFixture(t)

	cmd := newCmdWithListParser("dependencies", dependenciesParser.RegisterFlags)
	opts := &DependenciesOptions{
		Format:           "json",
		Direction:        "both",
		Stack:            "app-b",
		ProcessTemplates: true,
		ProcessFunctions: true,
		AuthDisabled:     true,
	}

	err := executeListDependenciesCmd(cmd, []string{}, opts)
	require.Error(t, err, "a bounded request whose own seed stack fails evaluation must surface the error")
	assert.Contains(t, err.Error(), "app-b must never be evaluated")
}

// TestExecuteListDependenciesCmd_UnboundedStillEvaluatesEverything documents the
// known, unchanged boundary of the fix: with no --stack/--component filter,
// there is no closure to scope evaluation to, so every stack (including
// app-b's always-erroring component) is still evaluated — matching historical
// behavior exactly, not a regression introduced by this fix.
func TestExecuteListDependenciesCmd_UnboundedStillEvaluatesEverything(t *testing.T) {
	initExecutorTestIO(t)
	chdirToDependenciesScopedFixture(t)

	cmd := newCmdWithListParser("dependencies", dependenciesParser.RegisterFlags)
	opts := &DependenciesOptions{
		Format:           "json",
		Direction:        "both",
		ProcessTemplates: true,
		ProcessFunctions: true,
		AuthDisabled:     true,
	}

	err := executeListDependenciesCmd(cmd, []string{}, opts)
	require.Error(t, err, "an unbounded request has no closure to scope to, so app-b's error still surfaces")
	assert.Contains(t, err.Error(), "app-b must never be evaluated")
}
