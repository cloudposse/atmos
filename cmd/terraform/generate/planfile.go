package generate

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	cfg "github.com/cloudposse/atmos/pkg/config"
	e "github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/schema"
)

var (
	// planfileParser handles flag parsing for planfile command.
	planfileParser *flags.StandardParser
)

// planfileCmd generates planfile for a terraform component.
var planfileCmd = &cobra.Command{
	Use:                "planfile <component>",
	Short:              "Generate a planfile for a Terraform component",
	Long:               "This command generates a `planfile` for a specified Atmos Terraform component.",
	Args:               cobra.ExactArgs(1),
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: false},
	RunE: func(cmd *cobra.Command, args []string) error {
		component := args[0]

		// Use Viper to respect precedence (flag > env > config > default)
		v := viper.GetViper()

		// Bind planfile-specific flags to Viper
		if err := planfileParser.BindFlagsToViper(cmd, v); err != nil {
			return err
		}

		// Get flag values from Viper
		stack := v.GetString("stack")
		file := v.GetString("file")
		format := v.GetString("format")
		processTemplates := v.GetBool("process-templates")
		processFunctions := v.GetBool("process-functions")
		skip := v.GetStringSlice("skip")

		// Validate required flags
		if stack == "" {
			return fmt.Errorf("stack is required (use --stack or -s)")
		}

		// Initialize Atmos configuration
		atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, true)
		if err != nil {
			return err
		}

		return e.ExecuteGeneratePlanfile(component, stack, file, format, processTemplates, processFunctions, skip, &atmosConfig)
	},
}

func init() {
	// Create parser with planfile-specific flags using functional options.
	planfileParser = flags.NewStandardParser(
		flags.WithStringFlag("stack", "s", "", "Atmos stack (required)"),
		flags.WithStringFlag("file", "f", "", "Planfile name"),
		flags.WithStringFlag("format", "", "json", "Output format: json or yaml"),
		flags.WithBoolFlag("process-templates", "", true, "Enable Go template processing in Atmos stack manifests"),
		flags.WithBoolFlag("process-functions", "", true, "Enable YAML functions processing in Atmos stack manifests"),
		flags.WithStringSliceFlag("skip", "", []string{}, "Skip processing specific Atmos YAML functions"),
		flags.WithEnvVars("stack", "ATMOS_STACK"),
		flags.WithEnvVars("file", "ATMOS_FILE"),
		flags.WithEnvVars("format", "ATMOS_FORMAT"),
		flags.WithEnvVars("process-templates", "ATMOS_PROCESS_TEMPLATES"),
		flags.WithEnvVars("process-functions", "ATMOS_PROCESS_FUNCTIONS"),
		flags.WithEnvVars("skip", "ATMOS_SKIP"),
	)

	// Register flags with the command.
	planfileParser.RegisterFlags(planfileCmd)

	// Bind flags to Viper for environment variable support.
	if err := planfileParser.BindToViper(viper.GetViper()); err != nil {
		panic(err)
	}

	// Mark stack as required.
	if err := planfileCmd.MarkFlagRequired("stack"); err != nil {
		panic(err)
	}

	GenerateCmd.AddCommand(planfileCmd)
}
