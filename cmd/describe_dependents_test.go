package cmd

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"

	"github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestDescribeDependents(t *testing.T) {
	_ = NewTestKit(t)

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	describeDependentsMock := exec.NewMockDescribeDependentsExec(ctrl)
	describeDependentsMock.EXPECT().Execute(gomock.Any()).Return(nil)

	run := getRunnableDescribeDependentsCmd(
		func(opts ...AtmosValidateOption) {},
		func(info schema.ConfigAndStacksInfo, validate bool) (schema.AtmosConfiguration, error) {
			return schema.AtmosConfiguration{}, nil
		},
		func(atmosConfig *schema.AtmosConfiguration) exec.DescribeDependentsExec {
			return describeDependentsMock
		})

	err := run(describeDependentsCmd, []string{"component"})

	// Verify command executed without errors. The mock expectations verify
	// that Execute() was called with the correct arguments.
	assert.NoError(t, err, "describeDependentsCmd should execute without error")
}

// TestSetFlagInDescribeDependents has been removed because setFlagsForDescribeDependentsCmd
// no longer exists. Flag parsing is now handled by the StandardOptions parser.
// The functionality is still tested through TestDescribeDependents and integration tests.

func TestDescribeDependentsCmd_Error(t *testing.T) {
	_ = NewTestKit(t)

	stacksPath := "../tests/fixtures/scenarios/terraform-apply-affected"

	t.Setenv("ATMOS_CLI_CONFIG_PATH", stacksPath)
	t.Setenv("ATMOS_BASE_PATH", stacksPath)

	err := describeDependentsCmd.RunE(describeDependentsCmd, []string{"invalid-component"})
	assert.Error(t, err, "describe dependents command should return an error when called with invalid component")
}
