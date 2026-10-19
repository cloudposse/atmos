package stack

import (
	"path/filepath"
	"slices"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/cmd/internal"
	errUtils "github.com/cloudposse/atmos/errors"
	e "github.com/cloudposse/atmos/internal/exec"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/stack/lifecycle"
)

// lifecycleFixturePlan builds a plan for the stack-lifecycle-field-test fixture with
// the real built-in handlers and the fixture's custom script commands.
func lifecycleFixturePlan(t *testing.T, verb string, selection lifecycle.Selection) (*lifecycle.Plan, error) {
	t.Helper()
	fixture, err := filepath.Abs(filepath.Join("..", "..", "tests", "fixtures", "scenarios", "stack-lifecycle-field-test"))
	require.NoError(t, err)
	require.DirExists(t, fixture)
	t.Chdir(fixture)
	t.Setenv("ATMOS_CLI_CONFIG_PATH", fixture)
	t.Setenv("ATMOS_BASE_PATH", fixture)
	e.ClearFindStacksMapCache()
	t.Cleanup(e.ClearFindStacksMapCache)

	config, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, true)
	require.NoError(t, err)

	// Custom commands are attached by the root command at startup; mirror that here.
	root := &cobra.Command{Use: "atmos"}
	require.NoError(t, internal.RegisterAll(root))
	script := &cobra.Command{Use: "script"}
	for _, name := range []string{"deploy", "destroy"} {
		script.AddCommand(&cobra.Command{Use: name, Annotations: map[string]string{"customCommand": "true"}})
	}
	root.AddCommand(script)

	stacks, handlers, err := loadLifecycleInventory(script, &config, verb, nil)
	require.NoError(t, err)
	return lifecycle.BuildPlan(stacks, handlers, selection, "{{")
}

func planLabels(t *testing.T, plan *lifecycle.Plan) []string {
	t.Helper()
	order, err := plan.Graph.TopologicalSort()
	require.NoError(t, err)
	labels := make([]string, 0, len(order))
	for i := range order {
		labels = append(labels, lifecycle.Label(&order[i]))
	}
	return labels
}

func assertBefore(t *testing.T, labels []string, first, second string) {
	t.Helper()
	i, j := slices.Index(labels, first), slices.Index(labels, second)
	require.NotEqual(t, -1, i, "%s missing from %v", first, labels)
	require.NotEqual(t, -1, j, "%s missing from %v", second, labels)
	assert.Less(t, i, j, "%s must run before %s in %v", first, second, labels)
}

func TestLifecycleFixtureDeployPlan(t *testing.T) {
	plan, err := lifecycleFixturePlan(t, "deploy", lifecycle.Selection{})
	require.NoError(t, err)
	labels := planLabels(t, plan)

	// Mixed kinds, including Helmfile and the custom script kind, are ordered by dependencies.
	assertBefore(t, labels, "terraform/dev/network", "terraform/dev/database")
	assertBefore(t, labels, "terraform/dev/network", "terraform/dev/cache")
	assertBefore(t, labels, "terraform/dev/database", "terraform/dev/app")
	assertBefore(t, labels, "terraform/dev/app", "script/dev/smoke")
	assertBefore(t, labels, "terraform/dev/network", "terraform/prod/app")
	assertBefore(t, labels, "terraform/platform/registry", "helmfile/platform/ingress")
	assert.Equal(t, []string{"helmfile", "apply", "--stack=platform", "ingress"}, plan.Commands[lifecycle.NodeID("helmfile", "platform", "ingress")])

	// Abstract and disabled components never run; Packer has no handler and is reported.
	assert.NotContains(t, labels, "terraform/dev/marker-base")
	assert.NotContains(t, labels, "terraform/dev/retired")
	assert.NotContains(t, labels, "packer/platform/image")
	notices := make([]string, 0, len(plan.Notices))
	for _, notice := range plan.Notices {
		notices = append(notices, notice.Kind+"/"+notice.Stack+"/"+notice.Component)
	}
	assert.Contains(t, notices, "packer/platform/image")
}

func TestLifecycleFixtureDestroyProtectsSurvivingConsumers(t *testing.T) {
	_, err := lifecycleFixturePlan(t, "destroy", lifecycle.Selection{Stack: "dev", Reverse: true})
	require.ErrorIs(t, err, errUtils.ErrSurvivingConsumers)
	assert.Contains(t, err.Error(), "terraform/prod/app -> terraform/dev/network")
}

func TestLifecycleFixtureDestroyWithDependentsReversesOrder(t *testing.T) {
	plan, err := lifecycleFixturePlan(t, "destroy", lifecycle.Selection{Stack: "dev", Dependents: -1, Reverse: true})
	require.NoError(t, err)
	labels := planLabels(t, plan)
	assertBefore(t, labels, "script/dev/smoke", "terraform/dev/app")
	assertBefore(t, labels, "terraform/dev/app", "terraform/dev/database")
	assertBefore(t, labels, "terraform/prod/app", "terraform/dev/network")
	assertBefore(t, labels, "terraform/dev/database", "terraform/dev/network")
	assert.NotContains(t, labels, "terraform/platform/registry")
}
