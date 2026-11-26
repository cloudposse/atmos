package terraform

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/ci"
	githubCI "github.com/cloudposse/atmos/pkg/ci/providers/github"
	h "github.com/cloudposse/atmos/pkg/hooks"
	"github.com/cloudposse/atmos/pkg/schema"
)

// planChanges is the error the executor returns for a plan run with -detailed-exitcode that found
// changes: exit 2 carrying the plan's own marker.
func planChanges() error {
	return errUtils.AsDetailedPlanChanges(fmt.Errorf("plan: %w", errUtils.ExitCodeError{Code: 2}))
}

func TestRunCIHooksForNodePreservesDetailedPlanStatus(t *testing.T) {
	withGitHubActionsDetection(t)
	initOutputTestIO(t)
	for _, test := range []struct {
		name       string
		event      h.HookEvent
		err        error
		hasChanges bool
		hasErrors  bool
		ciExitCode int
	}{
		{"unchanged plan", h.AfterTerraformPlan, nil, false, false, 0},
		{"changed plan", h.AfterTerraformPlan, planChanges(), true, false, 2},
		{"failed plan", h.AfterTerraformPlan, errUtils.ExitCodeError{Code: 1}, false, true, 1},
		{"init failure", h.AfterTerraformPlan, errUtils.AsPrerequisiteError(errUtils.ExitCodeError{Code: 2}), false, true, 1},
		{"before hook failure", h.AfterTerraformPlan, fmt.Errorf("%w: %w", errUtils.ErrPerComponentHookFailed, errUtils.AsPrerequisiteError(errUtils.ExitCodeError{Code: 2})), false, true, 1},
		{"policy failure", h.AfterTerraformPlan, errors.Join(planChanges(), errUtils.ErrPolicyDenied), false, true, 1},
		{"exit two before terraform ran", h.AfterTerraformPlan, fmt.Errorf("authentication failed: %w", errUtils.ExitCodeError{Code: 2}), false, true, 1},
		{"apply exit two", h.AfterTerraformApply, errUtils.ExitCodeError{Code: 2}, false, true, 2},
	} {
		t.Run(test.name, func(t *testing.T) {
			outputPath := filepath.Join(t.TempDir(), "output")
			t.Setenv("GITHUB_OUTPUT", outputPath)
			enabled, disabled := true, false
			config := &schema.AtmosConfiguration{CI: schema.CIConfig{
				Enabled: true,
				Output:  schema.CIOutputConfig{Enabled: &enabled},
				Summary: schema.CISummaryConfig{Enabled: &disabled},
				Checks:  schema.CIChecksConfig{Enabled: &disabled},
			}}
			nodeHooks := &terraformNodeHooks{cmd: newHookTestCmd(), afterEvent: test.event}
			info := &schema.ConfigAndStacksInfo{Stack: "dev", ComponentFromArg: "probe"}
			originalExit := errUtils.GetExitCode(test.err)
			nodeHooks.runCIHooksForNode(config, info, "", test.err)
			output, err := os.ReadFile(outputPath)
			require.NoError(t, err)
			require.Contains(t, string(output), fmt.Sprintf("has_changes=%t\n", test.hasChanges))
			require.Contains(t, string(output), fmt.Sprintf("has_errors=%t\n", test.hasErrors))
			require.Contains(t, string(output), fmt.Sprintf("exit_code=%d\n", test.ciExitCode))
			require.Equal(t, originalExit, errUtils.GetExitCode(test.err), "scheduler error remains unchanged")
		})
	}
}

func TestRunHooksOnErrorPreservesDetailedPlanStatus(t *testing.T) {
	command := newErrorPathCITestCommand(t)
	withGitHubActionsDetection(t)
	initOutputTestIO(t)
	for _, test := range []struct {
		name       string
		event      h.HookEvent
		err        error
		hasChanges bool
		hasErrors  bool
		ciExitCode int
	}{
		{"changed plan", h.AfterTerraformPlan, planChanges(), true, false, 2},
		{"failed plan", h.AfterTerraformPlan, errUtils.ExitCodeError{Code: 1}, false, true, 1},
		{"init failure", h.AfterTerraformPlan, errUtils.AsPrerequisiteError(fmt.Errorf("init failed: %w", errUtils.ExitCodeError{Code: 2})), false, true, 1},
		{"before hook failure", h.AfterTerraformPlan, fmt.Errorf("%w: %w", errUtils.ErrPerComponentHookFailed, errUtils.AsPrerequisiteError(errUtils.ExitCodeError{Code: 2})), false, true, 1},
		{"policy failure", h.AfterTerraformPlan, errors.Join(planChanges(), errUtils.ErrPolicyDenied), false, true, 1},
		{"exit two before terraform ran", h.AfterTerraformPlan, fmt.Errorf("authentication failed: %w", errUtils.ExitCodeError{Code: 2}), false, true, 1},
		{"apply exit two", h.AfterTerraformApply, errUtils.ExitCodeError{Code: 2}, false, true, 2},
	} {
		t.Run(test.name, func(t *testing.T) {
			outputPath := filepath.Join(t.TempDir(), "output")
			summaryPath := filepath.Join(t.TempDir(), "summary")
			t.Setenv("GITHUB_OUTPUT", outputPath)
			t.Setenv("GITHUB_STEP_SUMMARY", summaryPath)
			runHooksOnErrorWithOutput(test.event, command, []string{"probe"}, test.err, "")
			output, err := os.ReadFile(outputPath)
			require.NoError(t, err)
			require.Contains(t, string(output), fmt.Sprintf("exit_code=%d\n", test.ciExitCode))
			require.Contains(t, string(output), fmt.Sprintf("has_changes=%t\n", test.hasChanges))
			require.Contains(t, string(output), fmt.Sprintf("has_errors=%t\n", test.hasErrors))
			if test.hasErrors {
				summary, err := os.ReadFile(summaryPath)
				require.NoError(t, err)
				require.Contains(t, string(summary), test.err.Error(), "CI retains the original failure cause")
			}
		})
	}
}

func newErrorPathCITestCommand(t *testing.T) *cobra.Command {
	t.Helper()
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, "stacks"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(root, "components", "terraform", "probe"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "atmos.yaml"), []byte(`base_path: .
components:
  terraform:
    base_path: components/terraform
stacks:
  base_path: stacks
  included_paths: ['*.yaml']
  name_template: '{{ .vars.stage }}'
ci:
  enabled: true
  checks: {enabled: false}
`), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(root, "stacks", "dev.yaml"), []byte("vars: {stage: dev}\ncomponents: {terraform: {probe: {}}}\n"), 0o600))
	t.Chdir(root)
	t.Setenv("ATMOS_CLI_CONFIG_PATH", root)
	command := newHookTestCmd()
	require.NoError(t, command.Flags().Set("stack", "dev"))
	require.NoError(t, command.Flags().Set("ci", "true"))
	return command
}

func TestTerraformHookOutcomePreservesDetailedPlanStatus(t *testing.T) {
	for _, test := range []struct {
		name   string
		event  h.HookEvent
		err    error
		status h.RunStatus
		code   int
	}{
		{"unchanged plan", h.AfterTerraformPlan, nil, h.RunSuccess, 0},
		{"changed plan", h.AfterTerraformPlan, planChanges(), h.RunSuccess, 2},
		{"failed plan", h.AfterTerraformPlan, errUtils.ExitCodeError{Code: 1}, h.RunFailure, 1},
		{"apply exit two", h.AfterTerraformApply, errUtils.ExitCodeError{Code: 2}, h.RunFailure, 2},
		{"policy denied after changes", h.AfterTerraformPlan, errors.Join(planChanges(), errUtils.ErrPolicyDenied), h.RunFailure, 2},
		{"policy evaluation after changes", h.AfterTerraformPlan, errors.Join(planChanges(), errUtils.ErrPolicyEvaluation), h.RunFailure, 2},
		{"exit two from init", h.AfterTerraformPlan, errUtils.AsPrerequisiteError(errUtils.ExitCodeError{Code: 2}), h.RunFailure, 2},
		{"exit two from wrapped workspace selection", h.AfterTerraformPlan, fmt.Errorf("workspace: %w", errUtils.AsPrerequisiteError(errUtils.ExitCodeError{Code: 2})), h.RunFailure, 2},
		{"exit two from before hook", h.AfterTerraformPlan, fmt.Errorf("%w: %w", errUtils.ErrPerComponentHookFailed, errUtils.AsPrerequisiteError(errUtils.ExitCodeError{Code: 2})), h.RunFailure, 2},
		{"exit two before terraform ran", h.AfterTerraformPlan, fmt.Errorf("authentication failed: %w", errUtils.ExitCodeError{Code: 2}), h.RunFailure, 2},
		{"exit two without -detailed-exitcode", h.AfterTerraformPlan, fmt.Errorf("plan: %w", errUtils.ExitCodeError{Code: 2}), h.RunFailure, 2},
		{"marked exit two from a prerequisite", h.AfterTerraformPlan, errUtils.AsPrerequisiteError(planChanges()), h.RunFailure, 2},
		{"marked apply exit two", h.AfterTerraformApply, planChanges(), h.RunFailure, 2},
	} {
		t.Run(test.name, func(t *testing.T) {
			actual := terraformHookOutcome(test.event, test.err)
			require.Equal(t, test.status, actual.Status)
			require.Equal(t, test.code, actual.ExitCode)
			if test.status == h.RunSuccess {
				require.NoError(t, actual.Err)
			} else {
				require.ErrorIs(t, actual.Err, test.err)
			}
			require.Equal(t, test.code, errUtils.GetExitCode(test.err), "external command exit remains unchanged")
		})
	}
}

func TestDetailedPlanEnforcesRequiredAfterHook(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, "stacks"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(root, "components", "terraform", "probe"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "atmos.yaml"), []byte(`base_path: .
components:
  terraform:
    base_path: components/terraform
stacks:
  base_path: stacks
  included_paths: ['*.yaml']
  name_template: '{{ .vars.stage }}'
`), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(root, "stacks", "dev.yaml"), []byte(`vars: {stage: dev}
components:
  terraform:
    probe:
      hooks:
        required-after:
          kind: step
          type: script
          events: [after.terraform.plan]
          on_failure: fail
          with:
            interpreter: starlark
            script: 'fail("required-after-plan-was-enforced")'
`), 0o600))
	t.Chdir(root)
	t.Setenv("ATMOS_CLI_CONFIG_PATH", root)
	withoutCIDetection(t)
	command := newHookTestCmd()
	require.NoError(t, command.Flags().Set("stack", "dev"))
	require.ErrorContains(t, runHooksWithResult(h.AfterTerraformPlan, command, []string{"probe"}, "", planChanges()), "required-after-plan-was-enforced")
	// An exit 2 that did not come from the plan (an authentication failure before Terraform ran)
	// must not run this success-only hook.
	require.NoError(t, runHooksWithResult(h.AfterTerraformPlan, command, []string{"probe"}, "", fmt.Errorf("authentication failed: %w", errUtils.ExitCodeError{Code: 2})))
	// A failed engine plan must not run this success-only hook.
	require.NoError(t, runHooksWithResult(h.AfterTerraformPlan, command, []string{"probe"}, "", errUtils.ExitCodeError{Code: 1}))
	// A before-hook exit 2 must not run success hooks for a plan that never executed.
	beforeErr := fmt.Errorf("%w: %w", errUtils.ErrPerComponentHookFailed, errUtils.AsPrerequisiteError(errUtils.ExitCodeError{Code: 2}))
	require.NoError(t, runHooksWithResult(h.AfterTerraformPlan, command, []string{"probe"}, "", beforeErr))
}

// Exercise planCmd's deferred dispatch with a real child process returning Terraform's
// detailed exit code, rather than invoking the hook dispatcher directly.
func TestPlanCommandDispatchesSuccessHookOnDetailedExit(t *testing.T) {
	// Terraform reads TF_CLI_ARGS_plan itself, so the child plan runs with -detailed-exitcode.
	t.Setenv("TF_CLI_ARGS_plan", "-detailed-exitcode")
	require.ErrorContains(t, runPlanCommandExitingTwo(t), "required-after-plan-was-enforced")
}

// Without -detailed-exitcode an exit 2 is not Terraform's "changes present" status, so the
// success-only after-hook does not run and the command reports the original failure.
func TestPlanCommandDoesNotDispatchSuccessHookForExitTwoWithoutDetailedExitCode(t *testing.T) {
	t.Setenv("TF_CLI_ARGS_plan", "")
	err := runPlanCommandExitingTwo(t)
	require.Error(t, err)
	require.NotContains(t, err.Error(), "required-after-plan-was-enforced")
	require.Equal(t, 2, errUtils.GetExitCode(err))
	require.False(t, errUtils.IsDetailedPlanChanges(err))
}

// runPlanCommandExitingTwo runs planCmd for a component whose engine is the test binary, which
// exits 2 for `plan`, with a required success-only after-plan hook. It returns RunE's error.
func runPlanCommandExitingTwo(t *testing.T) error {
	t.Helper()
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, "stacks"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(root, "components", "terraform", "probe"), 0o755))
	executable, err := os.Executable()
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(root, "atmos.yaml"), []byte(fmt.Sprintf(`base_path: .
components:
  terraform:
    base_path: components/terraform
    command: %q
    auto_generate_backend_file: false
stacks:
  base_path: stacks
  included_paths: ['*.yaml']
  name_template: '{{ .vars.stage }}'
`, executable)), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(root, "stacks", "dev.yaml"), []byte(`vars: {stage: dev}
components:
  terraform:
    probe:
      hooks:
        required-after:
          kind: step
          type: script
          events: [after.terraform.plan]
          on_failure: fail
          with:
            interpreter: starlark
            script: 'fail("required-after-plan-was-enforced")'
`), 0o600))
	t.Chdir(root)
	t.Setenv("ATMOS_CLI_CONFIG_PATH", root)
	t.Setenv("_ATMOS_TEST_PLAN_EXIT_TWO", "1")
	withoutCIDetection(t)
	initOutputTestIO(t)
	rootCmd := &cobra.Command{Use: "atmos"}
	rootCmd.PersistentFlags().String("base-path", "", "")
	rootCmd.PersistentFlags().StringSlice("config", nil, "")
	rootCmd.PersistentFlags().StringSlice("config-path", nil, "")
	rootCmd.PersistentFlags().StringSlice("profile", nil, "")
	oldParent := terraformCmd.Parent()
	rootCmd.AddCommand(terraformCmd)
	t.Cleanup(func() {
		rootCmd.RemoveCommand(terraformCmd)
		if oldParent != nil {
			oldParent.AddCommand(terraformCmd)
		}
	})
	stackFlag := terraformCmd.PersistentFlags().Lookup("stack")
	oldStack, stackChanged := stackFlag.Value.String(), stackFlag.Changed
	skipInitFlag := terraformCmd.PersistentFlags().Lookup("skip-init")
	oldSkipInit, skipInitChanged := skipInitFlag.Value.String(), skipInitFlag.Changed
	t.Cleanup(func() {
		_ = terraformCmd.PersistentFlags().Set("stack", oldStack)
		stackFlag.Changed = stackChanged
		_ = terraformCmd.PersistentFlags().Set("skip-init", oldSkipInit)
		skipInitFlag.Changed = skipInitChanged
	})
	require.NoError(t, terraformCmd.PersistentFlags().Set("stack", "dev"))
	require.NoError(t, terraformCmd.PersistentFlags().Set("skip-init", "true"))
	return planCmd.RunE(planCmd, []string{"probe"})
}

func TestPrerequisiteErrorKeepsMessageAndChain(t *testing.T) {
	require.NoError(t, errUtils.AsPrerequisiteError(nil))
	original := fmt.Errorf("init failed: %w", errUtils.ExitCodeError{Code: 3})
	wrapped := errUtils.AsPrerequisiteError(original)
	require.EqualError(t, wrapped, original.Error())
	require.ErrorIs(t, wrapped, original)
	require.Equal(t, 3, errUtils.GetExitCode(wrapped))
}

// A required after-hook that fails on a detailed plan must not suppress the CI result: the CI hooks
// publish the plan summary first and the hook failure is returned afterwards.
func TestDetailedPlanPublishesCIResultBeforeReturningRequiredHookFailure(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, "stacks"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(root, "components", "terraform", "probe"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "atmos.yaml"), []byte(`base_path: .
components:
  terraform:
    base_path: components/terraform
stacks:
  base_path: stacks
  included_paths: ['*.yaml']
  name_template: '{{ .vars.stage }}'
ci:
  enabled: true
`), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(root, "stacks", "dev.yaml"), []byte(`vars: {stage: dev}
components:
  terraform:
    probe:
      hooks:
        required-after:
          kind: step
          type: script
          events: [after.terraform.plan]
          on_failure: fail
          with:
            interpreter: starlark
            script: 'fail("required-after-plan-was-enforced")'
`), 0o600))
	t.Chdir(root)
	t.Setenv("ATMOS_CLI_CONFIG_PATH", root)
	summary := filepath.Join(root, "summary.md")
	restoreRegistry := ci.SwapRegistryForTest()
	t.Cleanup(restoreRegistry)
	ci.Register(githubCI.NewProvider())
	t.Setenv("GITHUB_ACTIONS", "true")
	t.Setenv("GITHUB_STEP_SUMMARY", summary)
	initOutputTestIO(t)
	command := newHookTestCmd()
	require.NoError(t, command.Flags().Set("stack", "dev"))
	require.NoError(t, command.Flags().Set("ci", "true"))

	planOutput := "Terraform will perform the following actions:\n\nPlan: 1 to add, 0 to change, 0 to destroy.\n"
	err := runHooksWithResult(h.AfterTerraformPlan, command, []string{"probe"}, planOutput, planChanges())
	require.ErrorContains(t, err, "required-after-plan-was-enforced")
	published, readErr := os.ReadFile(summary)
	require.NoError(t, readErr, "the CI hooks must still publish the plan result")
	require.Contains(t, string(published), "1 to add")
}
