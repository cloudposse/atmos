package cmd

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	errUtils "github.com/cloudposse/atmos/errors"
	execpkg "github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/pkg/data"
	envpkg "github.com/cloudposse/atmos/pkg/env"
	iolib "github.com/cloudposse/atmos/pkg/io"
	"github.com/cloudposse/atmos/pkg/reexec"
	runnerstep "github.com/cloudposse/atmos/pkg/runner/step"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/script"
)

// prepareStandaloneScript keeps normal CLI initialization and error reporting,
// but isolates script arguments before early config/global-flag preprocessing.
// It handles a script path or an explicit stdin marker, not a subcommand. Atmos global
// flags may precede the script (`atmos --chdir=dir ./tool arg`); they stay in os.Args so the
// early preprocessing and Cobra apply them as usual, while the script path and its arguments
// are removed so a script's own flags are never mistaken for Atmos flags.
func prepareStandaloneScript() (func(), error) {
	globals, rest, ok := script.SplitGlobalFlags(os.Args[1:], rootFlagTakesValue)
	if !ok || !script.MayBeFile(rest) {
		return func() {}, nil
	}
	previousArgs := os.Args
	file, err := detectStandaloneFile(globals, rest)
	if err != nil {
		os.Args = previousArgs
		return nil, err
	}
	if file == nil {
		os.Args = previousArgs
		return func() {}, nil
	}
	previousRun := RootCmd.RunE
	// A re-exec (version switch, profile fallback) must receive the whole command line, and must
	// leave the script's own flags alone when it strips --chdir and --use-version.
	restoreReexec := reexec.SetOriginalArgs(previousArgs, len(rest))
	os.Args = append([]string{previousArgs[0]}, globals...)
	RootCmd.RunE = func(cmd *cobra.Command, _ []string) error {
		return runStandaloneScript(cmd, file)
	}
	return func() {
		os.Args = previousArgs
		RootCmd.RunE = previousRun
		RootCmd.SetArgs(nil)
		restoreReexec()
	}, nil
}

// detectStandaloneFile applies --chdir (or ATMOS_CHDIR) first, so a relative script path means
// the same thing here as in a version-switch re-exec that has already dropped --chdir, and then
// looks for a script at the first non-global argument.
func detectStandaloneFile(globals, rest []string) (*script.File, error) {
	os.Args = append([]string{os.Args[0]}, globals...)
	if err := processEarlyChdirFlag(); err != nil {
		return nil, err
	}
	return script.DetectFile(rest)
}

// rootFlagTakesValue tells the script detector which Atmos global flags consume the next word.
// Flags with an optional value (NoOptDefVal) only take a value written as --flag=value.
func rootFlagTakesValue(name string, short bool) (takes, found bool) {
	var flag *pflag.Flag
	if short {
		flag = RootCmd.PersistentFlags().ShorthandLookup(name)
	} else {
		flag = RootCmd.PersistentFlags().Lookup(name)
	}
	if flag == nil {
		return false, false
	}
	return flag.NoOptDefVal == "", true
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
	name, sourcePath := filepath.Base(file.Path), file.Path
	if file.Stdin {
		name, sourcePath = "<stdin>", ""
	}
	result, err := engine.Execute(cmd.Context(), script.Spec{
		InstallTools: runnerstep.ScriptToolInstaller(&atmosConfig),
		ParseCommand: standaloneCommandParser(file, streams.Data()),
		// SourcePath anchors imports and tracebacks for files; stdin has no source path.
		Name: name, SourcePath: sourcePath, ProjectRoot: standaloneProjectRoot(),
		Source: string(source), File: file,
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

// standaloneProjectRoot is the directory tracebacks are shown relative to: the current working
// directory, resolved through symlinks so it matches the script's resolved path. Scripts outside
// it keep absolute paths.
func standaloneProjectRoot() string {
	dir, err := os.Getwd()
	if err != nil {
		return ""
	}
	if resolved, err := filepath.EvalSymlinks(dir); err == nil {
		return resolved
	}
	return dir
}

// readStandaloneSource consumes stdin only when the standalone command runs.
func readStandaloneSource(file *script.File, input io.Reader) ([]byte, error) {
	if file.Stdin {
		return io.ReadAll(input)
	}
	// Standalone scripts intentionally accept user-selected file paths, including outside cwd.
	return os.ReadFile(file.Path) //nolint:gosec // The caller explicitly selected this source file.
}
