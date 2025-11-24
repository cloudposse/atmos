package terraform

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
	// shellParser handles flag parsing for shell command.
	shellParser *flags.StandardParser
)

// shellCmd represents the terraform shell command (custom Atmos command).
var shellCmd = &cobra.Command{
	Use:   "shell <component>",
	Short: "Configure an environment for an Atmos component and start a new shell",
	Long: `Configure an environment for a specific Atmos component in a stack and then start a new shell.

In this shell, you can execute all native Terraform commands directly without the need
to use Atmos-specific arguments and flags. This allows you to interact with Terraform
as you would in a typical setup, but within the configured Atmos environment.`,
	Args:               cobra.ExactArgs(1),
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: false},
	RunE: func(cmd *cobra.Command, args []string) error {
		component := args[0]

		// Use Viper to respect precedence (flag > env > config > default)
		v := viper.GetViper()

		// Bind terraform flags (--stack, etc.) to Viper
		if err := terraformParser.BindFlagsToViper(cmd, v); err != nil {
			return err
		}

		// Bind shell-specific flags to Viper
		if err := shellParser.BindFlagsToViper(cmd, v); err != nil {
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

		return e.ExecuteTerraformShell(component, stack, processTemplates, processFunctions, skip, &atmosConfig)
	},
}

func init() {
	// Create parser with shell-specific flags using functional options.
	shellParser = flags.NewStandardParser(
		flags.WithBoolFlag("process-templates", "", true, "Enable Go template processing in Atmos stack manifests"),
		flags.WithBoolFlag("process-functions", "", true, "Enable YAML functions processing in Atmos stack manifests"),
		flags.WithStringSliceFlag("skip", "", []string{}, "Skip processing specific Atmos YAML functions"),
		flags.WithEnvVars("process-templates", "ATMOS_PROCESS_TEMPLATES"),
		flags.WithEnvVars("process-functions", "ATMOS_PROCESS_FUNCTIONS"),
		flags.WithEnvVars("skip", "ATMOS_SKIP"),
	)

	// Register flags with the command.
	shellParser.RegisterFlags(shellCmd)

	// Bind flags to Viper for environment variable support.
	if err := shellParser.BindToViper(viper.GetViper()); err != nil {
		panic(err)
	}

	// Register completions for shellCmd.
	RegisterTerraformCompletions(shellCmd)

	// Attach to parent terraform command.
	terraformCmd.AddCommand(shellCmd)
}
