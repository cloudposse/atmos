package terraform

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
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
