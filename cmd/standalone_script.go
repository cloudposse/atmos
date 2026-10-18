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
	"github.com/cloudposse/atmos/pkg/ci"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/data"
	envpkg "github.com/cloudposse/atmos/pkg/env"
	iolib "github.com/cloudposse/atmos/pkg/io"
	"github.com/cloudposse/atmos/pkg/reexec"
	runnerstep "github.com/cloudposse/atmos/pkg/runner/step"
	"github.com/cloudposse/atmos/pkg/script"
	"github.com/cloudposse/atmos/pkg/terminal"
	"github.com/cloudposse/atmos/pkg/ui"
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
		// Not a script invocation Atmos can place. Name a leading-flag mistake rather than letting
		// the script path surface later as an unknown command.
		if err := script.DiagnoseLeadingFlags(os.Args[1:], rootFlagOrLocalTakesValue, rootFlagOptionalValue); err != nil {
			return nil, err
		}
		return func() {}, nil
	}
	previousArgs := os.Args
	startDir, _ := os.Getwd()
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
	restoreReexec := reexec.SetOriginalArgs(reexecArgs(previousArgs, rest, startDir), len(rest))
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

// detectStandaloneFile looks for a script at the first non-global argument. A relative script
// path is anchored to the directory the user stood in before --chdir (or ATMOS_CHDIR) moves the
// process, because that is the directory they typed it against. It then applies the chdir.
func detectStandaloneFile(globals, rest []string) (*script.File, error) {
	startDir, _ := os.Getwd()
	anchored := script.AnchorPath(rest, startDir)
	os.Args = append([]string{os.Args[0]}, globals...)
	if err := processEarlyChdirFlag(); err != nil {
		return nil, err
	}
	file, err := script.DetectFile(anchored)
	if file != nil && !file.Stdin && len(rest) > 0 {
		// Usage hints keep the spelling the user typed.
		file.Invoked = rest[0]
	}
	return file, err
}

// reexecArgs returns the command line a re-exec must receive. The re-executed process runs after
// --chdir was applied and stripped, so when the working directory moved, a relative script path is
// replaced by the absolute path it meant when the user typed it.
func reexecArgs(args, rest []string, startDir string) []string {
	if now, err := os.Getwd(); err == nil && now == startDir {
		return args
	}
	anchored := script.AnchorPath(rest, startDir)
	if len(rest) == 0 || len(rest) > len(args) || anchored[0] == rest[0] {
		return args
	}
	forwarded := append([]string(nil), args...)
	forwarded[len(forwarded)-len(rest)] = anchored[0]
	return forwarded
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

// rootFlagOrLocalTakesValue extends rootFlagTakesValue with the root command's local flags and the
// help flag, which are valid before any command and so never "unknown" to a user.
func rootFlagOrLocalTakesValue(name string, short bool) (takes, found bool) {
	if takes, found = rootFlagTakesValue(name, short); found {
		return takes, found
	}
	if name == "help" || name == "h" {
		return false, true
	}
	var flag *pflag.Flag
	if short {
		flag = RootCmd.Flags().ShorthandLookup(name)
	} else {
		flag = RootCmd.Flags().Lookup(name)
	}
	if flag == nil {
		return false, false
	}
	return flag.NoOptDefVal == "", true
}

// rootFlagOptionalValue reports whether a global flag takes a value only in the `--flag=value`
// form, such as --profile and --identity. Boolean flags are excluded: they never take a value.
func rootFlagOptionalValue(name string, short bool) bool {
	var flag *pflag.Flag
	if short {
		flag = RootCmd.PersistentFlags().ShorthandLookup(name)
	} else {
		flag = RootCmd.PersistentFlags().Lookup(name)
	}
	return flag != nil && flag.NoOptDefVal != "" && flag.Value.Type() != "bool"
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
	cfg.DisableMetricsSummaryByDefault(&atmosConfig)
	processEnv := script.ProcessEnvironment(envpkg.MergeGlobalEnv(os.Environ(), atmosConfig.Env), standaloneSelectionEnv(cmd))
	vars := runnerstep.NewVariables()
	vars.SetAtmosConfig(&atmosConfig)
	vars.SetScriptComponentInfoResolver(execpkg.ScriptComponentInfoResolver(&atmosConfig, nil))
	name, sourcePath := filepath.Base(file.Path), file.Path
	if file.Stdin {
		name, sourcePath = "<stdin>", ""
	}
	result, err := engine.Execute(cmd.Context(), script.Spec{
		Steps:        runnerstep.NewAutomationLibrary(vars, nil),
		InstallTools: runnerstep.ScriptToolInstaller(&atmosConfig),
		CI:           ci.NewReporter(&atmosConfig),
		ParseCommand: standaloneCommandParser(file, streams.Data()),
		// SourcePath anchors imports and tracebacks for files; stdin has no source path.
		Name: name, SourcePath: sourcePath, ProjectRoot: standaloneProjectRoot(),
		Source: string(source), File: file,
		ProcessEnv: processEnv,
		Stdout:     streams.Data(), Stderr: streams.UI(),
		ResolveComponent: runnerstep.ScriptComponentResolver(vars),
		Viewport:         standaloneViewport,
	})
	if err != nil {
		return err
	}
	if result.HasOutput {
		return data.Writeln(result.Value)
	}
	return nil
}

// standaloneViewport shows a subprocess's live output in the step output viewport, the same one
// `output: viewport` uses in workflow and command steps. Without a terminal it streams instead.
func standaloneViewport(title string, run func(stdout, stderr io.Writer) error) error {
	_, _, err := runnerstep.NewOutputModeWriter(runnerstep.OutputModeViewport, title, nil).ExecuteWithIO(run)
	return err
}

// standaloneSelectionEnv carries the profile and identity chosen with global flags written before
// the script path (`atmos --profile=dev ./tool`) into the environment of nested `atmos.*` calls.
func standaloneSelectionEnv(cmd *cobra.Command) map[string]string {
	identity := ""
	if cmd.Flags().Changed(cfg.IdentityFlagName) {
		identity = GetIdentityFromFlags(cmd, os.Args)
	}
	return script.SelectionEnv(cfg.GetActiveProfiles(&atmosConfig), identity)
}

// colorScanArgs returns the arguments that may carry Atmos global flags. For a standalone script
// invocation (`atmos [globals] ./tool.star args`) only the leading global flags qualify: everything
// after the script path belongs to the script, so a script's own `--force-color` must not switch
// Atmos color on. Any other invocation is scanned in full.
func colorScanArgs(args []string) []string {
	if len(args) < 2 {
		return args
	}
	globals, rest, ok := script.SplitGlobalFlags(args[1:], rootFlagTakesValue)
	if !ok || !script.MayBeFile(rest) {
		return args
	}
	return globals
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

// stdinIsTTY reports whether standard input is an interactive terminal. Tests replace it.
var stdinIsTTY = func() bool { return terminal.New().IsTTY(terminal.Stdin) }

// stdinScriptPrompt tells a person who typed `atmos -` at a terminal why nothing happens yet.
const stdinScriptPrompt = "Reading script from stdin; press Ctrl-D to end."

// readStandaloneSource consumes stdin only when the standalone command runs.
func readStandaloneSource(file *script.File, input io.Reader) ([]byte, error) {
	if file.Stdin {
		if stdinIsTTY() {
			ui.Info(stdinScriptPrompt)
		}
		return io.ReadAll(input)
	}
	// Standalone scripts intentionally accept user-selected file paths, including outside cwd.
	return os.ReadFile(file.Path) // The caller explicitly selected this source file.
}
