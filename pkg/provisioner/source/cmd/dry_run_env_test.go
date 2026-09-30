package cmd

import (
	"errors"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/cloudposse/atmos/pkg/schema"
)

func TestSourceDryRun_EnvironmentPrecedence(t *testing.T) {
	for _, verb := range []string{"pull", "delete"} {
		for _, tt := range []struct {
			name      string
			env       string
			cli       []string
			inherited bool
			dryRun    bool
		}{
			{name: "environment true", env: "true", inherited: true, dryRun: true},
			{name: "environment false", env: "false", inherited: true},
			{name: "invalid environment", env: "invalid", inherited: true},
			{name: "empty environment", inherited: true},
			{name: "explicit false", env: "true", cli: []string{"--dry-run=false"}, inherited: true},
			{name: "explicit true", env: "false", cli: []string{"--dry-run"}, inherited: true, dryRun: true},
			{name: "absent flag", env: "true"},
		} {
			t.Run(verb+"/"+tt.name, func(t *testing.T) {
				t.Setenv("ATMOS_DRY_RUN", tt.env)
				viper.Reset()
				t.Cleanup(viper.Reset)
				viper.Set("dry-run", !tt.dryRun)
				ctrl := gomock.NewController(t)
				loader := NewMockConfigLoader(ctrl)
				creator := NewMockAuthCreator(ctrl)
				provisioner := NewMockSourceProvisioner(ctrl)
				oldInit, oldCreate, oldProvision := initCliConfigFunc, createAuthFunc, provisionSourceFunc
				t.Cleanup(func() { initCliConfigFunc, createAuthFunc, provisionSourceFunc = oldInit, oldCreate, oldProvision })
				initCliConfigFunc, createAuthFunc, provisionSourceFunc = loader.InitCliConfig, creator.CreateAuthManager, provisioner.Provision
				configErr := errors.New("configuration reached")
				if !tt.dryRun {
					loader.EXPECT().InitCliConfig(gomock.Any(), gomock.Any()).Return(schema.AtmosConfiguration{}, configErr)
				}
				root := mountSourceCommand(t, verb, tt.inherited)
				root.SetArgs(append([]string{"source", verb, "vpc", "--stack", "dev"}, tt.cli...))
				err := root.Execute()
				if tt.dryRun {
					require.NoError(t, err)
				} else {
					require.ErrorIs(t, err, configErr)
				}
			})
		}
	}
}
