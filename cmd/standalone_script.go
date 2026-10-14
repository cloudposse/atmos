package cmd

import (
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"

	errUtils "github.com/cloudposse/atmos/errors"
	execpkg "github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/pkg/data"
	envpkg "github.com/cloudposse/atmos/pkg/env"
	iolib "github.com/cloudposse/atmos/pkg/io"
	runnerstep "github.com/cloudposse/atmos/pkg/runner/step"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/script"
)

// prepareStandaloneScript keeps normal CLI initialization and error reporting,
// but isolates script arguments before early config/global-flag preprocessing.
// It handles a leading script path or an explicit stdin marker, not a subcommand.
func prepareStandaloneScript() (func(), error) {
	file, err := script.DetectFile(os.Args[1:])
	if err != nil {
		return nil, err
	}
	if file == nil {
		return func() {}, nil
	}
	previousArgs, previousRun := os.Args, RootCmd.RunE
	os.Args = []string{os.Args[0]}
	RootCmd.RunE = func(cmd *cobra.Command, _ []string) error {
		return runStandaloneScript(cmd, file)
	}
	return func() {
		os.Args = previousArgs
		RootCmd.RunE = previousRun
		RootCmd.SetArgs(nil)
	}, nil
}

// suppressMetricsSummaryByDefault turns the local resource-usage summary off for
// standalone scripts unless the user explicitly set settings.metrics.enabled.
// A script is the user's own CLI tool, so Atmos-internal telemetry such as the
// final "Total for this invocation" line must be opt-in there. An explicit true
// or false is preserved; every other Atmos command keeps the default of enabled.
func suppressMetricsSummaryByDefault(config *schema.AtmosConfiguration) {
	if config.Settings.Metrics.Enabled == nil {
		disabled := false
		config.Settings.Metrics.Enabled = &disabled
	}
}

func runStandaloneScript(cmd *cobra.Command, file *script.File) error {
	streams := iolib.GetContext()
	source, err := readStandaloneSource(file, streams.Input())
	if err != nil {
		return fmt.Errorf("%w: read script: %w", errUtils.ErrScript, err)
	}
	engine, ok := script.Get(file.Interpreter)
	if !ok {
		return fmt.Errorf("%w: embedded interpreter %q is unavailable", errUtils.ErrScript, file.Interpreter)
	}
	suppressMetricsSummaryByDefault(&atmosConfig)
	processEnv := envpkg.MergeGlobalEnv(os.Environ(), atmosConfig.Env)
	vars := runnerstep.NewVariables()
	vars.SetAtmosConfig(&atmosConfig)
	vars.SetScriptComponentInfoResolver(execpkg.ScriptComponentInfoResolver(&atmosConfig, nil))
	name := file.Path
	if file.Stdin {
		name = "<stdin>"
	}
	result, err := engine.Execute(cmd.Context(), script.Spec{
		InstallTools: runnerstep.ScriptToolInstaller(&atmosConfig),
		ParseCommand: standaloneCommandParser(file, streams.Data()),
		Name:         name, Source: string(source), File: file,
		ProcessEnv: processEnv,
		Stdout:     streams.Data(), Stderr: streams.UI(),
		ResolveComponent: runnerstep.ScriptComponentResolver(vars),
	})
	if err != nil {
		return err
	}
	if result.HasOutput {
		return data.Writeln(result.Value)
	}
	return nil
}

// readStandaloneSource consumes stdin only when the standalone command runs.
func readStandaloneSource(file *script.File, input io.Reader) ([]byte, error) {
	if file.Stdin {
		return io.ReadAll(input)
	}
	// Standalone scripts intentionally accept user-selected file paths, including outside cwd.
	return os.ReadFile(file.Path) //nolint:gosec // The caller explicitly selected this source file.
}
