package cloudformation

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestApplySelectionFlagsDryRunPrecedence(t *testing.T) {
	for _, tt := range []struct {
		name string
		env  string
		args []string
		want bool
	}{
		{name: "environment true", env: "true", want: true},
		{name: "environment false", env: "false"},
		{name: "invalid environment", env: "invalid"},
		{name: "empty environment"},
		{name: "explicit false overrides environment", env: "true", args: []string{"--dry-run=false"}},
		{name: "explicit true overrides environment", env: "false", args: []string{"--dry-run"}, want: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("ATMOS_DRY_RUN", tt.env)
			viper.Reset()
			t.Cleanup(viper.Reset)
			viper.Set("dry-run", !tt.want)
			root := &cobra.Command{Use: "cloudformation"}
			flags.NewStandardParser(flags.WithDryRunFlag()).RegisterPersistentFlags(root)
			child := &cobra.Command{Use: "apply"}
			root.AddCommand(child)
			require.NoError(t, child.ParseFlags(tt.args))
			info := schema.ConfigAndStacksInfo{}
			applySelectionFlags(child, &info)
			require.Equal(t, tt.want, info.DryRun)
		})
	}
}
