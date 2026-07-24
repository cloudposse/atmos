package terraform

import (
	"github.com/spf13/viper"

	"github.com/cloudposse/atmos/cmd/terraform/shared"
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
func ParseTerraformRunOptions(v *viper.Viper) (*TerraformRunOptions, error) {
	return shared.ParseRunOptions(v)
}

func terraformPlanHideContains(values []string, target string) bool {
	return shared.TerraformPlanHideContains(values, target)
}
