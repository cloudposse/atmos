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

// testParser handles flag parsing for the test command.
var testParser *flags.StandardParser

// capturedTestOutput holds the terraform test stdout when CI mode is active.
// Written in RunE, read in PostRunE and the error-path defer.
var capturedTestOutput string

// testCmd represents the terraform test command.
var testCmd = &cobra.Command{
	Use:   "test",
	Short: "Execute integration tests for Terraform modules",
	Long: `Run integration tests for Terraform modules.

For complete Terraform/OpenTofu documentation, see:
  https://developer.hashicorp.com/terraform/cli/commands/test
  https://opentofu.org/docs/cli/commands/test`,
	PreRunE: func(cmd *cobra.Command, args []string) error {
		// Fire before.terraform.test. This runs both user-defined component hooks
		// (e.g. a `kind: step` / `type: emulator` hook that starts a local sandbox)
		// and any CI provider bindings.
		return runBeforeHooks(h.BeforeTerraformTest, cmd, args)
	},
	RunE: func(cmd *cobra.Command, args []string) (runErr error) {
		// Reset the captured output before any early return so the deferred hook and
		// PostRunE read consistent state.
		capturedTestOutput = ""

		// On failure, run after hooks with error context. Cobra skips PostRunE on
		// error, so this is the only place the after.terraform.test hook fires when a
		// test run fails — which is exactly when emulator teardown (when: always) and
		// the failing CI summary matter most.
		defer func() {
			if runErr != nil {
				runHooksOnErrorWithOutput(h.AfterTerraformTest, cmd, args, runErr, capturedTestOutput)
			}
		}()

		v := viper.GetViper()

		// Bind both parent and subcommand parsers.
		if err := terraformParser.BindFlagsToViper(cmd, v); err != nil {
			return err
		}
		if err := testParser.BindFlagsToViper(cmd, v); err != nil {
			return err
		}

		// Parse base terraform options.
		opts, err := ParseTerraformRunOptions(v)
		if err != nil {
			return err
		}

		// When CI mode is enabled, capture terraform test stdout/stderr for CI hooks
		// (summary, outputs). The output is tee'd: the terminal still receives it in
		// real time, and the buffer collects a copy for the CI summary parser.
		// CI mode is active when the --ci flag or ATMOS_CI/CI env var is set, or a CI
		// platform is auto-detected (e.g. GITHUB_ACTIONS=true).
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

		err = terraformRunWithOptions(terraformCmd, cmd, args, opts, shellOpts...)

		// Strip ANSI escape codes so CI templates get clean text. Combine stdout and
		// stderr so that failure messages (which terraform writes to stderr) are
		// available to the CI summary parser.
		if ciMode {
			combined := stdoutBuf.String()
			if errOut := stderrBuf.String(); errOut != "" {
				combined = combined + "\n" + errOut
			}
			capturedTestOutput = ansi.Strip(combined)
		}

		return err
	},
	PostRunE: func(cmd *cobra.Command, args []string) error {
		// Fire after.terraform.test on the success path. This single call runs both
		// user-defined component hooks (via hooks.RunAll — e.g. emulator teardown) and
		// the CI plugin (via RunCIHooks — the pass/fail step summary).
		return runHooksWithOutput(h.AfterTerraformTest, cmd, args, capturedTestOutput)
	},
}

func init() {
	// Create parser with test-specific flags using functional options.
	testParser = flags.NewStandardParser(
		WithBackendExecutionFlags(),
		flags.WithBoolFlag("ci", "", false, "Enable CI mode for automated pipelines (writes job summary, outputs)"),
		flags.WithEnvVars("ci", "ATMOS_CI", "CI"),
	)

	// Register test-specific flags with Cobra.
	testParser.RegisterFlags(testCmd)

	// Bind flags to Viper for environment variable support.
	if err := testParser.BindToViper(viper.GetViper()); err != nil {
		panic(err)
	}

	// Register completions for testCmd.
	RegisterTerraformCompletions(testCmd)

	// Register compat flags for this subcommand.
	internal.RegisterCommandCompatFlags("terraform", "test", TestCompatFlags())

	// Attach to parent terraform command.
	terraformCmd.AddCommand(testCmd)
}
