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

// TestExecuteSingle_MetadataRestrictions verifies guards precede authentication and dry-run dispatch.
func TestExecuteSingle_MetadataRestrictions(t *testing.T) {
	for _, metadata := range []string{"abstract", "locked", "real"} {
		for _, operation := range []Operation{OperationDiff, OperationApply, OperationDelete, OperationValidate, OperationOutput} {
			for _, dryRun := range []bool{false, true} {
				name := metadata + "/" + string(operation)
				if dryRun {
					name += "/dry-run"
				}
				t.Run(name, func(t *testing.T) {
					authErr := errors.New("authentication reached")
					authCalls := 0
					installExecutorSeamStubs(t, executorSeamStubs{
						processStacks: func(_ *schema.AtmosConfiguration, info schema.ConfigAndStacksInfo, _, _, _ bool, _ []string, _ auth.AuthManager) (schema.ConfigAndStacksInfo, error) {
							info.ComponentIsEnabled = true
							info.ComponentIsAbstract = metadata == "abstract"
							info.ComponentIsLocked = metadata == "locked"
							info.ComponentSection = map[string]any{"stack_name": "vpc", "path": "template.yaml"}
							return info, nil
						},
						setupComponentAuthForCLI: func(_ *schema.AtmosConfiguration, _ *schema.ConfigAndStacksInfo) (auth.AuthManager, error) {
							authCalls++
							return nil, authErr
						},
					})
					info := schema.ConfigAndStacksInfo{ComponentFromArg: "vpc", DryRun: dryRun}
					err := executeSingle(&component.ExecutionContext{}, &schema.AtmosConfiguration{}, &info, operation)
					rejected := metadata == "abstract" || metadata == "locked" && (operation == OperationApply || operation == OperationDelete)
					switch {
					case rejected:
						require.ErrorIs(t, err, errUtils.ErrInvalidSpecificAwsCloudFormationComponent)
						assert.Contains(t, err.Error(), "vpc")
						assert.Zero(t, authCalls)
					case dryRun:
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
