package terraform

import (
	"github.com/spf13/viper"

	"github.com/cloudposse/atmos/cmd/terraform/shared"
)

// TerraformRunOptions contains shared flags from terraformParser.
// Used by simple subcommands that only need the base terraform flags.
type TerraformRunOptions = shared.RunOptions

// ParseTerraformRunOptions parses shared terraform flags from Viper.
func ParseTerraformRunOptions(v *viper.Viper) *TerraformRunOptions {
	return shared.ParseRunOptions(v)
}
