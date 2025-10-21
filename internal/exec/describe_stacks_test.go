package exec

import (
	"errors"
	"os"
	"testing"

	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"

	"github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/pager"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

func TestDescribeStacksExec(t *testing.T) {
	testCases := []struct {
		name          string
		args          *DescribeStacksArgs
		setupMocks    func(ctrl *gomock.Controller) *describeStacksExec
		expectedError string
		expectedQuery string
	}{
		{
			name: "successful basic execution",
			args: &DescribeStacksArgs{},
			setupMocks: func(ctrl *gomock.Controller) *describeStacksExec {
				return &describeStacksExec{
					pageCreator:           pager.NewMockPageCreator(ctrl),
					isTTYSupportForStdout: func() bool { return false },
					printOrWriteToFile: func(_ *schema.AtmosConfiguration, _, _ string, _ any) error {
						return nil
					},
					executeDescribeStacks: func(_ *schema.AtmosConfiguration, _ string, _, _, _ []string, _, _, _, _ bool, _ []string) (map[string]any, error) {
						return map[string]any{"hello": "test"}, nil
					},
				}
			},
		},
		{
			name: "with query parameter",
			args: &DescribeStacksArgs{
				Query: ".hello",
			},
			setupMocks: func(ctrl *gomock.Controller) *describeStacksExec {
				return &describeStacksExec{
					pageCreator:           pager.NewMockPageCreator(ctrl),
					isTTYSupportForStdout: func() bool { return false },
					printOrWriteToFile: func(_ *schema.AtmosConfiguration, _, _ string, data any) error {
						assert.Equal(t, "test", data)
						return nil
					},
					executeDescribeStacks: func(_ *schema.AtmosConfiguration, _ string, _, _, _ []string, _, _, _, _ bool, _ []string) (map[string]any, error) {
						return map[string]any{"hello": "test"}, nil
					},
				}
			},
		},
		{
			name: "with filter by stack",
			args: &DescribeStacksArgs{
				FilterByStack: "test-stack",
			},
			setupMocks: func(ctrl *gomock.Controller) *describeStacksExec {
				return &describeStacksExec{
					pageCreator:           pager.NewMockPageCreator(ctrl),
					isTTYSupportForStdout: func() bool { return false },
					printOrWriteToFile: func(_ *schema.AtmosConfiguration, _, _ string, _ any) error {
						return nil
					},
					executeDescribeStacks: func(_ *schema.AtmosConfiguration, filterByStack string, _, _, _ []string, _, _, _, _ bool, _ []string) (map[string]any, error) {
						assert.Equal(t, "test-stack", filterByStack)
						return map[string]any{"filtered": true}, nil
					},
				}
			},
		},
		{
			name: "with file output",
			args: &DescribeStacksArgs{
				File: "output.json",
			},
			setupMocks: func(ctrl *gomock.Controller) *describeStacksExec {
				mockPageCreator := pager.NewMockPageCreator(ctrl)
				return &describeStacksExec{
					pageCreator:           mockPageCreator,
					isTTYSupportForStdout: func() bool { return true },
					printOrWriteToFile: func(_ *schema.AtmosConfiguration, format, file string, data any) error {
						assert.Equal(t, "output.json", file)
						return nil
					},
					executeDescribeStacks: func(_ *schema.AtmosConfiguration, _ string, _, _, _ []string, _, _, _, _ bool, _ []string) (map[string]any, error) {
						return map[string]any{"output": "to file"}, nil
					},
				}
			},
		},
		{
			name: "with execute error",
			args: &DescribeStacksArgs{},
			setupMocks: func(ctrl *gomock.Controller) *describeStacksExec {
				return &describeStacksExec{
					pageCreator:           pager.NewMockPageCreator(ctrl),
					isTTYSupportForStdout: func() bool { return false },
					executeDescribeStacks: func(_ *schema.AtmosConfiguration, _ string, _, _, _ []string, _, _, _, _ bool, _ []string) (map[string]any, error) {
						return nil, errors.New("execution error")
					},
				}
			},
			expectedError: "execution error",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			exec := tc.setupMocks(ctrl)
			err := exec.Execute(&schema.AtmosConfiguration{}, tc.args)

			if tc.expectedError != "" {
				assert.ErrorContains(t, err, tc.expectedError)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestExecuteDescribeStacks_Packer(t *testing.T) {
	err := os.Unsetenv("ATMOS_CLI_CONFIG_PATH")
	if err != nil {
		t.Fatalf("Failed to unset 'ATMOS_CLI_CONFIG_PATH': %v", err)
	}

	err = os.Unsetenv("ATMOS_BASE_PATH")
	if err != nil {
		t.Fatalf("Failed to unset 'ATMOS_BASE_PATH': %v", err)
	}

	log.SetLevel(log.InfoLevel)
	log.SetOutput(os.Stdout)

	// Define the working directory
	workDir := "../../tests/fixtures/scenarios/packer"
	t.Chdir(workDir)

	atmosConfig, err := config.InitCliConfig(schema.ConfigAndStacksInfo{}, true)
	assert.Nil(t, err)

	stacksMap, err := ExecuteDescribeStacks(
		&atmosConfig,
		"",
		nil,
		nil,
		nil,
		false,
		true,
		true,
		false,
		nil,
	)
	assert.Nil(t, err)

	val, err := u.EvaluateYqExpression(&atmosConfig, stacksMap, ".prod.components.packer.aws/bastion.vars.ami_tags.SourceAMI")
	assert.Nil(t, err)
	assert.Equal(t, "ami-0013ceeff668b979b", val)

	val, err = u.EvaluateYqExpression(&atmosConfig, stacksMap, ".nonprod.components.packer.aws/bastion.metadata.component")
	assert.Nil(t, err)
	assert.Equal(t, "aws/bastion", val)
}
