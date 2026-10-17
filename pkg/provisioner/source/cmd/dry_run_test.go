package cmd

import (
	"errors"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/schema"
)

func mountSourceCommand(t *testing.T, verb string, inherited bool) *cobra.Command {
	t.Helper()
	root := &cobra.Command{Use: "cloudformation", SilenceErrors: true, SilenceUsage: true}
	if inherited {
		flags.NewStandardParser(flags.WithDryRunFlag()).RegisterPersistentFlags(root)
	}
	group := &cobra.Command{Use: "source"}
	cfg := &Config{ComponentType: "aws/cloudformation", TypeLabel: "CloudFormation"}
	if verb == "pull" {
		group.AddCommand(PullCommand(cfg))
	} else {
		group.AddCommand(DeleteCommand(cfg))
	}
	root.AddCommand(group)
	return root
}

func TestSourceDryRun_ValidationBeforeSideEffects(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want error
	}{
		{name: "valid", args: []string{"vpc", "--stack", "dev"}},
		{name: "empty component", args: []string{"", "--stack", "dev"}, want: errUtils.ErrInvalidPositionalArgs},
		{name: "missing stack", args: []string{"vpc"}, want: errUtils.ErrRequiredFlagNotProvided},
	}
	for _, verb := range []string{"pull", "delete"} {
		for _, tt := range cases {
			t.Run(verb+"/"+tt.name, func(t *testing.T) {
				viper.Reset()
				t.Cleanup(viper.Reset)
				viper.Set("interactive", false)
				ctrl := gomock.NewController(t)
				loader := NewMockConfigLoader(ctrl)
				creator := NewMockAuthCreator(ctrl)
				provisioner := NewMockSourceProvisioner(ctrl)
				oldInit, oldCreate, oldProvision := initCliConfigFunc, createAuthFunc, provisionSourceFunc
				t.Cleanup(func() { initCliConfigFunc, createAuthFunc, provisionSourceFunc = oldInit, oldCreate, oldProvision })
				initCliConfigFunc, createAuthFunc, provisionSourceFunc = loader.InitCliConfig, creator.CreateAuthManager, provisioner.Provision
				// No expectations: configuration, authentication and provisioning must not run.
				cmd := mountSourceCommand(t, verb, true)
				cmd.SetArgs(append([]string{"source", verb, "--dry-run"}, tt.args...))
				err := cmd.Execute()
				if tt.want != nil {
					require.ErrorIs(t, err, tt.want)
				} else {
					require.NoError(t, err)
				}
			})
		}
	}
}

func TestSourceDryRun_DoesNotLeakFromViper(t *testing.T) {
	configErr := errors.New("configuration reached")
	for _, verb := range []string{"pull", "delete"} {
		for _, inherited := range []bool{false, true} {
			t.Run(verb+"/"+map[bool]string{false: "absent flag", true: "explicit false"}[inherited], func(t *testing.T) {
				viper.Reset()
				t.Cleanup(viper.Reset)
				viper.Set("dry-run", true)
				loader := NewMockConfigLoader(gomock.NewController(t))
				loader.EXPECT().InitCliConfig(gomock.Any(), gomock.Any()).Return(schema.AtmosConfiguration{}, configErr)
				old := initCliConfigFunc
				t.Cleanup(func() { initCliConfigFunc = old })
				initCliConfigFunc = loader.InitCliConfig
				cmd := mountSourceCommand(t, verb, inherited)
				args := []string{"source", verb, "vpc", "--stack", "dev"}
				if inherited {
					args = append(args, "--dry-run=false")
				}
				cmd.SetArgs(args)
				require.ErrorIs(t, cmd.Execute(), configErr)
			})
		}
	}
}
