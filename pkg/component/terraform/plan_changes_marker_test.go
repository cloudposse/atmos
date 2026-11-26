package terraform

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/component"
	"github.com/cloudposse/atmos/pkg/schema"
)

// The executor marks exit 2 of a plan run with -detailed-exitcode, and only that, so the command layer
// can tell the plan's "changes present" status from any other failure that carries exit 2.
func TestSuccessfulPlanResultMarksOnlyTheDetailedPlanStatus(t *testing.T) {
	exitTwo := errUtils.ExitCodeError{Code: 2}
	marked := successfulPlanResult("plan", []string{"plan", "-detailed-exitcode"}, nil, exitTwo)
	require.True(t, errUtils.IsDetailedPlanChanges(marked))
	require.True(t, completedPlanWithChanges(marked))
	require.ErrorIs(t, marked, exitTwo, "the marker keeps the original chain")
	require.Equal(t, 2, errUtils.GetExitCode(marked))

	// An empty process environment keeps TF_CLI_ARGS* exported by the developer out of the negative cases.
	isolated := component.WithEnvironment([]string{})
	for name, err := range map[string]error{
		"plan without -detailed-exitcode": successfulPlanResult("plan", []string{"plan"}, nil, exitTwo, isolated),
		"another command":                 successfulPlanResult("apply", []string{"apply", "-detailed-exitcode"}, nil, exitTwo),
		"another exit code":               successfulPlanResult("plan", []string{"plan", "-detailed-exitcode"}, nil, errUtils.ExitCodeError{Code: 1}),
		"a hook that exited with 2":       successfulPlanResult("plan", []string{"plan", "-detailed-exitcode"}, nil, errors.Join(errUtils.ErrComponentHookFailed, exitTwo)),
	} {
		require.False(t, errUtils.IsDetailedPlanChanges(err), name)
	}
	require.NoError(t, successfulPlanResult("plan", []string{"plan", "-detailed-exitcode"}, nil, nil))
}

// Executable mode runs one `terragrunt run -- plan` per unit; its exit 2 is the plan's own status only
// when -detailed-exitcode is in effect.
func TestExecuteTerragruntUnitMarksDetailedPlanChanges(t *testing.T) {
	for _, test := range []struct {
		name     string
		args     []string
		detailed bool
	}{
		{"with -detailed-exitcode", []string{"-no-color", "-detailed-exitcode"}, true},
		{"without -detailed-exitcode", []string{"-no-color"}, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			harness, info, _ := unitHarness(t)
			info.AdditionalArgsAndFlags = test.args
			info.ComponentEnvSection[testEnvProbeExit] = "2"
			t.Cleanup(ClearLastAuthContext)

			err := harness.executor.executeTerragruntUnit(t.Context(), harness.config, info, unitRun)
			require.Error(t, err)
			require.Equal(t, 2, errUtils.GetExitCode(err))
			require.Equal(t, test.detailed, errUtils.IsDetailedPlanChanges(err))
		})
	}
}

// A component without provider-supplied execution still runs Terraform with atmos.yaml `env`, so
// TF_CLI_ARGS_plan=-detailed-exitcode set there makes the plan's exit 2 the marked "changes present"
// status, exactly as it does for a component with extra arguments.
func TestRunExecutionPhaseWithoutExecutionHonorsAtmosEnvDetailedExitCode(t *testing.T) {
	shell := newShellFixture(t, nil)
	e := hookExecutor(shell)
	phase := &executionPhase{command: "plan", args: []string{"plan"}, insertAt: 1, componentPath: t.TempDir()}
	exitTwo := errUtils.ExitCodeError{Code: 2}
	isolated := []component.ExecutionOption{component.WithEnvironment([]string{})}
	run := func(env map[string]string) error {
		return e.runExecutionPhase(&schema.AtmosConfiguration{Env: env}, hookInfo("plan", nil), phase, isolated,
			func(*schema.ConfigAndStacksInfo, []string, []component.ExecutionOption) error { return exitTwo })
	}

	marked := run(map[string]string{"TF_CLI_ARGS_plan": "-detailed-exitcode"})
	require.True(t, errUtils.IsDetailedPlanChanges(marked))
	require.ErrorIs(t, marked, exitTwo)

	unmarked := run(nil)
	require.False(t, errUtils.IsDetailedPlanChanges(unmarked), "without the flag exit 2 is an ordinary failure")
	require.ErrorIs(t, unmarked, exitTwo)
}
