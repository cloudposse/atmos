package terraform

import (
	"bytes"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/cloudposse/atmos/cmd/internal"
	e "github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/pkg/ansi"
	"github.com/cloudposse/atmos/pkg/flags"
	h "github.com/cloudposse/atmos/pkg/hooks"
)

// planParser handles flag parsing for plan command.
var planParser *flags.StandardParser

// capturedPlanOutput holds the terraform plan stdout when CI mode is active.
// Written in RunE, read in PostRunE and the error-path defer.
var capturedPlanOutput string

// planCmd represents the terraform plan command.
var planCmd = &cobra.Command{
	Use:   "plan",
	Short: "Show changes required by the current configuration",
	Long: `Generate an execution plan, which shows what actions Terraform will take to reach the desired state of the configuration.

This command shows what Terraform will do when you run 'apply'. It helps you verify changes before making them to your infrastructure.

For complete Terraform/OpenTofu documentation, see:
  https://developer.hashicorp.com/terraform/cli/commands/plan
  https://opentofu.org/docs/cli/commands/plan`,
	PreRunE: func(cmd *cobra.Command, args []string) error {
		return runHooks(h.BeforeTerraformPlan, cmd, args)
	},
	RunE: func(cmd *cobra.Command, args []string) (runErr error) {
		// Reset captured output for this run.
		capturedPlanOutput = ""

		// On failure, run after hooks with error context so CI check runs
		// are updated to failure status. Cobra skips PostRunE on error.
		defer func() {
			if runErr != nil {
				runHooksOnErrorWithOutput(h.AfterTerraformPlan, cmd, args, runErr, capturedPlanOutput)
			}
		}()

		v := viper.GetViper()

		// Bind both parent and subcommand parsers.
		if err := terraformParser.BindFlagsToViper(cmd, v); err != nil {
			return err
		}
		if err := planParser.BindFlagsToViper(cmd, v); err != nil {
			return err
		}

		// Parse base terraform options.
		opts := ParseTerraformRunOptions(v)

		// Plan-specific flags (upload-status, skip-planfile) flow through the
		// legacy ProcessCommandLineArgs which sets info.PlanSkipPlanfile.
		// The Viper binding above ensures flag > env > config precedence works.

		// When CI mode is enabled, capture terraform plan stdout for CI hooks
		// (summary, comments, outputs). The output is tee'd: terminal still
		// receives it in real time, and the buffer collects a copy.
		ciMode, _ := cmd.Flags().GetBool("ci")
		if !ciMode {
			ciMode = v.GetBool("ci")
		}

		var shellOpts []e.ShellCommandOption
		var buf bytes.Buffer
		if ciMode {
			shellOpts = append(shellOpts, e.WithStdoutCapture(&buf))
		}

		err := terraformRunWithOptions(terraformCmd, cmd, args, opts, shellOpts...)

		// Strip ANSI escape codes so CI templates get clean text.
		if ciMode {
			capturedPlanOutput = ansi.Strip(buf.String())
		}

		return err
	},
	PostRunE: func(cmd *cobra.Command, args []string) error {
		return runHooksWithOutput(h.AfterTerraformPlan, cmd, args, capturedPlanOutput)
	},
}

func init() {
	// Create parser with plan-specific flags using functional options.
	planParser = flags.NewStandardParser(
		WithBackendExecutionFlags(),
		flags.WithBoolFlag("upload-status", "", false, "If set atmos will upload the plan result to the pro API"),
		flags.WithBoolFlag("affected", "", false, "Plan the affected components in dependency order"),
		flags.WithBoolFlag("all", "", false, "Plan all components in all stacks"),
		flags.WithBoolFlag("skip-planfile", "", false, "Skip writing the plan to a file by not passing the `-out` flag to Terraform when executing the command. Set it to true when using Terraform Cloud since the `-out` flag is not supported. Terraform Cloud automatically stores plans in its backend"),
		flags.WithBoolFlag("ci", "", false, "Enable CI mode for automated pipelines (writes job summary, outputs)"),
		flags.WithEnvVars("upload-status", "ATMOS_TERRAFORM_PLAN_UPLOAD_STATUS"),
		flags.WithEnvVars("skip-planfile", "ATMOS_TERRAFORM_PLAN_SKIP_PLANFILE"),
		flags.WithEnvVars("ci", "ATMOS_CI", "CI"),
	)

	// Register plan-specific flags with Cobra.
	planParser.RegisterFlags(planCmd)

	// Bind flags to Viper for environment variable support.
	if err := planParser.BindToViper(viper.GetViper()); err != nil {
		panic(err)
	}

	// Register completions for plan command.
	RegisterTerraformCompletions(planCmd)

	// Register compat flags for this subcommand.
	internal.RegisterCommandCompatFlags("terraform", "plan", PlanCompatFlags())

	// Attach to parent terraform command.
	terraformCmd.AddCommand(planCmd)
}
