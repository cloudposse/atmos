package cloudformation

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth"
	"github.com/cloudposse/atmos/pkg/component"
	"github.com/cloudposse/atmos/pkg/schema"
)

// TestExecuteSingle_ChangesetMetadata prevents named execution from bypassing component locks.
func TestExecuteSingle_ChangesetMetadata(t *testing.T) {
	operations := []Operation{
		OperationChangesetExecute, OperationChangesetCreate, OperationChangesetList, OperationChangesetDelete,
		OperationDiff, OperationDriftDetect, OperationDriftDescribe, OperationGetTemplate, OperationGetPolicy,
	}
	modes := []struct {
		name                string
		dryRun, autoApprove bool
	}{
		{name: "normal"},
		{name: "dry-run", dryRun: true},
		{name: "auto-approve", autoApprove: true},
		{name: "dry-run-auto-approve", dryRun: true, autoApprove: true},
	}
	for _, operation := range operations {
		for _, locked := range []bool{false, true} {
			for _, mode := range modes {
				name := string(operation) + "/unlocked/" + mode.name
				if locked {
					name = string(operation) + "/locked/" + mode.name
				}
				t.Run(name, func(t *testing.T) {
					authErr := errors.New("authentication reached")
					authCalls := 0
					installExecutorSeamStubs(t, executorSeamStubs{
						processStacks: func(_ *schema.AtmosConfiguration, info schema.ConfigAndStacksInfo, _, _, _ bool, _ []string, _ auth.AuthManager) (schema.ConfigAndStacksInfo, error) {
							info.ComponentIsEnabled = true
							info.ComponentIsLocked = locked
							info.ComponentSection = map[string]any{"stack_name": "vpc", "path": "template.yaml"}
							return info, nil
						},
						setupComponentAuthForCLI: func(_ *schema.AtmosConfiguration, _ *schema.ConfigAndStacksInfo) (auth.AuthManager, error) {
							authCalls++
							return nil, authErr
						},
					})
					info := schema.ConfigAndStacksInfo{ComponentFromArg: "vpc", DryRun: mode.dryRun}
					ctx := &component.ExecutionContext{Flags: map[string]any{
						"changeset-name": "reviewed-change", "auto-approve": mode.autoApprove,
					}}
					err := executeSingle(ctx, &schema.AtmosConfiguration{}, &info, operation)
					switch {
					case locked && operation == OperationChangesetExecute:
						require.ErrorIs(t, err, errUtils.ErrInvalidSpecificAwsCloudFormationComponent)
						assert.Contains(t, err.Error(), `component "vpc" is locked`)
						assert.Zero(t, authCalls)
					case mode.dryRun:
						require.NoError(t, err)
						assert.Zero(t, authCalls)
					default:
						require.ErrorIs(t, err, authErr)
						assert.Equal(t, 1, authCalls)
					}
				})
			}
		}
	}
}
