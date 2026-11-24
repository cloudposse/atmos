package terraform

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/component"
	"github.com/cloudposse/atmos/pkg/component/terragruntexec"
	"github.com/cloudposse/atmos/pkg/process"
	"github.com/cloudposse/atmos/pkg/schema"
)

// Compile-time guard for the option field the shell runner reads.
var _ = component.ExecutionOptions{Interrupt: process.InterruptGraceful}

// Terraform must stop on its own after Ctrl-C to save state and release its lock, so the
// Terraform invocation opts in; a user hook command keeps the default terminate behavior.
func TestTerraformCommandStopsGracefullyButHooksDoNot(t *testing.T) {
	shell := newShellFixture(t, nil)
	info := hookInfo("apply", &schema.ComponentExecution{
		BeforeHooks: []schema.ComponentHook{execHook("before", "apply")},
	})
	require.NoError(t, hookExecutor(shell).runMainTerraformCommand(&schema.AtmosConfiguration{}, info,
		[]string{"apply", "-auto-approve"}, t.TempDir(), component.WithEnvironment([]string{"IDENTITY=1"})))

	require.Equal(t, []string{"hook-before arg", "terraform apply"}, shell.names())
	assert.Equal(t, process.InterruptKill, shell.calls[0].options.Interrupt, "hooks keep their existing behavior")
	terraformCall := shell.calls[1]
	assert.Equal(t, process.InterruptGraceful, terraformCall.options.Interrupt)
	assert.Equal(t, []string{"IDENTITY=1"}, terraformCall.options.ProcessEnv, "caller options are preserved")
}

func TestTerraformHelperInvocationsStopGracefully(t *testing.T) {
	t.Run("reviewed module download", func(t *testing.T) {
		getter := newShellFixture(t, nil)
		executor := testExecutor(t)
		executor.services.ExecuteShellCommand = getter.mock.Execute
		info := &schema.ConfigAndStacksInfo{Command: "terraform", SanitizedEnv: []string{"PATH=/sanitized"}}
		rebuild := &reviewedRebuild{plan: &configurationPlan{Modules: true}}
		require.NoError(t, executor.installReviewedModules(&schema.AtmosConfiguration{}, info, rebuild, t.TempDir()))
		require.Len(t, getter.calls, 1)
		assert.Equal(t, process.InterruptGraceful, getter.calls[0].options.Interrupt)
		assert.Equal(t, []string{"PATH=/sanitized"}, getter.calls[0].options.ProcessEnv)
	})
	t.Run("version", func(t *testing.T) {
		shell := newShellFixture(t, nil)
		info := &schema.ConfigAndStacksInfo{Command: "terraform", SubCommand: "version"}
		config := &schema.AtmosConfiguration{BasePath: t.TempDir()}
		require.NoError(t, hookExecutor(shell).handleVersionSubcommand(config, info))
		require.Equal(t, []string{"terraform version"}, shell.names())
		assert.Equal(t, process.InterruptGraceful, shell.calls[0].options.Interrupt)
	})
	t.Run("terragrunt executable mode", func(t *testing.T) {
		shell := newShellFixture(t, nil)
		host := &terragruntHost{
			executor:    hookExecutor(shell),
			atmosConfig: &schema.AtmosConfiguration{},
			info:        &schema.ConfigAndStacksInfo{SanitizedEnv: []string{"PATH=/sanitized"}},
		}
		command := &terragruntexec.Command{Binary: "terragrunt", Args: []string{"apply"}}
		require.NoError(t, host.Execute(t.Context(), command))
		require.Len(t, shell.calls, 1)
		assert.Equal(t, process.InterruptGraceful, shell.calls[0].options.Interrupt)
	})
}

// The helper must not alias the caller's option slice, which retry wrappers reuse per attempt.
func TestGracefulInterruptOptionsDoesNotMutateTheCallerSlice(t *testing.T) {
	base := make([]component.ExecutionOption, 1, 4)
	base[0] = component.WithEnvironment([]string{"A=1"})
	first := gracefulInterruptOptions(base)
	second := gracefulInterruptOptions(base)
	require.Len(t, base, 1)
	require.Len(t, first, 2)
	require.Len(t, second, 2)
	var resolved component.ExecutionOptions
	for _, option := range first {
		option(&resolved)
	}
	assert.Equal(t, process.InterruptGraceful, resolved.Interrupt)
	assert.Equal(t, []string{"A=1"}, resolved.ProcessEnv)
}
