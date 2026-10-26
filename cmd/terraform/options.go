package terraform

import (
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/cloudposse/atmos/cmd/terraform/shared"
	"github.com/cloudposse/atmos/pkg/perf"
)

const (
	terraformFailureModeFailFast  = shared.TerraformFailureModeFailFast
	terraformFailureModeKeepGoing = shared.TerraformFailureModeKeepGoing
	terraformLogOrderStream       = shared.TerraformLogOrderStream
	terraformLogOrderGrouped      = shared.TerraformLogOrderGrouped
)

// TerraformRunOptions contains shared flags from terraformParser.
// Used by simple subcommands that only need the base terraform flags.
type TerraformRunOptions = shared.RunOptions

// ParseTerraformRunOptions parses and validates shared terraform flags from Viper.
// Pass cmd to also detect if --ui flag was explicitly set (for tri-state logic:
// unset vs true vs false); omit it (or pass nil) when UI tri-state detection
// isn't needed.
func ParseTerraformRunOptions(v *viper.Viper, cmd ...*cobra.Command) (*TerraformRunOptions, error) {
	defer perf.Track(nil, "terraform.ParseTerraformRunOptions")()

	opts, err := shared.ParseRunOptions(v)
	if err != nil {
		return nil, err
	}

	if len(cmd) > 0 && cmd[0] != nil && cmd[0].Flags().Changed("ui") {
		opts.UIFlagSet = true
	}

	return opts, nil
}
