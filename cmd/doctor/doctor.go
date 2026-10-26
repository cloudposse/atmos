// Package doctor registers project diagnostics without the normal executable configuration bootstrap.
package doctor

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"

	"github.com/spf13/cobra"

	"github.com/cloudposse/atmos/cmd/internal"
	errUtils "github.com/cloudposse/atmos/errors"
	stackexec "github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/config/homedir"
	"github.com/cloudposse/atmos/pkg/dependency"
	diagnostic "github.com/cloudposse/atmos/pkg/doctor"
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/flags/compat"
	"github.com/cloudposse/atmos/pkg/flags/osargs"
	atmosgit "github.com/cloudposse/atmos/pkg/git"
	"github.com/cloudposse/atmos/pkg/naming"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/signals"
	"github.com/cloudposse/atmos/pkg/startup"
	"github.com/cloudposse/atmos/pkg/ui"
	"github.com/cloudposse/atmos/pkg/ui/batch"
	"github.com/cloudposse/atmos/pkg/validation"
)

func init() {
	internal.Register(&CommandProvider{}, internal.WithDocumentationURL("https://atmos.tools/cli/commands/doctor"))
}

// configureStartup applies the root's configuration-free startup (global flags, color, masking, and the
// UI and data writers). It is a variable so tests that inject their own writers can keep them in place.
var configureStartup = startup.Configure

// CommandProvider supplies the built-in doctor command.
type CommandProvider struct{}

func (*CommandProvider) GetName() string { return "doctor" }

func (*CommandProvider) GetGroup() string                                           { return "Other Commands" }
func (*CommandProvider) GetFlagsBuilder() flags.Builder                             { return nil }
func (*CommandProvider) GetPositionalArgsBuilder() *flags.PositionalArgsBuilder     { return nil }
func (*CommandProvider) GetCompatibilityFlags() map[string]compat.CompatibilityFlag { return nil }
func (*CommandProvider) GetAliases() []internal.CommandAlias                        { return nil }
func (*CommandProvider) IsExperimental() bool                                       { return false }
func (*CommandProvider) GetCommand() *cobra.Command                                 { return NewCommand() }

// NewCommand constructs an isolated command for registration and tests.
func NewCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use: "doctor", Short: "Diagnose project health and apply verified migrations", Args: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return nil
			}
			// The message avoids Cobra's "unknown command" wording, which the root would rewrap with a hint
			// about 'atmos --help'. Machine formats receive the usual operational_error report.
			return failInvocation(cmd, requestedFormat(cmd), optionError(fmt.Sprintf("unexpected argument %q", args[0]), usageHint))
		},
		// Deliberately override the root's config-loading pre-run, keeping only its configuration-free
		// setup so parsed global flags (--mask, --no-color, --force-color, --redirect-stderr) still apply.
		PersistentPreRunE: func(cmd *cobra.Command, _ []string) error { return configureStartup(cmd) }, RunE: run,
		SilenceUsage: true, SilenceErrors: true,
		Annotations: map[string]string{ConfigFreeAnnotation: "true"},
	}
	f := cmd.Flags()
	f.Bool("fix", false, "Apply verified behavior-preserving repairs")
	f.Bool("dry-run", false, "Verify fixes without writing files or backups (requires --fix)")
	f.String(formatFlagName, formatText, "Report format: text, rich, json, sarif")
	f.String("fail-on", "error", "Failure threshold: error, warning, none")
	f.StringSlice("only", nil, "Check IDs or categories to run")
	f.StringArray("exclude", nil, "Exclude project-relative glob patterns")
	f.Bool("affected", false, "Check inputs affected since the selected Git base")
	f.String("base", "", "Git comparison base for --affected")
	f.Bool("list-checks", false, "List built-in checks")
	f.Bool("offline", false, "Skip network repository metadata checks")
	cmd.SetFlagErrorFunc(func(cmd *cobra.Command, err error) error {
		return failInvocation(cmd, requestedFormat(cmd), optionError(err.Error(), usageHint))
	})
	return cmd
}

// ConfigFreeAnnotation marks a command that never loads atmos.yaml. Anything that would read the
// configuration on its behalf, such as the telemetry disclosure or the update check shown with help,
// must skip it so a malformed configuration cannot surface through those paths.
const ConfigFreeAnnotation = "atmos.tools/config-free"

// IsConfigFree reports whether cmd, or one of its parents, runs without loading atmos.yaml.
func IsConfigFree(cmd *cobra.Command) bool {
	defer perf.Track(nil, "doctor.IsConfigFree")()

	for current := cmd; current != nil; current = current.Parent() {
		if current.Annotations[ConfigFreeAnnotation] == "true" {
			return true
		}
	}
	return false
}

// formatFlagName is the report format flag.
const formatFlagName = "format"

// usageHint points invalid invocations at doctor's own help.
const usageHint = "Run 'atmos doctor --help' to see the supported flags"

// rawArgs returns the command line after the program name. A flag error can fire before --format is parsed,
// so the format is also read from the raw arguments. It is a variable so tests can supply their own arguments.
var rawArgs = func() []string { return os.Args[1:] }

// requestedFormat returns the report format selected on the command line, or the text default.
func requestedFormat(cmd *cobra.Command) string {
	if cmd.Flags().Changed(formatFlagName) {
		if format, err := cmd.Flags().GetString(formatFlagName); err == nil {
			return format
		}
	}
	if format := osargs.ParseString(rawArgs(), formatFlagName); format != "" {
		return format
	}
	return formatText
}

// FailBeforeDispatch reports a failure that happens before doctor's command runs (for example an
// unusable --chdir) for the raw command-line args. Machine formats receive the usual operational_error
// report on stdout and exit with the usage status; other formats return err unchanged.
func FailBeforeDispatch(args []string, err error) error {
	defer perf.Track(nil, "doctor.FailBeforeDispatch")()

	format := osargs.ParseString(args, formatFlagName)
	if !isMachineFormat(format) {
		return err
	}
	return failWithReport(format, false, false, err)
}

func run(cmd *cobra.Command, _ []string) error {
	defer perf.Track(nil, "doctor.command")()
	format, _ := cmd.Flags().GetString(formatFlagName)
	// An invalid format cannot select the machine report encoding, so it stays a plain error.
	if err := validateFormat(format); err != nil {
		return err
	}
	if err := validateInvocation(cmd, format); err != nil {
		return failInvocation(cmd, format, err)
	}
	if list, _ := cmd.Flags().GetBool("list-checks"); list {
		return listChecks(format)
	}
	o, err := commandOptions(cmd)
	if err != nil {
		return failInvocation(cmd, format, errUtils.WithExitCode(err, usageExitCode))
	}
	release := signals.SuspendInterruptExit()
	defer release()
	ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt)
	defer stop()
	engine := diagnostic.Engine{Stacks: staticStackValidator{}}
	var report diagnostic.Report
	var runErr error
	if isMachineFormat(format) {
		report, runErr = engine.Run(ctx, o)
	} else {
		report, runErr = runStreaming(ctx, &engine, &o)
	}
	// Cancellation wins over a failed write so Ctrl-C always exits with the SIGINT code.
	if errors.Is(runErr, context.Canceled) {
		return finishCancelled(format, &report, renderOptions{Fix: o.Fix, FixCommand: fixCommand(cmd)})
	}
	return finishRun(cmd, format, &report, runErr, renderOptions{Fix: o.Fix, FixCommand: fixCommand(cmd)})
}

// finishCancelled handles an interrupted run. The exit code is always the SIGINT code. The partial report
// is rendered only when someone needs it: machine formats always get a report, and an interactive run
// shows one when files were already changed or a backup was written, so the user learns what to recover.
func finishCancelled(format string, report *diagnostic.Report, opts renderOptions) error {
	interrupted := errUtils.ExitCodeError{Code: errUtils.ExitCodeSIGINT, Silent: true}
	if !isMachineFormat(format) && len(report.ChangedFiles) == 0 && report.Backup == "" {
		return interrupted
	}
	report.OperationalError = interruptedMessage(report)
	// A failed write must not hide the interruption, so its error is dropped.
	_ = render(report, format, opts)
	return interrupted
}

// interruptedMessage explains an interrupted run and names the files it had already changed.
func interruptedMessage(report *diagnostic.Report) string {
	message := "Interrupted before the run finished"
	if len(report.ChangedFiles) == 0 {
		if report.Backup != "" {
			return message + "; a backup was written, so verify the project files against it"
		}
		return message + "; no project files were changed"
	}
	return fmt.Sprintf("%s; %d %s already changed: %s", message, len(report.ChangedFiles), plural(len(report.ChangedFiles), "file was", "files were"), diagnostic.SummarizePaths(report.ChangedFiles))
}

// finishRun renders a completed engine run and maps it to the exit code.
func finishRun(cmd *cobra.Command, format string, report *diagnostic.Report, runErr error, opts renderOptions) error {
	// A run that stopped before evaluating anything has no findings to summarize.
	if runErr != nil && !isMachineFormat(format) && nothingEvaluated(report) {
		return errUtils.WithExitCode(runErr, usageExitCode)
	}
	if renderErr := render(report, format, opts); renderErr != nil {
		return errUtils.WithExitCode(renderErr, usageExitCode)
	}
	threshold, _ := cmd.Flags().GetString("fail-on")
	if code := report.ExitCode(threshold); code != 0 {
		return errUtils.ExitCodeError{Code: code, Silent: true}
	}
	return nil
}

// nothingEvaluated reports whether the engine stopped before checking any input.
func nothingEvaluated(r *diagnostic.Report) bool {
	return len(r.Checks) == 0 && len(r.Findings) == 0 && len(r.ChangedFiles) == 0 && r.Backup == ""
}

// failInvocation reports an invalid invocation. Machine formats receive the same report shape as any other
// run, carrying operational_error on stdout, so consumers parse one schema; text formats get the error itself.
func failInvocation(cmd *cobra.Command, format string, err error) error {
	if !isMachineFormat(format) {
		return err
	}
	fix, _ := cmd.Flags().GetBool("fix")
	dryRun, _ := cmd.Flags().GetBool("dry-run")
	return failWithReport(format, fix, dryRun, err)
}

// failWithReport writes the operational_error report for a machine format and exits with the usage status.
func failWithReport(format string, fix, dryRun bool, err error) error {
	report := diagnostic.Report{Version: 1, Findings: []diagnostic.Finding{}, Checks: []diagnostic.Check{}, ChangedFiles: []string{}, Fix: fix, DryRun: dryRun, OperationalError: err.Error()}
	if renderErr := render(&report, format, renderOptions{}); renderErr != nil {
		return errUtils.WithExitCode(renderErr, usageExitCode)
	}
	return errUtils.ExitCodeError{Code: usageExitCode, Silent: true}
}

type staticStackValidator struct{}

// staticInputs adapts the files doctor captured to the inputs of the production stack loader.
func staticInputs(inputs diagnostic.StackInputs) stackexec.StaticStackInputs {
	return stackexec.StaticStackInputs{Files: inputs.Files, Reader: inputs.Reader}
}

func (staticStackValidator) Validate(ctx context.Context, config *schema.AtmosConfiguration, inputs diagnostic.StackInputs) error {
	return stackexec.ValidateStaticStacks(ctx, config, staticInputs(inputs))
}

func (staticStackValidator) ValidateDependencies(ctx context.Context, config *schema.AtmosConfiguration, inputs diagnostic.StackInputs) error {
	return stackexec.ValidateStaticDependencies(ctx, config, staticInputs(inputs))
}

func (staticStackValidator) DependencyTargets(ctx context.Context, config *schema.AtmosConfiguration, inputs diagnostic.StackInputs) (map[dependency.ComponentRef][]dependency.ComponentRef, error) {
	return stackexec.StaticDependencyTargets(ctx, config, staticInputs(inputs))
}

func commandOptions(cmd *cobra.Command) (diagnostic.Options, error) {
	selection := config.EarlyConfigAndStacksInfoFromArgs(os.Args[1:])
	o := diagnostic.Options{ConfigFiles: selection.AtmosConfigFilesFromArg, ConfigDirs: selection.AtmosConfigDirsFromArg, Profiles: selection.ProfilesFromArg, BasePath: selection.AtmosBasePath}
	o.UnverifiedInputs = ambientInputs(&o)
	o.Fix, _ = cmd.Flags().GetBool("fix")
	o.DryRun, _ = cmd.Flags().GetBool("dry-run")
	o.Offline, _ = cmd.Flags().GetBool("offline")
	o.Only, _ = cmd.Flags().GetStringSlice("only")
	o.Exclude, _ = cmd.Flags().GetStringArray("exclude")
	o.Affected, _ = cmd.Flags().GetBool("affected")
	if o.Affected {
		base, _ := cmd.Flags().GetString("base")
		var err error
		o.Selected, err = validation.AffectedFiles(base)
		if err != nil {
			return o, err
		}
		// Git reports affected paths relative to the worktree root, not the working directory.
		o.SelectedRoot, err = atmosgit.GetRoot()
		if err != nil {
			return o, err
		}
	}
	return o, nil
}

func runStreaming(ctx context.Context, engine *diagnostic.Engine, o *diagnostic.Options) (diagnostic.Report, error) {
	renderHeading(o.Fix, o.DryRun)
	var report diagnostic.Report
	err := batch.Run(0, func(observe batch.Observer) error {
		engine.Observe = progressObserver(observe)
		var err error
		report, err = engine.Run(ctx, *o)
		return err
	}, batch.WithPhaseSummaries())
	ui.Writeln("")
	return report, err
}

// ambientInputs detects configuration layers without reading or executing them.
// Explicit --config/--config-path selections bypass home/system discovery in Atmos.
func ambientInputs(options *diagnostic.Options) []string {
	var reasons []string
	if value, ok := os.LookupEnv("ATMOS_COMPONENTS_TERRAFORM_INIT_RUN_RECONFIGURE"); ok && value != "" {
		reasons = append(reasons, "Legacy Terraform reconfigure environment override is active; migration needs effective-input review")
	}
	if len(options.ConfigFiles) > 0 || len(options.ConfigDirs) > 0 {
		return reasons
	}
	dirs := []string{config.SystemDirConfigFilePath}
	if runtime.GOOS == "windows" {
		appData, _ := os.LookupEnv(config.WindowsAppDataEnvVar)
		dirs = []string{appData}
	}
	if home, err := homedir.Dir(); err == nil {
		dirs = append(dirs, filepath.Join(home, ".atmos"))
	} else {
		reasons = append(reasons, "Home configuration discovery was unavailable")
	}
	for _, dir := range dirs {
		if ambientDirectoryExists(dir) {
			reasons = append(reasons, "Home or system Atmos configuration exists outside this project; use an explicit --config selection for isolated project verification")
			break
		}
	}
	return reasons
}

func ambientDirectoryExists(dir string) bool {
	if dir == "" {
		return false
	}
	for _, name := range []string{"atmos.yaml", "atmos.yml", "atmos.d", ".atmos.d"} {
		if _, err := os.Stat(filepath.Join(dir, name)); err == nil {
			return true
		} else if !os.IsNotExist(err) {
			return true
		}
	}
	return false
}

func (staticStackValidator) NamingResults(ctx context.Context, config *schema.AtmosConfiguration, inputs diagnostic.StackInputs) (map[naming.Identity]naming.Result, error) {
	reporter := diagnostic.StaticNamingReporter{
		LoadStacks: func(ctx context.Context, config *schema.AtmosConfiguration, inputs diagnostic.StackInputs) (map[string]any, error) {
			return stackexec.LoadStaticStacks(ctx, config, staticInputs(inputs))
		},
		Resolver: stackexec.NamingResolver(),
	}
	return reporter.NamingResults(ctx, config, inputs)
}

func (staticStackValidator) CloudFormationSettings(ctx context.Context, config *schema.AtmosConfiguration, inputs diagnostic.StackInputs) (map[string]any, error) {
	return stackexec.StaticCloudFormationSettings(ctx, config, stackexec.StaticStackInputs{Files: inputs.Files, Reader: inputs.Reader})
}
