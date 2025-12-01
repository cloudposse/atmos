package terraform

import (
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	e "github.com/cloudposse/atmos/internal/exec"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/schema"
)

// cleanParser handles flag parsing for clean command.
var cleanParser *flags.StandardParser

// cleanCmd represents the terraform clean command (custom Atmos command).
var cleanCmd = &cobra.Command{
	Use:   "clean <component>",
	Short: "Clean up Terraform state and artifacts",
	Long: `Remove temporary files, state locks, and other artifacts created during Terraform operations.

This helps reset the environment and ensures no leftover data interferes with subsequent runs.

Common use cases:
- Releasing locks on Terraform state files.
- Cleaning up temporary workspaces or plans.
- Preparing the environment for a fresh deployment.`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		// Get component from args (optional - if empty, cleans all components)
		var component string
		if len(args) > 0 {
			component = args[0]
		}

		// Use Viper to respect precedence (flag > env > config > default)
		v := viper.GetViper()

		// Bind terraform flags (--stack, etc.) to Viper
		if err := terraformParser.BindFlagsToViper(cmd, v); err != nil {
			return err
		}

		// Bind clean-specific flags to Viper
		if err := cleanParser.BindFlagsToViper(cmd, v); err != nil {
			return err
		}

		// Get flag values from Viper
		stack := v.GetString("stack")
		force := v.GetBool("force")
		everything := v.GetBool("everything")
		skipLockFile := v.GetBool("skip-lock-file")
		dryRun := v.GetBool("dry-run")

		// Initialize Atmos configuration
		atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, true)
		if err != nil {
			return err
		}

		opts := &e.CleanOptions{
			Component:    component,
			Stack:        stack,
			Force:        force,
			Everything:   everything,
			SkipLockFile: skipLockFile,
			DryRun:       dryRun,
		}
		return e.ExecuteClean(opts, &atmosConfig)
	},
}

func init() {
	// Create parser with clean-specific flags using functional options.
	cleanParser = flags.NewStandardParser(
		flags.WithBoolFlag("everything", "", false, "If set atmos will also delete the Terraform state files and directories for the component"),
		flags.WithBoolFlag("force", "f", false, "Forcefully delete Terraform state files and directories without interaction"),
		flags.WithBoolFlag("skip-lock-file", "", false, "Skip deleting the `.terraform.lock.hcl` file"),
		flags.WithEnvVars("everything", "ATMOS_TERRAFORM_CLEAN_EVERYTHING"),
		flags.WithEnvVars("force", "ATMOS_TERRAFORM_CLEAN_FORCE"),
		flags.WithEnvVars("skip-lock-file", "ATMOS_TERRAFORM_CLEAN_SKIP_LOCK_FILE"),
	)

	// Register flags with the command as persistent flags.
	cleanParser.RegisterPersistentFlags(cleanCmd)

	// Bind flags to Viper for environment variable support.
	if err := cleanParser.BindToViper(viper.GetViper()); err != nil {
		panic(err)
	}

	// Register completions for cleanCmd.
	RegisterTerraformCompletions(cleanCmd)

	// Attach to parent terraform command.
	terraformCmd.AddCommand(cleanCmd)
}
