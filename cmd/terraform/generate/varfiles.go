package generate

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	cfg "github.com/cloudposse/atmos/pkg/config"
	e "github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/schema"
)

var (
	// varfilesParser handles flag parsing for varfiles command.
	varfilesParser *flags.StandardParser
)

// varfilesCmd generates varfiles for all terraform components in all stacks.
var varfilesCmd = &cobra.Command{
	Use:                "varfiles",
	Short:              "Generate varfiles for all Terraform components in all stacks",
	Long:               "This command generates varfiles for all Atmos Terraform components across all stacks.",
	Args:               cobra.NoArgs,
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: false},
	RunE: func(cmd *cobra.Command, args []string) error {
		// Use Viper to respect precedence (flag > env > config > default)
		v := viper.GetViper()

		// Bind varfiles-specific flags to Viper
		if err := varfilesParser.BindFlagsToViper(cmd, v); err != nil {
			return err
		}

		// Get flag values from Viper
		fileTemplate := v.GetString("file-template")
		stacksCsv := v.GetString("stacks")
		componentsCsv := v.GetString("components")
		format := v.GetString("format")

		// Validate required flags
		if fileTemplate == "" {
			return fmt.Errorf("file-template is required (use --file-template)")
		}

		// Parse CSV values
		var stacks []string
		if stacksCsv != "" {
			stacks = strings.Split(stacksCsv, ",")
		}

		var components []string
		if componentsCsv != "" {
			components = strings.Split(componentsCsv, ",")
		}

		// Validate format
		if format != "" && format != "json" && format != "hcl" {
			return fmt.Errorf("invalid '--format' argument '%s'. Valid values are 'hcl' and 'json'", format)
		}
		if format == "" {
			format = "hcl"
		}

		// Initialize Atmos configuration
		atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, true)
		if err != nil {
			return err
		}

		return e.ExecuteTerraformGenerateVarfiles(&atmosConfig, fileTemplate, format, stacks, components)
	},
}

func init() {
	// Create parser with varfiles-specific flags using functional options.
	varfilesParser = flags.NewStandardParser(
		flags.WithStringFlag("file-template", "", "", "Template for generating varfile files (required)"),
		flags.WithStringFlag("stacks", "", "", "Only process the specified stacks (comma-separated)"),
		flags.WithStringFlag("components", "", "", "Only generate varfiles for specified components (comma-separated)"),
		flags.WithStringFlag("format", "", "hcl", "Output format: hcl or json"),
		flags.WithEnvVars("file-template", "ATMOS_TERRAFORM_GENERATE_VARFILES_FILE_TEMPLATE"),
		flags.WithEnvVars("stacks", "ATMOS_STACKS"),
		flags.WithEnvVars("components", "ATMOS_COMPONENTS"),
		flags.WithEnvVars("format", "ATMOS_FORMAT"),
	)

	// Register flags with the command.
	varfilesParser.RegisterFlags(varfilesCmd)

	// Bind flags to Viper for environment variable support.
	if err := varfilesParser.BindToViper(viper.GetViper()); err != nil {
		panic(err)
	}

	// Mark file-template as required.
	if err := varfilesCmd.MarkFlagRequired("file-template"); err != nil {
		panic(err)
	}

	GenerateCmd.AddCommand(varfilesCmd)
}
