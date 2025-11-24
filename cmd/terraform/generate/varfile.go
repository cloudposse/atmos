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
	// varfileParser handles flag parsing for varfile command.
	varfileParser *flags.StandardParser
)

// varfileCmd generates varfile for a terraform component.
var varfileCmd = &cobra.Command{
	Use:                "varfile <component>",
	Short:              "Generate a varfile for a Terraform component",
	Long:               "This command generates a `varfile` for a specified Atmos Terraform component.",
	Args:               cobra.ExactArgs(1),
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: false},
	RunE: func(cmd *cobra.Command, args []string) error {
		component := args[0]

		// Use Viper to respect precedence (flag > env > config > default)
		v := viper.GetViper()

		// Bind varfile-specific flags to Viper
		if err := varfileParser.BindFlagsToViper(cmd, v); err != nil {
			return err
		}

		// Get flag values from Viper
		stack := v.GetString("stack")
		file := v.GetString("file")
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

		return e.ExecuteGenerateVarfile(component, stack, file, processTemplates, processFunctions, skip, &atmosConfig)
	},
}

func init() {
	// Create parser with varfile-specific flags using functional options.
	varfileParser = flags.NewStandardParser(
		flags.WithStringFlag("stack", "s", "", "Atmos stack (required)"),
		flags.WithStringFlag("file", "f", "", "Path to the varfile to generate"),
		flags.WithBoolFlag("process-templates", "", true, "Enable Go template processing in Atmos stack manifests"),
		flags.WithBoolFlag("process-functions", "", true, "Enable YAML functions processing in Atmos stack manifests"),
		flags.WithStringSliceFlag("skip", "", []string{}, "Skip processing specific Atmos YAML functions"),
		flags.WithEnvVars("stack", "ATMOS_STACK"),
		flags.WithEnvVars("file", "ATMOS_FILE"),
		flags.WithEnvVars("process-templates", "ATMOS_PROCESS_TEMPLATES"),
		flags.WithEnvVars("process-functions", "ATMOS_PROCESS_FUNCTIONS"),
		flags.WithEnvVars("skip", "ATMOS_SKIP"),
	)

	// Register flags with the command.
	varfileParser.RegisterFlags(varfileCmd)

	// Bind flags to Viper for environment variable support.
	if err := varfileParser.BindToViper(viper.GetViper()); err != nil {
		panic(err)
	}

	// Mark stack as required.
	if err := varfileCmd.MarkFlagRequired("stack"); err != nil {
		panic(err)
	}

	GenerateCmd.AddCommand(varfileCmd)
}
