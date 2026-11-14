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
)

func TestTerraformHookOutcomePreservesDetailedPlanStatus(t *testing.T) {
	for _, test := range []struct {
		name   string
		event  h.HookEvent
		err    error
		status h.RunStatus
		code   int
	}{
		{"unchanged plan", h.AfterTerraformPlan, nil, h.RunSuccess, 0},
		{"changed plan", h.AfterTerraformPlan, fmt.Errorf("plan: %w", errUtils.ExitCodeError{Code: 2}), h.RunSuccess, 2},
		{"failed plan", h.AfterTerraformPlan, errUtils.ExitCodeError{Code: 1}, h.RunFailure, 1},
		{"apply exit two", h.AfterTerraformApply, errUtils.ExitCodeError{Code: 2}, h.RunFailure, 2},
		{"policy denied after changes", h.AfterTerraformPlan, errors.Join(errUtils.ExitCodeError{Code: 2}, errUtils.ErrPolicyDenied), h.RunFailure, 2},
		{"policy evaluation after changes", h.AfterTerraformPlan, errors.Join(errUtils.ExitCodeError{Code: 2}, errUtils.ErrPolicyEvaluation), h.RunFailure, 2},
		{"exit two from init", h.AfterTerraformPlan, errUtils.AsPrerequisiteError(errUtils.ExitCodeError{Code: 2}), h.RunFailure, 2},
		{"exit two from wrapped workspace selection", h.AfterTerraformPlan, fmt.Errorf("workspace: %w", errUtils.AsPrerequisiteError(errUtils.ExitCodeError{Code: 2})), h.RunFailure, 2},
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
	require.ErrorContains(t, runHooksWithResult(h.AfterTerraformPlan, command, []string{"probe"}, "", errUtils.ExitCodeError{Code: 2}), "required-after-plan-was-enforced")
	// A failed engine plan must not run this success-only hook.
	require.NoError(t, runHooksWithResult(h.AfterTerraformPlan, command, []string{"probe"}, "", errUtils.ExitCodeError{Code: 1}))
}

// Exercise planCmd's deferred dispatch with a real child process returning Terraform's
// detailed exit code, rather than invoking the hook dispatcher directly.
func TestPlanCommandDispatchesSuccessHookOnDetailedExit(t *testing.T) {
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
	require.ErrorContains(t, planCmd.RunE(planCmd, []string{"probe"}), "required-after-plan-was-enforced")
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
	err := runHooksWithResult(h.AfterTerraformPlan, command, []string{"probe"}, planOutput, errUtils.ExitCodeError{Code: 2})
	require.ErrorContains(t, err, "required-after-plan-was-enforced")
	published, readErr := os.ReadFile(summary)
	require.NoError(t, readErr, "the CI hooks must still publish the plan result")
	require.Contains(t, string(published), "1 to add")
}
