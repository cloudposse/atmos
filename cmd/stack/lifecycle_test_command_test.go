package stack

import (
	"path/filepath"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/stack/lifecycle"
)

func TestStackTestCommandRegistration(t *testing.T) {
	command, rest, err := stackCmd.Find([]string{"test"})
	require.NoError(t, err)
	require.Empty(t, rest)
	assert.Equal(t, "test", command.Name())
	assert.True(t, command.Runnable())
	assert.Nil(t, command.Flag("allow-destroy"))
}

func TestStackTestDryRunSelectsTerraformAndExcludesUnsupportedKinds(t *testing.T) {
	cmd, _ := lifecycleFixtureCommand(t, "test", "--stack=platform", "--dry-run")
	output := captureLifecycle(t)
	require.NoError(t, cmd.RunE(cmd, nil))
	text := output.String()
	assert.Contains(t, text, "Would test terraform/platform/registry")
	assert.Contains(t, text, "terraform test --stack=platform registry")
	assert.Contains(t, text, "helmfile/platform/ingress: component kind does not define this operation")
	assert.Contains(t, text, "packer/platform/image: component kind does not define this operation")
	assert.NotContains(t, text, "Would deploy")
	assert.NotContains(t, text, "Would build")
	assert.NotContains(t, text, "terraform/dev/")
}

func TestStackTestComponentSelectionAndChildFailures(t *testing.T) {
	for _, exitCode := range []int{0, 3} {
		t.Run(strconv.Itoa(exitCode), func(t *testing.T) {
			plan, err := lifecycleFixturePlan(t, "test", &lifecycle.Selection{Stack: "dev", Components: []string{"network", "app"}})
			require.NoError(t, err)
			require.Equal(t, []string{"terraform/dev/network", "terraform/dev/app"}, planLabels(t, plan))
			logPath := filepath.Join(t.TempDir(), "children.log")
			t.Setenv(childExitEnv, strconv.Itoa(exitCode))
			t.Setenv(childLogEnv, logPath)
			output := captureLifecycle(t)
			run := &lifecycleRun{
				config: &schema.AtmosConfiguration{}, plan: plan,
				options: &lifecycleOptions{verb: "test", concurrency: 1, failureMode: "fail-fast", logOrder: "stream"},
			}
			err = runLifecyclePlan(lifecycleTestCommandWithContext(), run)
			commands := []string{"terraform test --stack=dev network"}
			if exitCode == 0 {
				require.NoError(t, err)
				commands = append(commands, "terraform test --stack=dev app")
				assert.Contains(t, output.String(), "2 succeeded, 0 failed, 0 skipped")
			} else {
				require.ErrorIs(t, err, errUtils.ErrComponentExecutionFailed)
				assert.Contains(t, output.String(), "test terraform/dev/app: skipped")
			}
			assert.Equal(t, commands, childCommandLines(t, logPath))
		})
	}
}

func TestStackTestPreviewUsesNativeTestHooks(t *testing.T) {
	plan, err := lifecycleFixturePlan(t, "test", &lifecycle.Selection{Stack: "platform"})
	require.NoError(t, err)
	node := plan.Graph.Nodes[lifecycle.NodeID("terraform", "platform", "registry")]
	require.NotNil(t, node)
	node.Metadata["hooks"] = map[string]any{
		"start":       map[string]any{"kind": "command", "command": "sandbox-start", "events": []any{"before.terraform.test"}},
		"cleanup":     map[string]any{"kind": "command", "command": "sandbox-stop", "when": "always", "events": []any{"after.terraform.test"}},
		"deploy-only": map[string]any{"kind": "command", "command": "deploy-only", "events": []any{"before.terraform.deploy"}},
	}
	cmd := newLifecycleCommand("test")
	require.NoError(t, cmd.ParseFlags([]string{"--skip-hooks=start"}))
	output := captureLifecycle(t)
	require.NoError(t, previewLifecycle(plan, "test", nil, cmd))
	text := output.String()
	assert.Contains(t, text, "hook start: event=before.terraform.test")
	assert.Contains(t, text, "skipped by --skip-hooks")
	assert.Contains(t, text, "hook cleanup: event=after.terraform.test kind=command when=always")
	assert.NotContains(t, text, "deploy-only")
}

func TestStackTestAcceptsRepeatedAndCommaSeparatedComponents(t *testing.T) {
	cmd, _ := lifecycleFixtureCommand(t, "test", "--stack=dev", "--component=network,app", "--component=database", "--dry-run")
	output := captureLifecycle(t)
	require.NoError(t, cmd.RunE(cmd, nil))
	text := output.String()
	for _, name := range []string{"network", "app", "database"} {
		assert.Contains(t, text, "Would test terraform/dev/"+name)
	}
	assert.NotContains(t, text, "Would test terraform/dev/cache")
	assert.NotContains(t, text, "Would deploy")
}

func TestStackTestConcurrentExecutionNeedsNoDeployApproval(t *testing.T) {
	plan, err := lifecycleFixturePlan(t, "test", &lifecycle.Selection{Stack: "platform"})
	require.NoError(t, err)
	logPath := filepath.Join(t.TempDir(), "children.log")
	t.Setenv(childExitEnv, "0")
	t.Setenv(childLogEnv, logPath)
	captureLifecycle(t)
	run := &lifecycleRun{
		config: &schema.AtmosConfiguration{}, plan: plan,
		options: &lifecycleOptions{verb: "test", concurrency: 2, failureMode: "fail-fast", logOrder: "grouped"},
	}
	require.NoError(t, runLifecyclePlan(lifecycleTestCommandWithContext(), run))
	assert.Equal(t, []string{"terraform test --stack=platform registry"}, childCommandLines(t, logPath))
}
