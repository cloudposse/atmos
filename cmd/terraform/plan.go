package terraform

import (
	"bytes"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/cloudposse/atmos/cmd/internal"
	e "github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/pkg/ansi"
	"github.com/cloudposse/atmos/pkg/ci"
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
		// Reset per-run globals. Both must be initialised before any early return
		// so that the deferred hook and PostRunE read consistent state even when
		// terraformRunWithOptions exits before reaching its routing block.
		capturedPlanOutput = ""
		wasMultiComponentExecution = false

		// On failure, run after hooks with error context so CI check runs
		// are updated to failure status. Cobra skips PostRunE on error.
		// In multi-component mode the per-component hook already fired for each
		// component, so the global error call is suppressed to avoid double-firing.
		defer func() {
			if runErr != nil && !wasMultiComponentExecution {
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
		// CI mode is active when:
		// 1. --ci flag or ATMOS_CI/CI env var is set, OR
		// 2. A CI platform is auto-detected (e.g., GITHUB_ACTIONS=true).
		ciMode, _ := cmd.Flags().GetBool("ci")
		if !ciMode {
			ciMode = v.GetBool("ci")
		}
		if !ciMode {
			ciMode = ci.IsCI()
		}

		var shellOpts []e.ShellCommandOption
		var stdoutBuf, stderrBuf bytes.Buffer
		if ciMode {
			shellOpts = append(shellOpts, e.WithStdoutCapture(&stdoutBuf))
			shellOpts = append(shellOpts, e.WithStderrCapture(&stderrBuf))
		}

		err := terraformRunWithOptions(terraformCmd, cmd, args, opts, shellOpts...)

		// Strip ANSI escape codes so CI templates get clean text.
		// Combine stdout and stderr so that error messages (which terraform
		// writes to stderr) are available to the CI summary parser.
		if ciMode {
			combined := stdoutBuf.String()
			if errOut := stderrBuf.String(); errOut != "" {
				combined = combined + "\n" + errOut
			}
			capturedPlanOutput = ansi.Strip(combined)
		}

		return err
	},
	PostRunE: func(cmd *cobra.Command, args []string) error {
		// In multi-component mode, CI hooks already fired per-component inside
		// ExecuteTerraformQuery. Calling them again here would either double-fire
		// on the last component or fire with empty/misattributed output.
		// User-defined hooks (hooks.RunAll) are not per-component-aware today, so
		// suppress the entire PostRunE block to avoid confusion until that is fixed.
		if wasMultiComponentExecution {
			return nil
		}
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
		flags.WithIntFlag("max-concurrency", "", 1, "Maximum number of Terraform plan components to execute concurrently"),
		flags.WithStringFlag("failure-mode", "", terraformFailureModeFailFast, "Terraform plan failure handling mode. Supported values: fail-fast, keep-going"),
		flags.WithStringFlag("log-order", "", "stream", "Order concurrent Terraform plan logs. Supported values: stream, grouped"),
		flags.WithStringSliceFlag("hide", "", nil, "Hide Terraform plan output sections. Supported values: no-changes"),
		flags.WithStringFlag("execution-summary-file", "", "", "Write graph-backed Terraform plan execution summary JSON to the specified file"),
		flags.WithBoolFlag("skip-planfile", "", false, "Skip writing the plan to a file by not passing the `-out` flag to Terraform when executing the command. Set it to true when using Terraform Cloud since the `-out` flag is not supported. Terraform Cloud automatically stores plans in its backend"),
		flags.WithBoolFlag("ci", "", false, "Enable CI mode for automated pipelines (writes job summary, outputs)"),
		flags.WithEnvVars("upload-status", "ATMOS_TERRAFORM_PLAN_UPLOAD_STATUS"),
		flags.WithEnvVars("max-concurrency", "ATMOS_TERRAFORM_PLAN_MAX_CONCURRENCY"),
		flags.WithEnvVars("failure-mode", "ATMOS_TERRAFORM_PLAN_FAILURE_MODE"),
		flags.WithEnvVars("log-order", "ATMOS_TERRAFORM_PLAN_LOG_ORDER"),
		flags.WithEnvVars("hide", "ATMOS_TERRAFORM_PLAN_HIDE"),
		flags.WithEnvVars("execution-summary-file", "ATMOS_TERRAFORM_PLAN_EXECUTION_SUMMARY_FILE"),
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
