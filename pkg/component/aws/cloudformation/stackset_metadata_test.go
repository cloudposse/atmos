package cloudformation

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth"
	"github.com/cloudposse/atmos/pkg/component"
	"github.com/cloudposse/atmos/pkg/schema"
)

// TestExecuteSingle_StackSetLocks verifies metadata restrictions before authentication,
// including dry-run and confirmation overrides, while preserving unlocked and listing operations.
func TestExecuteSingle_StackSetLocks(t *testing.T) {
	operations := []struct {
		operation Operation
		mutates   bool
	}{
		{OperationStackSetCreate, true},
		{OperationStackSetUpdate, true},
		{OperationStackSetDelete, true},
		{OperationStackSetInstances, false},
	}
	for _, operation := range operations {
		for _, locked := range []bool{false, true} {
			for _, dryRun := range []bool{false, true} {
				for _, autoApprove := range []bool{false, true} {
					name := fmt.Sprintf("%s/locked=%t/dry-run=%t/auto-approve=%t", operation.operation, locked, dryRun, autoApprove)
					t.Run(name, func(t *testing.T) {
						verifyStackSetLock(t, operation.operation, locked, dryRun, autoApprove, locked && operation.mutates)
					})
				}
			}
		}
	}
}

func verifyStackSetLock(t *testing.T, operation Operation, locked, dryRun, autoApprove, rejected bool) {
	t.Helper()
	authErr := errors.New("authentication reached")
	authCalls := 0
	installExecutorSeamStubs(t, executorSeamStubs{
		processStacks: func(_ *schema.AtmosConfiguration, info schema.ConfigAndStacksInfo, _, _, _ bool, _ []string, _ auth.AuthManager) (schema.ConfigAndStacksInfo, error) {
			info.ComponentIsEnabled = true
			info.ComponentIsLocked = locked
			info.ComponentSection = map[string]any{
				"stack_name": "fleet",
				"path":       "template.yaml",
				"provision":  map[string]any{"targets": map[string]any{"fleet": map[string]any{"kind": "aws/stackset"}}},
			}
			return info, nil
		},
		setupComponentAuthForCLI: func(_ *schema.AtmosConfiguration, _ *schema.ConfigAndStacksInfo) (auth.AuthManager, error) {
			authCalls++
			return nil, authErr
		},
	})
	// Metadata is deliberately absent from the input: restrictions must use the resolved stack.
	info := schema.ConfigAndStacksInfo{ComponentFromArg: "fleet", DryRun: dryRun}
	ctx := &component.ExecutionContext{Flags: map[string]any{"auto-approve": autoApprove}}
	err := executeSingle(ctx, &schema.AtmosConfiguration{}, &info, operation)
	switch {
	case rejected:
		require.ErrorIs(t, err, errUtils.ErrInvalidSpecificAwsCloudFormationComponent)
		assert.Contains(t, err.Error(), "locked")
		assert.Contains(t, err.Error(), "fleet")
		assert.Zero(t, authCalls)
	case dryRun:
		require.NoError(t, err)
		assert.Zero(t, authCalls)
	default:
		require.ErrorIs(t, err, authErr)
		assert.Equal(t, 1, authCalls)
	}
}
