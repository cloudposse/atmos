package exec

import (
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"

	"github.com/cloudposse/atmos/pkg/pager"
	"github.com/cloudposse/atmos/pkg/schema"
)

// --- Test ---
func TestExecuteDescribeComponentCmd_Success_YAMLWithPager(t *testing.T) {
	ctrl := gomock.NewController(t)
	pagerMock := pager.NewMockPageCreator(ctrl)

	mockedExec := &DescribeComponentExec{
		pageCreator: pagerMock,
		printOrWriteToFile: func(atmosConfig *schema.AtmosConfiguration, format string, file string, data any) error {
			return nil
		},
		IsTTYSupportForStdout: func() bool {
			return true
		},
		executeDescribeComponent: func(component, stack string, processTemplates, processYamlFunctions bool, skip []string) (map[string]any, error) {
			return map[string]any{
				"component": component,
				"stack":     stack,
			}, nil
		},
		initCliConfig: func(configAndStacksInfo schema.ConfigAndStacksInfo, processStacks bool) (schema.AtmosConfiguration, error) {
			return schema.AtmosConfiguration{}, nil
		},
	}

	params := DescribeComponentParams{
		Component: "component-1",
		Stack:     "nonprod",
		Pager:     "less",
		Format:    "yaml",
	}
	pagerMock.EXPECT().Run(gomock.Any(), gomock.Any()).Return(nil).Times(1)
	// stub out internal deps
	err := mockedExec.ExecuteDescribeComponentCmd(params)
	assert.NoError(t, err)
}
