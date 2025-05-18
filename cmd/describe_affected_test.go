package cmd

import (
	"testing"

	"github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/golang/mock/gomock"
)

func TestDescribeAffected(t *testing.T) {
	t.Chdir("../tests/fixtures/scenarios/basic")
	ctrl := gomock.NewController(t)
	describeAffectedMock := exec.NewMockDescribeAffectedExec(ctrl)
	describeAffectedMock.EXPECT().Execute(gomock.Any()).Return(nil)
	run := getRunnableDescribeAffectedCmd(func(opts ...AtmosValidateOption) {
	}, parseDescribeAffectedCliArgs, func(atmosConfig *schema.AtmosConfiguration) exec.DescribeAffectedExec {
		return describeAffectedMock
	})
	run(describeAffectedCmd, []string{})
}
