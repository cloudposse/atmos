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

	mockedExec := &DescribeComponentExec{
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

	tests := []struct {
		name          string
		params        DescribeComponentParams
		expectedError bool
	}{
		{
			name: "Test pager with YAML format",
			params: DescribeComponentParams{
				Component: "component-1",
				Stack:     "nonprod",
				Pager:     "less",
				Format:    "yaml",
			},
		},
		{
			name: "Test pager with JSON format",
			params: DescribeComponentParams{
				Component: "component-1",
				Stack:     "nonprod",
				Pager:     "less",
				Format:    "json",
			},
		},
		{
			name: "Test no pager with YAML format",
			params: DescribeComponentParams{
				Component: "component-1",
				Stack:     "nonprod",
				Pager:     "more",
				Format:    "yaml",
			},
		},
		{
			name: "Test no pager with JSON format",
			params: DescribeComponentParams{
				Component: "component-1",
				Stack:     "nonprod",
				Pager:     "more",
				Format:    "json",
			},
		},
		{
			name: "Test invalid format",
			params: DescribeComponentParams{
				Component: "component-1",
				Stack:     "nonprod",
				Pager:     "less",
				Format:    "invalid-format",
			},
			expectedError: true,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if test.params.Pager == "less" && !test.expectedError {
				ctrl := gomock.NewController(t)
				pagerMock := pager.NewMockPageCreator(ctrl)
				pagerMock.EXPECT().Run(gomock.Any(), gomock.Any()).Return(nil).Times(1)
				mockedExec.pageCreator = pagerMock
			} else {
				mockedExec.printOrWriteToFile = func(atmosConfig *schema.AtmosConfiguration, format string, file string, data any) error {
					assert.Equal(t, test.params.Format, format)
					assert.Equal(t, "", file)
					assert.Equal(t, map[string]any{
						"component": "component-1",
						"stack":     "nonprod",
					}, data)
					return nil
				}
			}
			// stub out internal deps
			err := mockedExec.ExecuteDescribeComponentCmd(test.params)
			if test.expectedError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}

		})
	}
}
