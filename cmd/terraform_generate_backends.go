package cmd

import (
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	e "github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/pkg/flags"
)

func newTerraformGenerateBackendsParser() *flags.StandardParser {
	// Build parser with format flag from builder, then add custom flags manually.
	options := []flags.Option{
		// Format flag with validation.
		flags.WithStringFlag("format", "f", "hcl", "Output format (valid: hcl, json, backend-config)"),
		flags.WithEnvVars("format", "ATMOS_FORMAT"),
		flags.WithValidValues("format", "hcl", "json", "backend-config"),

		// Custom flags specific to terraform generate backends.
		flags.WithStringFlag("file-template", "", "",
			"Template for generating backend configuration files, supporting absolute/relative paths and context tokens (e.g., {tenant}, {environment}, {component}). Subdirectories are created automatically. If not specified, files are written to corresponding Terraform component folders."),
		flags.WithEnvVars("file-template", "ATMOS_FILE_TEMPLATE"),

		flags.WithStringFlag("stacks", "", "",
			"Only process the specified stacks (comma-separated values), supporting top-level stack manifest paths or derived Atmos stack names"),
		flags.WithEnvVars("stacks", "ATMOS_STACKS"),

		flags.WithStringFlag("components", "", "",
			"Only generate the backend files for the specified `atmos` components (comma-separated values)."),
		flags.WithEnvVars("components", "ATMOS_COMPONENTS"),
	}

	return flags.NewStandardParser(options...)
}

var terraformGenerateBackendsParser = newTerraformGenerateBackendsParser()

// terraformGenerateBackendsCmd generates backend configs for all terraform components.
var terraformGenerateBackendsCmd = &cobra.Command{
	Use:   "backends",
	Short: "Generate backend configurations for all Terraform components",
	Long:  "This command generates the backend configuration files for all Terraform components in the Atmos environment.",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Check Atmos configuration.
		checkAtmosConfig()

		err := e.ExecuteTerraformGenerateBackendsCmd(cmd, args)
		return err
	},
}

func init() {
	// Register flags using builder pattern.
	terraformGenerateBackendsParser.RegisterFlags(terraformGenerateBackendsCmd)
	_ = terraformGenerateBackendsParser.BindToViper(viper.GetViper())

	terraformGenerateCmd.AddCommand(terraformGenerateBackendsCmd)
}
