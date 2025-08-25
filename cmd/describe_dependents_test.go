package cmd

import (
	"os"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/stretchr/testify/assert"

	"github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestDescribeDependents(t *testing.T) {
	ctrl := gomock.NewController(t)
	describeDependentsMock := exec.NewMockDescribeDependentsExec(ctrl)
	describeDependentsMock.EXPECT().Execute(gomock.Any()).Return(nil)
	run := getRunnableDescribeDependentsCmd(func(opts ...AtmosValidateOption) {},
		func(componentType string, cmd *cobra.Command, args, additionalArgsAndFlags []string) (schema.ConfigAndStacksInfo, error) {
			return schema.ConfigAndStacksInfo{}, nil
		},
		func(info schema.ConfigAndStacksInfo, validate bool) (schema.AtmosConfiguration, error) {
			return schema.AtmosConfiguration{}, nil
		},
		func(atmosConfig *schema.AtmosConfiguration) exec.DescribeDependentsExec {
			return describeDependentsMock
		})
	run(describeDependentsCmd, []string{"component"})
}

func TestSetFlagInDescribeDependents(t *testing.T) {
	// Initialize test cases
	tests := []struct {
		name        string
		setFlags    func(*pflag.FlagSet)
		expected    *exec.DescribeDependentsExecProps
		expectedErr bool
	}{
		{
			name: "Set string flags",
			setFlags: func(fs *pflag.FlagSet) {
				fs.Set("format", "yaml")
			},
			expected: &exec.DescribeDependentsExecProps{
				Format:               "yaml",
				ProcessTemplates:     true,
				ProcessYamlFunctions: true,
			},
		},
		{
			name: "No flags changed, set default format",
			setFlags: func(fs *pflag.FlagSet) {
				// No flags set
			},
			expected: &exec.DescribeDependentsExecProps{
				Format:               "json",
				ProcessTemplates:     true,
				ProcessYamlFunctions: true,
			},
		},
		{
			name: "Set invalid format, no override",
			setFlags: func(fs *pflag.FlagSet) {
				fs.Set("format", "invalid_format")
			},
			expectedErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fs := pflag.NewFlagSet("test", pflag.ContinueOnError)
			fs.StringP("format", "f", "yaml", "Specify the output format (`yaml` is default)")
			fs.StringP("output", "o", "list", "Specify the output type (`list` is default)")
			fs.StringP("query", "q", "", "Specify a query to filter the output")
			tt.setFlags(fs)
			describeDependentArgs := &exec.DescribeDependentsExecProps{}
			err := setFlagsForDescribeDependentsCmd(fs, describeDependentArgs)
			if tt.expectedErr {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)
			assert.Equal(t, tt.expected, describeDependentArgs)
		})
	}
}

func TestDescribeDependentsCmd_Error(t *testing.T) {
	stacksPath := "../tests/fixtures/scenarios/terraform-apply-affected"

	err := os.Setenv("ATMOS_CLI_CONFIG_PATH", stacksPath)
	assert.NoError(t, err, "Setting 'ATMOS_CLI_CONFIG_PATH' environment variable should execute without error")

	err = os.Setenv("ATMOS_BASE_PATH", stacksPath)
	assert.NoError(t, err, "Setting 'ATMOS_BASE_PATH' environment variable should execute without error")

	// Unset ENV variables after testing
	defer func() {
		os.Unsetenv("ATMOS_CLI_CONFIG_PATH")
		os.Unsetenv("ATMOS_BASE_PATH")
	}()

	err = describeDependentsCmd.RunE(describeDependentsCmd, []string{"invalid-component"})
	assert.Error(t, err, "describe dependents command should return an error when called with invalid component")
}
