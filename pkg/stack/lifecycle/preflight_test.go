package lifecycle

import (
	"errors"
	"fmt"
	"testing"

	cockroach "github.com/cockroachdb/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/dependency"
)

func planLabels(t *testing.T, plan *Plan) []string {
	t.Helper()
	order, err := plan.Graph.TopologicalSort()
	require.NoError(t, err)
	labels := make([]string, 0, len(order))
	for i := range order {
		labels = append(labels, Label(&order[i]))
	}
	return labels
}

func lock(stacks map[string]any, node []string) {
	section(stacks, node[0], node[1], node[2])["metadata"] = map[string]any{"locked": true}
}

func TestRequiredPrerequisiteOfUnsupportedKindFailsBeforeExecution(t *testing.T) {
	app := []string{"terraform", "dev", "app"}
	ingress := []string{"helmfile", "dev", "ingress"}
	stacks := inventory(app, ingress)
	depends(stacks, app, ingress, true)
	commands := capabilities("terraform")

	_, err := BuildPlan(stacks, commands, Selection{}, "{{")
	require.ErrorIs(t, err, errUtils.ErrUnsupportedComponentType)
	require.EqualError(t, err, "unsupported component type: terraform/dev/app requires operation on helmfile/dev/ingress")

	// Expansion reports the same error rather than a different outcome.
	_, err = BuildPlan(stacks, commands, Selection{Dependencies: -1}, "{{")
	require.EqualError(t, err, "unsupported component type: terraform/dev/app requires operation on helmfile/dev/ingress")

	// Optional prerequisites remain notices.
	depends(stacks, app, ingress, false)
	plan, err := BuildPlan(stacks, commands, Selection{}, "{{")
	require.NoError(t, err)
	assert.Equal(t, []string{"terraform/dev/app"}, planLabels(t, plan))
	require.Len(t, plan.Notices, 1)
	assert.Equal(t, "helmfile", plan.Notices[0].Kind)
}

func TestRequiredPrerequisiteOfSupportedKindOutsideSelectionIsAssumedDeployed(t *testing.T) {
	app := []string{"terraform", "prod", "app"}
	network := []string{"terraform", "dev", "network"}
	stacks := inventory(app, network)
	depends(stacks, app, network, true)
	plan, err := BuildPlan(stacks, capabilities("terraform"), Selection{Stack: "prod"}, "{{")
	require.NoError(t, err)
	assert.Equal(t, []string{"terraform/prod/app"}, planLabels(t, plan))
}

func TestRequiredPrerequisiteCheckDoesNotApplyToDestroy(t *testing.T) {
	app := []string{"terraform", "dev", "app"}
	ingress := []string{"helmfile", "dev", "ingress"}
	stacks := inventory(app, ingress)
	depends(stacks, app, ingress, true)
	plan, err := BuildPlan(stacks, capabilities("terraform"), Selection{Reverse: true}, "{{")
	require.NoError(t, err)
	assert.Equal(t, []string{"terraform/dev/app"}, planLabels(t, plan))
}

func TestLockedComponentsAreExcludedWithNotice(t *testing.T) {
	for _, reverse := range []bool{false, true} {
		a := []string{"terraform", "qa", "a"}
		b := []string{"terraform", "qa", "b"}
		stacks := inventory(a, b)
		lock(stacks, a)
		plan, err := BuildPlan(stacks, capabilities("terraform"), Selection{Reverse: reverse}, "{{")
		require.NoError(t, err, "reverse=%t", reverse)
		assert.Equal(t, []string{"terraform/qa/b"}, planLabels(t, plan))
		require.Equal(t, []Notice{{"terraform", "qa", "a", "locked; excluded"}}, plan.Notices)
	}
}

func TestLockedPrerequisiteIsExcludedDuringExpansion(t *testing.T) {
	app := []string{"terraform", "dev", "app"}
	network := []string{"terraform", "dev", "network"}
	stacks := inventory(app, network)
	depends(stacks, app, network, true)
	lock(stacks, network)
	plan, err := BuildPlan(stacks, capabilities("terraform"), Selection{Affected: affected(app), Dependencies: -1}, "{{")
	require.NoError(t, err)
	assert.Equal(t, []string{"terraform/dev/app"}, planLabels(t, plan))
	require.Equal(t, []Notice{{"terraform", "dev", "network", "locked; excluded"}}, plan.Notices)
}

func TestDestroyFailsWhenSurvivingConsumersDependOnSelection(t *testing.T) {
	network := []string{"terraform", "dev", "network"}
	app := []string{"terraform", "prod", "app"}
	stacks := inventory(network, app)
	depends(stacks, app, network, true)
	commands := capabilities("terraform")

	_, err := BuildPlan(stacks, commands, Selection{Stack: "dev", Reverse: true}, "{{")
	require.ErrorIs(t, err, errUtils.ErrSurvivingConsumers)
	assert.Contains(t, err.Error(), "terraform/prod/app -> terraform/dev/network")
	require.Len(t, cockroach.GetAllHints(err), 1)
	assert.Contains(t, cockroach.GetAllHints(err)[0], "--include-dependents")

	plan, err := BuildPlan(stacks, commands, Selection{Stack: "dev", Reverse: true, Dependents: -1}, "{{")
	require.NoError(t, err)
	assert.Equal(t, []string{"terraform/prod/app", "terraform/dev/network"}, planLabels(t, plan))

	// Deploy has no surviving-consumer rule.
	_, err = BuildPlan(stacks, commands, Selection{Stack: "dev"}, "{{")
	require.NoError(t, err)
}

func TestDestroySurvivingConsumerRules(t *testing.T) {
	network := []string{"terraform", "dev", "network"}
	app := []string{"terraform", "prod", "app"}
	stacks := inventory(network, app)
	depends(stacks, app, network, false)
	commands := capabilities("terraform")
	selection := Selection{Stack: "dev", Reverse: true}

	// Optional dependents do not block.
	_, err := BuildPlan(stacks, commands, selection, "{{")
	require.NoError(t, err)

	// Locked consumers survive and therefore block.
	depends(stacks, app, network, true)
	lock(stacks, app)
	_, err = BuildPlan(stacks, commands, selection, "{{")
	require.ErrorIs(t, err, errUtils.ErrSurvivingConsumers)
	_, err = BuildPlan(stacks, commands, Selection{Stack: "dev", Reverse: true, Dependents: -1}, "{{")
	require.ErrorIs(t, err, errUtils.ErrSurvivingConsumers)

	// Consumers of kinds that cannot be destroyed produce a notice instead.
	stacks = inventory(network, []string{"packer", "prod", "image"})
	depends(stacks, []string{"packer", "prod", "image"}, network, true)
	plan, err := BuildPlan(stacks, commands, selection, "{{")
	require.NoError(t, err)
	require.Len(t, plan.Notices, 1)
	assert.Equal(t, "packer", plan.Notices[0].Kind)
	assert.Contains(t, plan.Notices[0].Reason, "requires terraform/dev/network")

	// Selecting every stack seeds the Packer consumer first with the generic
	// unsupported-kind notice; the surviving-consumer reason must replace it.
	plan, err = BuildPlan(stacks, commands, Selection{Reverse: true}, "{{")
	require.NoError(t, err)
	require.Len(t, plan.Notices, 1)
	assert.Equal(t, "image", plan.Notices[0].Component)
	assert.Contains(t, plan.Notices[0].Reason, "requires terraform/dev/network")
}

func TestCycleErrorUsesComponentLabels(t *testing.T) {
	a := []string{"terraform", "qa", "a"}
	b := []string{"terraform", "qa", "b"}
	stacks := inventory(a, b)
	depends(stacks, a, b, true)
	depends(stacks, b, a, true)
	for _, reverse := range []bool{false, true} {
		_, err := BuildPlan(stacks, capabilities("terraform"), Selection{Reverse: reverse}, "{{")
		require.ErrorIs(t, err, dependency.ErrCircularDependency)
		assert.Contains(t, err.Error(), "terraform/qa/a -> terraform/qa/b -> terraform/qa/a")
		assert.NotContains(t, err.Error(), "9:terraform")
	}
}

func TestSelfDependencyUsesComponentLabel(t *testing.T) {
	a := []string{"terraform", "qa", "a"}
	stacks := inventory(a)
	depends(stacks, a, a, true)
	_, err := BuildPlan(stacks, capabilities("terraform"), Selection{}, "{{")
	require.ErrorIs(t, err, dependency.ErrSelfDependency)
	assert.Contains(t, err.Error(), "terraform/qa/a")
	assert.NotContains(t, err.Error(), "9:terraform")
}

func TestNoticesAreDeduplicatedAndOrdered(t *testing.T) {
	a := []string{"terraform", "dev", "a"}
	b := []string{"terraform", "dev", "b"}
	shared := []string{"packer", "dev", "image"}
	stacks := inventory(a, b, shared)
	depends(stacks, a, shared, false)
	depends(stacks, b, shared, false)
	plan, err := BuildPlan(stacks, capabilities("terraform"), Selection{Dependencies: -1}, "{{")
	require.NoError(t, err)
	// The unsupported kind is reported once even though it is reached by seeding and by two consumers.
	require.Equal(t, []Notice{{"packer", "dev", "image", "component kind does not define this operation"}}, plan.Notices)
}

func TestDeletedDefinitionNoticeNamesTheVerb(t *testing.T) {
	a := []string{"terraform", "dev", "a"}
	stacks := inventory(a)
	deleted := affected(a)
	deleted[0].Deleted = true

	plan, err := BuildPlan(stacks, capabilities("terraform"), Selection{Affected: deleted}, "{{")
	require.NoError(t, err)
	require.Len(t, plan.Notices, 1)
	assert.Contains(t, plan.Notices[0].Reason, "--allow-destroy")

	plan, err = BuildPlan(stacks, capabilities("terraform"), Selection{Affected: deleted, Reverse: true}, "{{")
	require.NoError(t, err)
	require.Len(t, plan.Notices, 1)
	assert.Contains(t, plan.Notices[0].Reason, "removed from configuration")
	assert.NotContains(t, plan.Notices[0].Reason, "--allow-destroy")
}

func TestNoticesAreSortedByComponentThenReason(t *testing.T) {
	app := []string{"terraform", "dev", "app"}
	bravo := []string{"packer", "dev", "bravo"}
	alpha := []string{"packer", "dev", "alpha"}
	stacks := inventory(app, bravo, alpha)

	plan, err := BuildPlan(stacks, capabilities("terraform"), Selection{}, "{{")
	require.NoError(t, err)
	reason := "component kind does not define this operation"
	// Map iteration order must not leak into the output.
	assert.Equal(t, []Notice{{"packer", "dev", "alpha", reason}, {"packer", "dev", "bravo", reason}}, plan.Notices)
}

func TestReadableCycleKeepsOriginalErrorWhenNoCycleIsLocated(t *testing.T) {
	g := dependency.NewGraph()
	require.NoError(t, g.AddNode(&dependency.Node{ID: "a", Type: "terraform", Stack: "dev", Component: "a"}))
	require.NoError(t, g.AddNode(&dependency.Node{ID: "b", Type: "terraform", Stack: "dev", Component: "b"}))
	require.NoError(t, g.AddDependency("a", "b"))

	// Negative path: an acyclic graph yields no cycle, so the sorter's error is returned unchanged.
	assert.Empty(t, findCycle(g))
	original := fmt.Errorf("%w: reported elsewhere", dependency.ErrCircularDependency)
	assert.Same(t, original, readableCycle(g, original))

	// Errors that are not cycles are never rewritten.
	other := errors.New("unrelated")
	assert.Same(t, other, readableCycle(g, other))
}

func TestFindCycleIgnoresDanglingDependencies(t *testing.T) {
	g := dependency.NewGraph()
	require.NoError(t, g.AddNode(&dependency.Node{ID: "a", Type: "terraform", Stack: "dev", Component: "a"}))
	require.NoError(t, g.AddNode(&dependency.Node{ID: "b", Type: "terraform", Stack: "dev", Component: "b"}))
	// A dependency edge whose target is not in the graph is skipped rather than followed.
	g.Nodes["a"].Dependencies = []string{"0ghost", "b"}
	g.Nodes["b"].Dependencies = []string{"a"}

	cycle := findCycle(g)
	assert.Equal(t, []string{"a", "b", "a"}, cycle)
}
