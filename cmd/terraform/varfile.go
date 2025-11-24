package terraform

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	e "github.com/cloudposse/atmos/internal/exec"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui"
)

var (
	// varfileParser handles flag parsing for varfile command.
	varfileParser *flags.StandardParser
	// writeVarfileParser handles flag parsing for write varfile command.
	writeVarfileParser *flags.StandardParser
)

// varfileCmd represents the terraform varfile command (legacy Atmos command).
// Deprecated: Use 'terraform generate varfile' instead.
var varfileCmd = &cobra.Command{
	Use:                "varfile <component>",
	Short:              "Generate a varfile for a Terraform component (deprecated)",
	Long:               `Generate a varfile for a Terraform component. This command is deprecated in favor of 'terraform generate varfile'.`,
	Args:               cobra.ExactArgs(1),
	Deprecated:         "use 'atmos terraform generate varfile' instead",
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: false},
	RunE: func(cmd *cobra.Command, args []string) error {
		component := args[0]

		ui.Warning("'terraform varfile' is deprecated, use 'terraform generate varfile' instead")

		// Use Viper to respect precedence (flag > env > config > default)
		v := viper.GetViper()

		// Bind terraform flags (--stack, etc.) to Viper
		if err := terraformParser.BindFlagsToViper(cmd, v); err != nil {
			return err
		}

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

// writeVarfileCmd represents the terraform write varfile command (legacy Atmos command).
// Deprecated: Use 'terraform generate varfile' instead.
var writeVarfileCmd = &cobra.Command{
	Use:                "write varfile <component>",
	Short:              "Generate a varfile for a Terraform component (deprecated)",
	Long:               `Generate a varfile for a Terraform component. This command is deprecated in favor of 'terraform generate varfile'.`,
	Args:               cobra.ExactArgs(1),
	Deprecated:         "use 'atmos terraform generate varfile' instead",
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: false},
	RunE: func(cmd *cobra.Command, args []string) error {
		component := args[0]

		ui.Warning("'terraform write varfile' is deprecated, use 'terraform generate varfile' instead")

		// Use Viper to respect precedence (flag > env > config > default)
		v := viper.GetViper()

		// Bind terraform flags (--stack, etc.) to Viper
		if err := terraformParser.BindFlagsToViper(cmd, v); err != nil {
			return err
		}

		// Bind write varfile-specific flags to Viper
		if err := writeVarfileParser.BindFlagsToViper(cmd, v); err != nil {
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
		flags.WithStringFlag("file", "f", "", "Path to the varfile to generate"),
		flags.WithBoolFlag("process-templates", "", true, "Enable Go template processing in Atmos stack manifests"),
		flags.WithBoolFlag("process-functions", "", true, "Enable YAML functions processing in Atmos stack manifests"),
		flags.WithStringSliceFlag("skip", "", []string{}, "Skip processing specific Atmos YAML functions"),
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

	// Create parser with write varfile-specific flags using functional options.
	writeVarfileParser = flags.NewStandardParser(
		flags.WithStringFlag("file", "f", "", "Path to the varfile to generate"),
		flags.WithBoolFlag("process-templates", "", true, "Enable Go template processing in Atmos stack manifests"),
		flags.WithBoolFlag("process-functions", "", true, "Enable YAML functions processing in Atmos stack manifests"),
		flags.WithStringSliceFlag("skip", "", []string{}, "Skip processing specific Atmos YAML functions"),
		flags.WithEnvVars("file", "ATMOS_FILE"),
		flags.WithEnvVars("process-templates", "ATMOS_PROCESS_TEMPLATES"),
		flags.WithEnvVars("process-functions", "ATMOS_PROCESS_FUNCTIONS"),
		flags.WithEnvVars("skip", "ATMOS_SKIP"),
	)

	// Register flags with the command.
	writeVarfileParser.RegisterFlags(writeVarfileCmd)

	// Bind flags to Viper for environment variable support.
	if err := writeVarfileParser.BindToViper(viper.GetViper()); err != nil {
		panic(err)
	}

	// Register completions for varfileCmd.
	RegisterTerraformCompletions(varfileCmd)
	RegisterTerraformCompletions(writeVarfileCmd)

	// Attach to parent terraform command.
	terraformCmd.AddCommand(varfileCmd)
	terraformCmd.AddCommand(writeVarfileCmd)
}
