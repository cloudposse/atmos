package generate

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	errUtils "github.com/cloudposse/atmos/errors"
	e "github.com/cloudposse/atmos/internal/exec"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/schema"
)

// backendsParser handles flag parsing for backends command.
var backendsParser *flags.StandardParser

// backendsCmd generates backend configs for all terraform components.
var backendsCmd = &cobra.Command{
	Use:                "backends",
	Short:              "Generate backend configurations for all Terraform components",
	Long:               "This command generates the backend configuration files for all Terraform components in the Atmos environment.",
	Args:               cobra.NoArgs,
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: false},
	RunE: func(cmd *cobra.Command, args []string) error {
		// Use Viper to respect precedence (flag > env > config > default)
		v := viper.GetViper()

		// Bind backends-specific flags to Viper
		if err := backendsParser.BindFlagsToViper(cmd, v); err != nil {
			return err
		}

		// Get flag values from Viper
		fileTemplate := v.GetString("file-template")
		stacksCsv := v.GetString("stacks")
		componentsCsv := v.GetString("components")
		format := v.GetString("format")

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
		if format != "" && format != "json" && format != "hcl" && format != "backend-config" {
			return fmt.Errorf("%w: '%s'. Valid values are 'hcl', 'json', and 'backend-config'", errUtils.ErrInvalidFlag, format)
		}
		if format == "" {
			format = "hcl"
		}

		// Initialize Atmos configuration
		atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, true)
		if err != nil {
			return err
		}

		return e.ExecuteTerraformGenerateBackends(&atmosConfig, fileTemplate, format, stacks, components)
	},
}

func init() {
	// Create parser with backends-specific flags using functional options.
	backendsParser = flags.NewStandardParser(
		flags.WithStringFlag("file-template", "", "", "Template for generating backend configuration files"),
		flags.WithStringFlag("stacks", "", "", "Only process the specified stacks (comma-separated)"),
		flags.WithStringFlag("components", "", "", "Only generate backend files for specified components (comma-separated)"),
		flags.WithStringFlag("format", "", "hcl", "Output format: hcl, json, or backend-config"),
		flags.WithEnvVars("file-template", "ATMOS_TERRAFORM_GENERATE_BACKENDS_FILE_TEMPLATE"),
		flags.WithEnvVars("stacks", "ATMOS_STACKS"),
		flags.WithEnvVars("components", "ATMOS_COMPONENTS"),
		flags.WithEnvVars("format", "ATMOS_FORMAT"),
	)

	// Register flags with the command.
	backendsParser.RegisterFlags(backendsCmd)

	// Bind flags to Viper for environment variable support.
	if err := backendsParser.BindToViper(viper.GetViper()); err != nil {
		panic(err)
	}

	GenerateCmd.AddCommand(backendsCmd)
}
