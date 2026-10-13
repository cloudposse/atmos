// Package doctor registers project diagnostics without the normal executable configuration bootstrap.
package doctor

import (
	"context"
	"errors"
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
	diagnostic "github.com/cloudposse/atmos/pkg/doctor"
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/flags/compat"
	atmosgit "github.com/cloudposse/atmos/pkg/git"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/signals"
	"github.com/cloudposse/atmos/pkg/ui/batch"
	"github.com/cloudposse/atmos/pkg/validation"
)

func init() { internal.Register(&CommandProvider{}) }

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
			if err := cobra.NoArgs(cmd, args); err != nil {
				return errUtils.WithExitCode(err, 2)
			}
			return nil
		},
		// Deliberately override the root's config-loading pre-run.
		PersistentPreRunE: func(*cobra.Command, []string) error { return nil }, RunE: run,
		SilenceUsage: true, SilenceErrors: true,
	}
	f := cmd.Flags()
	f.Bool("fix", false, "Apply verified behavior-preserving repairs")
	f.Bool("dry-run", false, "Verify fixes without writing files or backups (requires --fix)")
	f.String("format", "text", "Report format: text, rich, json, sarif")
	f.String("fail-on", "error", "Failure threshold: error, warning, none")
	f.StringSlice("only", nil, "Check IDs or categories to run")
	f.StringArray("exclude", nil, "Exclude project-relative glob patterns")
	f.Bool("affected", false, "Check inputs affected since the selected Git base")
	f.String("base", "", "Git comparison base for --affected")
	f.Bool("list-checks", false, "List built-in checks")
	f.Bool("offline", false, "Skip network repository metadata checks")
	cmd.SetFlagErrorFunc(func(_ *cobra.Command, err error) error { return errUtils.WithExitCode(err, 2) })
	return cmd
}

func run(cmd *cobra.Command, _ []string) error {
	defer perf.Track(nil, "doctor.command")()
	format, _ := cmd.Flags().GetString("format")
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
		return errUtils.ExitCodeError{Code: errUtils.ExitCodeSIGINT, Silent: true}
	}
	return finishRun(cmd, format, &report, runErr, o.Fix)
}

// finishRun renders a completed engine run and maps it to the exit code.
func finishRun(cmd *cobra.Command, format string, report *diagnostic.Report, runErr error, fix bool) error {
	// A run that stopped before evaluating anything has no findings to summarize.
	if runErr != nil && !isMachineFormat(format) && nothingEvaluated(report) {
		return errUtils.WithExitCode(runErr, usageExitCode)
	}
	if renderErr := render(report, format, fix); renderErr != nil {
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
	report := diagnostic.Report{Version: 1, Findings: []diagnostic.Finding{}, Checks: []diagnostic.Check{}, ChangedFiles: []string{}, Fix: fix, DryRun: dryRun, OperationalError: err.Error()}
	if renderErr := render(&report, format, false); renderErr != nil {
		return errUtils.WithExitCode(renderErr, usageExitCode)
	}
	return errUtils.ExitCodeError{Code: usageExitCode, Silent: true}
}

type staticStackValidator struct{}

func (staticStackValidator) Validate(ctx context.Context, config *schema.AtmosConfiguration, inputs map[string]map[string]any) error {
	return stackexec.ValidateStaticStackMaps(ctx, config, inputs)
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
	var report diagnostic.Report
	err := batch.Run(0, func(observe batch.Observer) error {
		engine.Observe = progressObserver(observe)
		var err error
		report, err = engine.Run(ctx, *o)
		return err
	})
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
