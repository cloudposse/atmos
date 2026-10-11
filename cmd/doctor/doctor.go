// Package doctor registers project diagnostics without the normal executable configuration bootstrap.
package doctor

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"slices"
	"time"

	"github.com/spf13/cobra"

	"github.com/cloudposse/atmos/cmd/internal"
	errUtils "github.com/cloudposse/atmos/errors"
	stackexec "github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/config/homedir"
	"github.com/cloudposse/atmos/pkg/data"
	diagnostic "github.com/cloudposse/atmos/pkg/doctor"
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/flags/compat"
	atmosgit "github.com/cloudposse/atmos/pkg/git"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/signals"
	"github.com/cloudposse/atmos/pkg/ui"
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
	threshold, _ := cmd.Flags().GetString("fail-on")
	if !oneOf(format, "text", "rich", formatJSON, formatSARIF) || !oneOf(threshold, "error", "warning", "none") {
		return errUtils.WithExitCode(diagnostic.ErrOptions, 2)
	}
	list, _ := cmd.Flags().GetBool("list-checks")
	if list {
		return listChecks(format)
	}
	o, err := commandOptions(cmd)
	if err != nil {
		return errUtils.WithExitCode(err, 2)
	}
	release := signals.SuspendInterruptExit()
	defer release()
	ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt)
	defer stop()
	engine := diagnostic.Engine{Stacks: staticStackValidator{}}
	var report diagnostic.Report
	var runErr error
	if format == formatJSON || format == formatSARIF {
		report, runErr = engine.Run(ctx, o)
	} else {
		report, runErr = runStreaming(ctx, &engine, &o)
	}
	renderErr := render(&report, format)
	// Cancellation wins over a failed write so Ctrl-C always exits with the SIGINT code.
	if errors.Is(runErr, context.Canceled) {
		return errUtils.ExitCodeError{Code: errUtils.ExitCodeSIGINT, Silent: true}
	}
	if renderErr != nil {
		return errUtils.WithExitCode(renderErr, 2)
	}
	if code := report.ExitCode(threshold); code != 0 {
		return errUtils.ExitCodeError{Code: code, Silent: true}
	}
	return nil
}

func render(r *diagnostic.Report, format string) error {
	switch format {
	case formatJSON:
		return data.WriteJSON(r)
	case formatSARIF:
		return renderSARIF(r)
	case "rich":
		ui.Writeln(validation.Rich(r.ValidationReport(), validation.RichOptions{}))
	default:
		renderFindings(r)
	}
	return renderSummary(r)
}

func oneOf(value string, values ...string) bool {
	return slices.Contains(values, value)
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
	if cmd.Flags().Changed("base") && !o.Affected {
		return o, fmt.Errorf("%w: --base requires --affected", diagnostic.ErrOptions)
	}
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

func progressObserver(observe batch.Observer) func(diagnostic.Event) {
	phase := ""
	ids := map[string]int{}
	nextID := 0
	started := time.Now()
	return func(event diagnostic.Event) {
		if event.Finding != nil {
			f := *event.Finding
			if root, err := os.Getwd(); err == nil && filepath.IsAbs(f.File) {
				if rel, err := filepath.Rel(root, f.File); err == nil {
					f.File = rel
				}
			}
			observe(batch.Event{Warning: fmt.Sprintf("%s:%d %s [%s] %s", f.File, f.Line, f.RuleID, f.Status, f.Message), Severity: string(f.Severity)})
			return
		}
		if phase != event.Phase {
			phase = event.Phase
			observe(batch.Event{Reset: true, Count: event.Total})
		}
		if event.Status == "warning" {
			observe(batch.Event{Warning: event.Label})
			return
		}
		key := event.Phase + "/" + event.Label
		id, ok := ids[key]
		if !ok {
			nextID++
			id = nextID
			ids[key] = id
		}
		count := event.Total
		if count == 0 {
			count = -1
		}
		observe(batch.Event{Count: count, ID: id, Label: event.Label, Phase: event.Phase, Done: event.Done, Outcome: event.Status, Elapsed: time.Since(started), Diagnostic: true})
	}
}

func renderSARIF(r *diagnostic.Report) error {
	out, err := r.ValidationReport().SARIF()
	if err != nil {
		return err
	}
	var document map[string]any
	if err := json.Unmarshal(out, &document); err != nil {
		return err
	}
	if runs, ok := document["runs"].([]any); ok && len(runs) > 0 {
		if run, ok := runs[0].(map[string]any); ok {
			run["properties"] = map[string]any{"doctor": r}
			run["invocations"] = []any{map[string]any{"executionSuccessful": r.OperationalError == ""}}
		}
	}
	return data.WriteJSON(document)
}

func renderFindings(r *diagnostic.Report) {
	for i := range r.Findings {
		f := &r.Findings[i]
		if f.Status == "fixed" {
			continue
		}
		location := f.File
		if f.Line > 0 {
			location += fmt.Sprintf(":%d", f.Line)
		}
		ui.Writef("%s %s [%s, %s] %s\n", location, f.RuleID, f.Severity, f.Status, f.Message)
		if f.Reason != "" {
			ui.Writef("  %s\n", f.Reason)
		}
		if f.Remediation != "" {
			ui.Writef("  %s\n", f.Remediation)
		}
	}
}

func renderSummary(r *diagnostic.Report) error {
	fixed, remaining := 0, 0
	for i := range r.Findings {
		f := &r.Findings[i]
		if f.Status == "fixed" {
			fixed++
		} else {
			remaining++
		}
	}
	ui.Writef("Fixed %d findings in %d files. %d findings remain.\n", fixed, len(r.ChangedFiles), remaining)
	for _, check := range r.Checks {
		if check.Status == "incomplete" || check.Status == "skipped" {
			ui.Writef("%s: %s. %s\n", check.ID, check.Status, check.Reason)
		}
	}
	if r.DryRun {
		ui.Writeln("Dry run: project files and backups were not written.")
	}
	if r.Backup != "" {
		ui.Writef("Backup: %s\n%s\n", r.Backup, r.Recovery)
	}
	if r.OperationalError != "" {
		ui.Error(r.OperationalError)
	}
	return nil
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

const (
	formatJSON  = "json"
	formatSARIF = "sarif"
)

func listChecks(format string) error {
	if format == formatJSON {
		return data.WriteJSON(diagnostic.Rules())
	}
	if format == formatSARIF {
		report := diagnostic.Report{Version: 1}
		for _, rule := range diagnostic.Rules() {
			report.Checks = append(report.Checks, diagnostic.Check{ID: rule.ID, Status: "listed", Reason: rule.Description})
		}
		return renderSARIF(&report)
	}
	for _, r := range diagnostic.Rules() {
		ui.Writef("%s [%s]: %s\n", r.ID, r.Category, r.Description)
	}
	return nil
}
