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

// applyParser handles flag parsing for apply command.
var applyParser *flags.StandardParser

// capturedApplyOutput holds the terraform apply stdout when CI mode is active.
// Written in RunE, read in PostRunE and the error-path defer.
var capturedApplyOutput string

// applyCmd represents the terraform apply command.
var applyCmd = &cobra.Command{
	Use:   "apply",
	Short: "Apply changes to infrastructure",
	Long: `Apply the changes required to reach the desired state of the configuration.

This will prompt for confirmation before making changes unless -auto-approve is used.

For complete Terraform/OpenTofu documentation, see:
  https://developer.hashicorp.com/terraform/cli/commands/apply
  https://opentofu.org/docs/cli/commands/apply`,
	PreRunE: func(cmd *cobra.Command, args []string) error {
		return runHooks(h.BeforeTerraformApply, cmd, args)
	},
	RunE: func(cmd *cobra.Command, args []string) (runErr error) {
		// Reset captured output for this run.
		capturedApplyOutput = ""

		// On failure, run after hooks with error context so CI check runs
		// are updated to failure status. Cobra skips PostRunE on error.
		defer func() {
			if runErr != nil {
				runHooksOnErrorWithOutput(h.AfterTerraformApply, cmd, args, runErr, capturedApplyOutput)
			}
		}()

		v := viper.GetViper()

		// Bind both parent and subcommand parsers.
		if err := terraformParser.BindFlagsToViper(cmd, v); err != nil {
			return err
		}
		if err := applyParser.BindFlagsToViper(cmd, v); err != nil {
			return err
		}

		// Parse base terraform options.
		opts := ParseTerraformRunOptions(v)

		// Apply-specific flags (from-plan, planfile) flow through the
		// legacy ProcessCommandLineArgs which sets info.UseTerraformPlan, info.PlanFile.
		// The Viper binding above ensures flag > env > config precedence works.

		// When CI mode is enabled, capture terraform apply stdout for CI hooks
		// (summary, comments, outputs). The output is tee'd: terminal still
		// receives it in real time, and the buffer collects a copy.
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
			capturedApplyOutput = ansi.Strip(combined)
		}

		return err
	},
	PostRunE: func(cmd *cobra.Command, args []string) error {
		return runHooksWithOutput(h.AfterTerraformApply, cmd, args, capturedApplyOutput)
	},
}

func init() {
	// Create parser with apply-specific flags using functional options.
	applyParser = flags.NewStandardParser(
		WithBackendExecutionFlags(),
		flags.WithStringFlag("from-plan", "", "", "Apply from plan file (uses deterministic location if path not specified)"),
		flags.WithNoOptDefVal("from-plan", "true"),
		flags.WithStringFlag("planfile", "", "", "Set the plan file to use"),
		flags.WithBoolFlag("affected", "", false, "Apply the affected components in dependency order"),
		flags.WithBoolFlag("all", "", false, "Apply all components in all stacks"),
		flags.WithBoolFlag("ci", "", false, "Enable CI mode for automated pipelines (writes job summary, outputs)"),
		flags.WithBoolFlag("verify-plan", "", false, "Verify stored planfile matches current state before applying"),
		flags.WithEnvVars("from-plan", "ATMOS_TERRAFORM_APPLY_FROM_PLAN"),
		flags.WithEnvVars("planfile", "ATMOS_TERRAFORM_APPLY_PLANFILE"),
		flags.WithEnvVars("verify-plan", "ATMOS_TERRAFORM_VERIFY_PLAN"),
		flags.WithEnvVars("ci", "ATMOS_CI", "CI"),
	)

	// Register apply-specific flags with Cobra.
	applyParser.RegisterFlags(applyCmd)

	// Bind flags to Viper for environment variable support.
	if err := applyParser.BindToViper(viper.GetViper()); err != nil {
		panic(err)
	}

	// Register completions for apply command.
	RegisterTerraformCompletions(applyCmd)

	// Register compat flags for this subcommand.
	internal.RegisterCommandCompatFlags("terraform", "apply", ApplyCompatFlags())

	// Attach to parent terraform command.
	terraformCmd.AddCommand(applyCmd)
}
