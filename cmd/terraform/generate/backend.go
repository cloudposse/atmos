package generate

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	e "github.com/cloudposse/atmos/internal/exec"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/schema"
)

// backendParser handles flag parsing for backend command.
var backendParser *flags.StandardParser

// backendCmd generates backend config for a terraform component.
var backendCmd = &cobra.Command{
	Use:                "backend <component>",
	Short:              "Generate backend configuration for a Terraform component",
	Long:               `This command generates the backend configuration for a Terraform component using the specified stack`,
	Args:               cobra.ExactArgs(1),
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: false},
	RunE: func(cmd *cobra.Command, args []string) error {
		component := args[0]

		// Use Viper to respect precedence (flag > env > config > default)
		v := viper.GetViper()

		// Bind backend-specific flags to Viper
		if err := backendParser.BindFlagsToViper(cmd, v); err != nil {
			return err
		}

		// Get flag values from Viper
		stack := v.GetString("stack")
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

		return e.ExecuteGenerateBackend(component, stack, processTemplates, processFunctions, skip, &atmosConfig)
	},
}

func init() {
	// Create parser with backend-specific flags using functional options.
	backendParser = flags.NewStandardParser(
		flags.WithStringFlag("stack", "s", "", "Atmos stack (required)"),
		flags.WithBoolFlag("process-templates", "", true, "Enable Go template processing in Atmos stack manifests"),
		flags.WithBoolFlag("process-functions", "", true, "Enable YAML functions processing in Atmos stack manifests"),
		flags.WithStringSliceFlag("skip", "", []string{}, "Skip processing specific Atmos YAML functions"),
		flags.WithEnvVars("stack", "ATMOS_STACK"),
		flags.WithEnvVars("process-templates", "ATMOS_PROCESS_TEMPLATES"),
		flags.WithEnvVars("process-functions", "ATMOS_PROCESS_FUNCTIONS"),
		flags.WithEnvVars("skip", "ATMOS_SKIP"),
	)

	// Register flags with the command.
	backendParser.RegisterFlags(backendCmd)

	// Bind flags to Viper for environment variable support.
	if err := backendParser.BindToViper(viper.GetViper()); err != nil {
		panic(err)
	}

	// Mark stack as required.
	if err := backendCmd.MarkFlagRequired("stack"); err != nil {
		panic(err)
	}

	GenerateCmd.AddCommand(backendCmd)
}
