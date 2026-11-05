package stack

import (
	"path/filepath"
	"strconv"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/dependency"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/stack/lifecycle"
)

func TestStackPlanCommandRegistration(t *testing.T) {
	command, rest, err := stackCmd.Find([]string{"plan"})
	require.NoError(t, err)
	require.Empty(t, rest)
	assert.Equal(t, "plan", command.Name())
	assert.True(t, command.Runnable())
	assert.Nil(t, command.Flag("allow-destroy"))
	assert.ErrorIs(t, command.Args(command, []string{"app"}), errUtils.ErrInvalidPositionalArgs)
}

func TestStackPlanNativeMappingsAndHookPreviews(t *testing.T) {
	for _, tc := range []struct {
		kind  string
		path  []string
		event string
	}{
		{"terraform", []string{"terraform", "plan"}, "terraform.plan"},
		{"helmfile", []string{"helmfile", "diff"}, "helmfile.diff"},
		{"helm", []string{"helm", "plan"}, "helm.diff"},
		{"kubernetes", []string{"kubernetes", "plan"}, "kubernetes.plan"},
		{"aws/cloudformation", []string{"aws", "cloudformation", "plan"}, "aws/cloudformation.diff"},
	} {
		t.Run(tc.kind, func(t *testing.T) {
			handler, ok := lifecycle.DefaultRegistry.Lookup(tc.kind, lifecycle.PlanEvent)
			require.True(t, ok)
			assert.Equal(t, tc.path, handler.Path)
			assert.Equal(t, []string{"before." + tc.event, "after." + tc.event}, handler.HookEvents)
			assert.Empty(t, handler.ApprovalArgs)
			node := &dependency.Node{Type: tc.kind, Stack: "dev", Component: "app", Metadata: map[string]any{"hooks": map[string]any{
				"before-plan": map[string]any{"kind": "command", "command": "prepare", "events": []any{"before." + tc.event}},
				"after-plan":  map[string]any{"kind": "command", "command": "report", "events": []any{"after." + tc.event}},
				"deploy-only": map[string]any{"kind": "command", "command": "deploy-only", "events": []any{"before." + tc.kind + ".deploy"}},
			}}}
			previews, err := previewHooks(node, handler.HookEvents, "before-plan")
			require.NoError(t, err)
			require.Len(t, previews, 2)
			assert.Contains(t, previews[0], "hook before-plan: event=before."+tc.event)
			assert.Contains(t, previews[0], "skipped by --skip-hooks")
			assert.Contains(t, previews[1], "hook after-plan: event=after."+tc.event)
		})
	}
}

func TestStackPlanDryRunUsesNativeOperationsWithoutLaunchingChildren(t *testing.T) {
	cmd, _ := lifecycleFixtureCommand(t, "plan", "--stack=platform", "--dry-run")
	logPath := filepath.Join(t.TempDir(), "children.log")
	t.Setenv(childExitEnv, "0")
	t.Setenv(childLogEnv, logPath)
	output := captureLifecycle(t)
	require.NoError(t, cmd.RunE(cmd, nil))
	text := output.String()
	assert.Contains(t, text, "Would plan terraform/platform/registry")
	assert.Contains(t, text, "terraform plan --stack=platform registry")
	assert.Contains(t, text, "Would plan helmfile/platform/ingress")
	assert.Contains(t, text, "helmfile diff --stack=platform ingress")
	assert.Contains(t, text, "packer/platform/image: component kind does not define this operation")
	assert.NotContains(t, text, "Would deploy")
	assert.NoFileExists(t, logPath)
}

func TestStackPlanExecutesNativeCommandsAndPropagatesFailures(t *testing.T) {
	for _, exitCode := range []int{0, 3} {
		t.Run(strconv.Itoa(exitCode), func(t *testing.T) {
			run, logPath := lifecycleChildRun(t, "plan", exitCode, nil)
			output := captureLifecycle(t)
			err := runLifecyclePlan(lifecycleTestCommandWithContext(), run)
			commands := []string{"--base-path=fixture terraform plan --stack=platform registry"}
			if exitCode == 0 {
				require.NoError(t, err)
				commands = append(commands, "--base-path=fixture helmfile diff --stack=platform ingress")
				assert.Contains(t, output.String(), "2 succeeded, 0 failed, 0 skipped")
			} else {
				require.ErrorIs(t, err, errUtils.ErrComponentExecutionFailed)
				assert.Contains(t, output.String(), "plan helmfile/platform/ingress: skipped")
			}
			assert.Equal(t, commands, childCommandLines(t, logPath))
		})
	}
}

func TestStackPlanSelectsRepeatedAndCommaSeparatedComponents(t *testing.T) {
	cmd, _ := lifecycleFixtureCommand(t, "plan", "--stack=dev", "--component=network,app", "--component=database", "--dry-run")
	output := captureLifecycle(t)
	require.NoError(t, cmd.RunE(cmd, nil))
	for _, name := range []string{"network", "app", "database"} {
		assert.Contains(t, output.String(), "Would plan terraform/dev/"+name)
	}
	assert.NotContains(t, output.String(), "Would plan terraform/dev/cache")
	opts, err := lifecycleTestCommand(t, "plan", "--component")
	require.NoError(t, err)
	assert.True(t, needsLifecycleStack(&opts))
	assert.True(t, needsLifecycleComponent(&opts))
}

func TestStackPlanConcurrencyDoesNotRequireDeployApproval(t *testing.T) {
	run, logPath := lifecycleChildRun(t, "plan", 0, func(o *lifecycleOptions) { o.concurrency = 2 })
	captureLifecycle(t)
	require.NoError(t, runLifecyclePlan(lifecycleTestCommandWithContext(), run))
	assert.Equal(t, []string{
		"--base-path=fixture terraform plan --stack=platform registry",
		"--base-path=fixture helmfile diff --stack=platform ingress",
	}, childCommandLines(t, logPath))
}

func TestStackPlanCustomCapabilityDiscovery(t *testing.T) {
	root := &cobra.Command{Use: "atmos"}
	custom := &cobra.Command{Use: "plan", Annotations: map[string]string{"customCommand": "true"}}
	root.AddCommand(custom)
	config := &schema.AtmosConfiguration{Commands: []schema.Command{{
		Name: "plan", Component: &schema.CommandComponent{Type: "widget"},
		Arguments: []schema.CommandArgument{{Name: "component", Provides: "component"}},
		Flags:     []schema.CommandFlag{{Name: "stack", Provides: "stack"}},
	}}}
	handlers := discoverLifecycleCommands(root, config, "plan")
	require.Contains(t, handlers, "widget")
	args, err := handlers["widget"].Arguments(&dependency.Node{Stack: "dev", Component: "app"})
	require.NoError(t, err)
	assert.Equal(t, []string{"plan", "--stack=dev", "app"}, args)
	assert.NotContains(t, discoverLifecycleCommands(root, config, "deploy"), "widget")
}
