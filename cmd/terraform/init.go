package terraform

import (
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/cloudposse/atmos/cmd/internal"
	"github.com/cloudposse/atmos/pkg/flags"
	h "github.com/cloudposse/atmos/pkg/hooks"
)

// initParser handles flag parsing for init command.
var initParser *flags.StandardParser

// initCmd represents the terraform init command.
var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Prepare your working directory for other commands",
	Long: `Initialize the working directory containing Terraform configuration files.

It will download necessary provider plugins and set up the backend.
Note: Atmos will automatically call init for you when running plan and apply commands.

For complete Terraform/OpenTofu documentation, see:
  https://developer.hashicorp.com/terraform/cli/commands/init
  https://opentofu.org/docs/cli/commands/init`,
	PreRunE: func(cmd *cobra.Command, args []string) error {
		return runHooks(h.BeforeTerraformInit, cmd, args)
	},
	RunE: func(cmd *cobra.Command, args []string) (runErr error) {
		// Reset the shared multi-component marker so PostRunE and the
		// deferred error hook read consistent state even on an early return.
		wasMultiComponentExecution = false

		// On failure, run after hooks with error context so CI check runs
		// are updated to failure status. Cobra skips PostRunE on error.
		defer func() {
			if runErr != nil && !wasMultiComponentExecution {
				runHooksOnErrorWithOutput(h.AfterTerraformInit, cmd, args, runErr, "")
			}
		}()

		v := viper.GetViper()

		// Bind both parent and subcommand parsers.
		if err := terraformParser.BindFlagsToViper(cmd, v); err != nil {
			return err
		}
		if err := initParser.BindFlagsToViper(cmd, v); err != nil {
			return err
		}

		// Parse base terraform options.
		opts, err := ParseTerraformRunOptions(v)
		if err != nil {
			return err
		}

		return terraformRunWithOptions(terraformCmd, cmd, args, opts)
	},
	PostRunE: func(cmd *cobra.Command, args []string) error {
		// In multi-component mode, per-component hooks already fired inside
		// the component walker; avoid a duplicate global call.
		if wasMultiComponentExecution {
			return nil
		}
		// init produces no plan/apply output to summarize, so no captured
		// output is passed.
		return runHooksWithOutput(h.AfterTerraformInit, cmd, args, "")
	},
}

func init() {
	// Create parser with init-specific flags.
	initParser = flags.NewStandardParser(
		WithBackendExecutionFlags(),
		flags.WithBoolFlag("affected", "", false, "Initialize the affected components in dependency order"),
		flags.WithBoolFlag("all", "", false, "Initialize all components in all stacks"),
		flags.WithIntFlag("max-concurrency", "", 1, "Maximum number of Terraform init components to execute concurrently"),
		flags.WithStringFlag("failure-mode", "", terraformFailureModeFailFast, "Terraform init failure handling mode. Supported values: fail-fast, keep-going"),
		flags.WithStringFlag("log-order", "", "stream", "Order concurrent Terraform init logs. Supported values: stream, grouped"),
		flags.WithEnvVars("affected", "ATMOS_TERRAFORM_INIT_AFFECTED"),
		flags.WithEnvVars("all", "ATMOS_TERRAFORM_INIT_ALL"),
		flags.WithEnvVars("max-concurrency", "ATMOS_TERRAFORM_INIT_MAX_CONCURRENCY"),
		flags.WithEnvVars("failure-mode", "ATMOS_TERRAFORM_INIT_FAILURE_MODE"),
		flags.WithEnvVars("log-order", "ATMOS_TERRAFORM_INIT_LOG_ORDER"),
	)

	// Register init-specific flags with Cobra.
	initParser.RegisterFlags(initCmd)

	// Bind flags to Viper for environment variable support.
	if err := initParser.BindToViper(viper.GetViper()); err != nil {
		panic(err)
	}

	// Register completions for initCmd.
	RegisterTerraformCompletions(initCmd)

	// Register compat flags for this subcommand.
	internal.RegisterCommandCompatFlags("terraform", "init", InitCompatFlags())

	// Attach to parent terraform command.
	terraformCmd.AddCommand(initCmd)
}
