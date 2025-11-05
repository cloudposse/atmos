package cmd

import (
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	e "github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/pkg/flags"
)

func newTerraformGenerateVarfilesParser() *flags.StandardParser {
	// Build parser with format flag and custom flags manually.
	options := []flags.Option{
		// Format flag with validation.
		flags.WithStringFlag("format", "f", "hcl", "Output format (valid: hcl, json, backend-config)"),
		flags.WithEnvVars("format", "ATMOS_FORMAT"),
		flags.WithValidValues("format", "hcl", "json", "backend-config"),

		// Custom flags specific to terraform generate varfiles.
		flags.WithRequiredStringFlag("file-template", "",
			"Template for generating backend configuration files, supporting absolute/relative paths and context tokens (e.g., {tenant}, {environment}, {component}). Subdirectories are created automatically. If not specified, files are written to corresponding Terraform component folders."),
		flags.WithEnvVars("file-template", "ATMOS_FILE_TEMPLATE"),

		flags.WithStringFlag("stacks", "", "",
			"Only process the specified stacks (comma-separated values), supporting top-level stack manifest paths or derived Atmos stack names"),
		flags.WithEnvVars("stacks", "ATMOS_STACKS"),

		flags.WithStringFlag("components", "", "",
			"Only generate the `.tfvar` files for the specified `atmos` components (use comma-separated values)."),
		flags.WithEnvVars("components", "ATMOS_COMPONENTS"),
	}

	return flags.NewStandardParser(options...)
}

var terraformGenerateVarfilesParser = newTerraformGenerateVarfilesParser()

// terraformGenerateVarfilesCmd generates varfiles for all terraform components in all stacks.
var terraformGenerateVarfilesCmd = &cobra.Command{
	Use:   "varfiles",
	Short: "Generate varfiles for all Terraform components in all stacks",
	Long:  "This command generates varfiles for all Atmos Terraform components across all stacks.",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Check Atmos configuration.
		checkAtmosConfig()

		err := e.ExecuteTerraformGenerateVarfilesCmd(cmd, args)
		return err
	},
}

func init() {
	// Register flags using builder pattern.
	terraformGenerateVarfilesParser.RegisterFlags(terraformGenerateVarfilesCmd)
	_ = terraformGenerateVarfilesParser.BindToViper(viper.GetViper())

	terraformGenerateCmd.AddCommand(terraformGenerateVarfilesCmd)
}
