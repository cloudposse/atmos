package cmd

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"

	"github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestDescribeDependents(t *testing.T) {
	_ = NewTestKit(t)

	// Reset Viper to clear any environment variable bindings from previous tests.
	// This prevents ATMOS_IDENTITY or IDENTITY env vars from interfering with the test.
	viper.Reset()

	// Clear identity environment variables to prevent Viper from reading them.
	// In CI, these might be set and cause auth validation to fail when no auth is configured.
	t.Setenv("ATMOS_IDENTITY", "")
	t.Setenv("IDENTITY", "")

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

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

	err := run(describeDependentsCmd, []string{"component"})

	// Verify command executed without errors. The mock expectations verify
	// that Execute() was called with the correct arguments.
	assert.NoError(t, err, "describeDependentsCmd should execute without error")
}

// TestDescribeDependentsSetsAuthDisabled mirrors TestDescribeAffectedSetsAuthDisabled
// for `describe dependents`. Before the wiring landed, `cmd/describe_dependents.go`
// computed `identityName = __DISABLED__` but never stored that signal anywhere the
// executor could read, so the inner per-component auth resolution still ran. CodeRabbit
// flagged this on PR #2471 (the `DescribeDependentsExecProps` struct was missing the
// `AuthDisabled` field). This test pins both ends: the cmd sets `props.AuthDisabled`,
// and the executor receives it.
func TestDescribeDependentsSetsAuthDisabled(t *testing.T) {
	tests := []struct {
		name             string
		envIdentity      string
		wantAuthDisabled bool
	}{
		{name: "identity=false sets AuthDisabled", envIdentity: "false", wantAuthDisabled: true},
		{name: "identity=off sets AuthDisabled", envIdentity: "off", wantAuthDisabled: true},
		{name: "identity=0 sets AuthDisabled", envIdentity: "0", wantAuthDisabled: true},
		{name: "identity=no sets AuthDisabled", envIdentity: "no", wantAuthDisabled: true},
		{name: "no identity flag does not set AuthDisabled", envIdentity: "", wantAuthDisabled: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_ = NewTestKit(t)

			viper.Reset()
			t.Setenv("ATMOS_IDENTITY", tc.envIdentity)
			t.Setenv("IDENTITY", "")
			if tc.envIdentity != "" {
				viper.Set("identity", tc.envIdentity)
			}

			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			var captured *exec.DescribeDependentsExecProps
			mock := exec.NewMockDescribeDependentsExec(ctrl)
			mock.EXPECT().Execute(gomock.Any()).DoAndReturn(func(props *exec.DescribeDependentsExecProps) error {
				captured = props
				return nil
			})

			run := getRunnableDescribeDependentsCmd(
				func(opts ...AtmosValidateOption) {},
				func(componentType string, cmd *cobra.Command, args, additionalArgsAndFlags []string) (schema.ConfigAndStacksInfo, error) {
					return schema.ConfigAndStacksInfo{}, nil
				},
				func(info schema.ConfigAndStacksInfo, validate bool) (schema.AtmosConfiguration, error) {
					return schema.AtmosConfiguration{}, nil
				},
				func(_ *schema.AtmosConfiguration) exec.DescribeDependentsExec { return mock },
			)

			err := run(describeDependentsCmd, []string{"component"})
			assert.NoError(t, err)
			assert.NotNil(t, captured, "Execute was not called with props")
			assert.Equal(t, tc.wantAuthDisabled, captured.AuthDisabled,
				"AuthDisabled should reflect the normalized identity flag value")
			if tc.wantAuthDisabled {
				assert.Nil(t, captured.AuthManager,
					"AuthManager must be nil when authentication is explicitly disabled")
			}
		})
	}
}

func TestSetFlagInDescribeDependents(t *testing.T) {
	_ = NewTestKit(t)

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
			_ = NewTestKit(t)

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
	_ = NewTestKit(t)

	stacksPath := "../tests/fixtures/scenarios/terraform-apply-affected"

	t.Setenv("ATMOS_CLI_CONFIG_PATH", stacksPath)
	t.Setenv("ATMOS_BASE_PATH", stacksPath)

	err := describeDependentsCmd.RunE(describeDependentsCmd, []string{"invalid-component"})
	assert.Error(t, err, "describe dependents command should return an error when called with invalid component")
}
