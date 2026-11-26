package adapters

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

// runPlanAll plans the dev test stack (app depends on database, database on vpc) with vpc failing
// with vpcErr, and returns the components that ran and the command error.
func runPlanAll(t *testing.T, vpcErr error) ([]string, error) {
	t.Helper()
	var executed []string
	err := ExecuteTerraform(context.Background(), TerraformOptions{
		AtmosConfig: &schema.AtmosConfiguration{},
		Info:        &schema.ConfigAndStacksInfo{All: true, SubCommand: "plan"},
		Stacks:      terraformAdapterTestStacks(),
		Executor: func(execution TerraformExecution) (TerraformExecutionResult, error) {
			executed = append(executed, execution.Info.Component)
			if execution.Info.Component == "vpc" {
				return TerraformExecutionResult{}, vpcErr
			}
			return TerraformExecutionResult{}, nil
		},
	})
	return executed, err
}

// An exit status 2 that did not come from the plan subprocess (an authentication failure before
// Terraform ran, for example) is a failed node: its dependents do not run as if it had changes.
func TestExecuteTerraformPlanExitTwoOutsideThePlanIsAFailure(t *testing.T) {
	executed, err := runPlanAll(t, fmt.Errorf("authentication failed: %w", errUtils.ExitCodeError{Code: 2}))
	require.Error(t, err)
	require.Equal(t, []string{"vpc"}, executed, "the dependents of a failed node are skipped")
}

// The plan subprocess's own exit 2 (marked by the executor) is a successful node with changes: its
// dependents run and the bulk command reports exit 2.
func TestExecuteTerraformPlanExitTwoFromThePlanIsChangedSuccess(t *testing.T) {
	executed, err := runPlanAll(t, errUtils.AsDetailedPlanChanges(errUtils.ExitCodeError{Code: 2}))
	var exitErr errUtils.ExitCodeError
	require.ErrorAs(t, err, &exitErr)
	require.Equal(t, 2, exitErr.Code)
	require.Equal(t, []string{"vpc", "database", "app"}, executed)
}
