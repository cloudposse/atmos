package lifecycle

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
)

func TestTestOperationDoesNotRequireUnselectedPrerequisitesToSupportTests(t *testing.T) {
	app := []string{"terraform", "dev", "app"}
	ingress := []string{"helmfile", "dev", "ingress"}
	stacks := inventory(app, ingress)
	depends(stacks, app, ingress, true)
	commands := map[string]*Handler{"terraform": NativeHandler("terraform", "test")}
	plan, err := BuildPlan(stacks, commands, &Selection{Operation: Test, Components: []string{"app"}}, "{{")
	require.NoError(t, err)
	assert.Equal(t, []string{"terraform/dev/app"}, planLabels(t, plan))
	assert.Equal(t, []string{"terraform", "test", "--stack=dev", "app"}, plan.Commands[NodeID("terraform", "dev", "app")])

	_, err = BuildPlan(stacks, commands, &Selection{Operation: Test, Components: []string{"app"}, Dependencies: 1}, "{{")
	require.ErrorIs(t, err, errUtils.ErrUnsupportedComponentType)
	assert.Contains(t, err.Error(), "terraform/dev/app requires operation on helmfile/dev/ingress")

	depends(stacks, app, ingress, false)
	plan, err = BuildPlan(stacks, commands, &Selection{Operation: Test, Components: []string{"app"}, Dependencies: 1}, "{{")
	require.NoError(t, err)
	assert.Equal(t, []string{"terraform/dev/app"}, planLabels(t, plan))
	assert.Equal(t, []Notice{{"helmfile", "dev", "ingress", "expanded component does not define this operation"}}, plan.Notices)
}

func TestTestOperationChecksOnlyExpandedDependencyDepth(t *testing.T) {
	app := []string{"terraform", "dev", "app"}
	api := []string{"terraform", "dev", "api"}
	ingress := []string{"helmfile", "dev", "ingress"}
	stacks := inventory(app, api, ingress)
	depends(stacks, app, api, true)
	depends(stacks, api, ingress, true)
	commands := map[string]*Handler{"terraform": NativeHandler("terraform", "test")}
	plan, err := BuildPlan(stacks, commands, &Selection{Operation: Test, Components: []string{"app"}, Dependencies: 1}, "{{")
	require.NoError(t, err)
	assert.Equal(t, []string{"terraform/dev/api", "terraform/dev/app"}, planLabels(t, plan))
	for _, depth := range []int{2, -1} {
		_, err = BuildPlan(stacks, commands, &Selection{Operation: Test, Components: []string{"app"}, Dependencies: depth}, "{{")
		require.ErrorIs(t, err, errUtils.ErrUnsupportedComponentType)
	}
}

func TestTestOperationReportsDeletedDefinitionsWithoutDestroyInstructions(t *testing.T) {
	removed := affected([]string{"terraform", "dev", "gone"})
	removed[0].Deleted = true
	plan, err := BuildPlan(nil, capabilities("terraform"), &Selection{Operation: Test, Affected: removed}, "{{")
	require.NoError(t, err)
	assert.Zero(t, plan.Graph.Size())
	assert.Equal(t, 1, plan.Deleted)
	require.Len(t, plan.Notices, 1)
	assert.Equal(t, Notice{"terraform", "dev", "gone", "removed from configuration; `atmos stack test` only operates on components that are still defined"}, plan.Notices[0])
	assert.NotContains(t, plan.Notices[0].Reason, "--allow-destroy")
}
