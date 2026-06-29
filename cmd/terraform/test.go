package terraform

import (
	"bytes"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/cloudposse/atmos/cmd/internal"
	e "github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/pkg/ansi"
	"github.com/cloudposse/atmos/pkg/ci"
	tfci "github.com/cloudposse/atmos/pkg/ci/plugins/terraform"
	"github.com/cloudposse/atmos/pkg/flags"
	h "github.com/cloudposse/atmos/pkg/hooks"
	"github.com/cloudposse/atmos/pkg/ui"
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

		// In CI, run `terraform/tofu test -json` for structured, tool-agnostic
		// results (both tools support it) and capture stdout WITHOUT teeing — the
		// raw JSON stream is suppressed and we render a clean human summary below.
		// `-json` is injected via opts.AppendArgs so it lands in
		// info.AdditionalArgsAndFlags and reaches terraform directly, rather than
		// being mis-parsed as a positional stack argument.
		var shellOpts []e.ShellCommandOption
		var stdoutBuf, stderrBuf bytes.Buffer
		if ciMode {
			opts.AppendArgs = appendJSONFlag(opts.AppendArgs)
			shellOpts = append(shellOpts, e.WithStdoutOverride(&stdoutBuf))
			shellOpts = append(shellOpts, e.WithStderrCapture(&stderrBuf))
		}

		err = terraformRunWithOptions(terraformCmd, cmd, args, opts, shellOpts...)

		if ciMode {
			jsonOut := stdoutBuf.String()
			// Feed the JSON stream to the CI hooks (ParseOutput sniffs JSON);
			// append stderr so pre-terraform failures (e.g. auth) are still parseable.
			capturedTestOutput = ansi.Strip(jsonOut)
			if errOut := stderrBuf.String(); errOut != "" {
				capturedTestOutput = capturedTestOutput + "\n" + ansi.Strip(errOut)
			}
			// Render a clean human-readable summary to the terminal/log. When the
			// stream is not a parseable `-json` test run (e.g. an init/auth failure
			// before any test executed), never swallow what terraform actually wrote:
			// surface the raw captured stdout and stderr so the failure is visible.
			if text := tfci.RenderTestText([]byte(jsonOut)); text != "" {
				ui.Write(text)
			} else {
				if jsonOut != "" {
					ui.Write(jsonOut)
				}
				if errOut := stderrBuf.String(); errOut != "" {
					ui.Write(errOut)
				}
			}
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

// appendJSONFlag adds `-json` to the terraform test pass-through flags unless it
// is already present, so CI runs emit the machine-readable event stream. Both
// Terraform and OpenTofu support `test -json`, so this is tool-agnostic and does
// not depend on the binary name (which may be aliased).
func appendJSONFlag(extra []string) []string {
	for _, a := range extra {
		if a == "-json" || a == "--json" {
			return extra
		}
	}
	out := make([]string, len(extra), len(extra)+1)
	copy(out, extra)
	return append(out, "-json")
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
