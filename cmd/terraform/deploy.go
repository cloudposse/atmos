package terraform

import (
	"bytes"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	e "github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/pkg/ansi"
	"github.com/cloudposse/atmos/pkg/ci"
	"github.com/cloudposse/atmos/pkg/flags"
	h "github.com/cloudposse/atmos/pkg/hooks"
)

// deployParser handles flag parsing for deploy command.
var deployParser *flags.StandardParser

// capturedDeployOutput holds the terraform deploy stdout when CI mode is active.
// Written in RunE, read in PostRunE and the error-path defer.
var capturedDeployOutput string

// deployCmd represents the terraform deploy command (custom Atmos command).
var deployCmd = &cobra.Command{
	Use:   "deploy",
	Short: "Deploy the specified infrastructure using Terraform",
	Long: `Deploys infrastructure by running the Terraform apply command with automatic approval.

This ensures that the changes defined in your Terraform configuration are applied without requiring manual confirmation, streamlining the deployment process.`,
	PreRunE: func(cmd *cobra.Command, args []string) error {
		return runHooks(h.BeforeTerraformApply, cmd, args)
	},
	RunE: func(cmd *cobra.Command, args []string) (runErr error) {
		// Reset captured output for this run.
		capturedDeployOutput = ""

		// On failure, run after hooks with error context so CI check runs
		// are updated to failure status. Cobra skips PostRunE on error.
		defer func() {
			if runErr != nil {
				runHooksOnErrorWithOutput(h.AfterTerraformApply, cmd, args, runErr, capturedDeployOutput)
			}
		}()

		v := viper.GetViper()

		// Bind both parent and subcommand parsers.
		if err := terraformParser.BindFlagsToViper(cmd, v); err != nil {
			return err
		}
		if err := deployParser.BindFlagsToViper(cmd, v); err != nil {
			return err
		}

		// Parse base terraform options.
		opts := ParseTerraformRunOptions(v)

		// Deploy-specific flags (deploy-run-init, from-plan, planfile) flow through
		// the legacy ProcessCommandLineArgs which sets info.DeployRunInit, etc.
		// The Viper binding above ensures flag > env > config precedence works.

		// When CI mode is enabled, capture terraform deploy stdout for CI hooks
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
			capturedDeployOutput = ansi.Strip(combined)
		}

		return err
	},
	PostRunE: func(cmd *cobra.Command, args []string) error {
		return runHooksWithOutput(h.AfterTerraformApply, cmd, args, capturedDeployOutput)
	},
}

func init() {
	// Create parser with deploy-specific flags using functional options.
	deployParser = flags.NewStandardParser(
		WithBackendExecutionFlags(),
		flags.WithBoolFlag("deploy-run-init", "", false, "If set atmos will run `terraform init` before executing the command"),
		flags.WithStringFlag("from-plan", "", "", "Apply from plan file (uses deterministic location if path not specified)"),
		flags.WithNoOptDefVal("from-plan", "true"),
		flags.WithStringFlag("planfile", "", "", "Set the plan file to use"),
		flags.WithBoolFlag("affected", "", false, "Deploy the affected components in dependency order"),
		flags.WithBoolFlag("all", "", false, "Deploy all components in all stacks"),
		flags.WithBoolFlag("ci", "", false, "Enable CI mode for automated pipelines (writes job summary, outputs)"),
		flags.WithBoolFlag("verify-plan", "", false, "Verify stored planfile matches current state before applying"),
		flags.WithEnvVars("deploy-run-init", "ATMOS_TERRAFORM_DEPLOY_RUN_INIT"),
		flags.WithEnvVars("from-plan", "ATMOS_TERRAFORM_DEPLOY_FROM_PLAN"),
		flags.WithEnvVars("planfile", "ATMOS_TERRAFORM_DEPLOY_PLANFILE"),
		flags.WithEnvVars("verify-plan", "ATMOS_TERRAFORM_VERIFY_PLAN"),
		flags.WithEnvVars("ci", "ATMOS_CI", "CI"),
	)

	// Register deploy-specific flags with Cobra.
	deployParser.RegisterFlags(deployCmd)

	// Bind flags to Viper for environment variable support.
	if err := deployParser.BindToViper(viper.GetViper()); err != nil {
		panic(err)
	}

	// Register completions for deployCmd.
	RegisterTerraformCompletions(deployCmd)

	// Attach to parent terraform command.
	terraformCmd.AddCommand(deployCmd)
}
