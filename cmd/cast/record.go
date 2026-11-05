package cast

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/cloudposse/atmos/cmd/internal"
	"github.com/cloudposse/atmos/cmd/markdown"
	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/asciicast"
	"github.com/cloudposse/atmos/pkg/flags"
	runnerstep "github.com/cloudposse/atmos/pkg/runner/step"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui"
)

const (
	recordFlagOutput = "output"
	recordFlagFormat = "format"
	recordFlagMode   = "mode"

	recordFlagShell     = "shell"
	recordFlagShellInit = "shell-init"

	recordViperPrefix = "cast.record"

	recordModeSession = "session"
	recordModeSteps   = "steps"

	// RecordFormatCast keeps the raw asciicast recording (no render pass),
	// the same meaning ".cast" already has for the global --cast flag and
	// atmos cast render.
	recordFormatCast = "cast"
)

var recordParser = newRecordParser()

var recordCmd = &cobra.Command{
	Use:   "record <input.tape>",
	Short: "Record a VHS-dialect tape into an asciicast recording",
	Long: `Interpret a VHS-dialect tape using Atmos's built-in terminal recorder.

Session mode (the default) drives an interactive terminal with typing, keypresses,
pauses, and pattern-based waits. Steps mode runs each typed command separately
and checks its exit status; interactive directives require session mode.

Use --output to save a .cast recording or render an artifact. The output extension
selects the format unless --format is supplied.`,
	Example: markdown.CastRecordUsageMarkdown,
	Args:    cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := recordParser.BindFlagsToViper(cmd, viper.GetViper()); err != nil {
			return err
		}
		// WithValidValues is only enforced by Parse() or an explicit
		// ValidateFlagValues call; RunE reads the bound flags manually, so
		// validate here to reject bad values before any recording starts.
		if err := recordParser.ValidateFlagValues(cmd); err != nil {
			return err
		}
		return runRecordCommand(cmd, args[0])
	},
}

func newRecordParser() *flags.StandardParser {
	return flags.NewStandardParser(
		flags.WithViperPrefix(recordViperPrefix),
		flags.WithStringFlag(recordFlagOutput, "o", "", "Required. Write the recording (or rendered artifact) to this path"),
		flags.WithEnvVars(recordFlagOutput, "ATMOS_CAST_RECORD_OUTPUT"),
		flags.WithStringFlag(recordFlagFormat, "f", "", "Output format (cast, gif, mp4, html, ascii, png, jpg, jpeg)"),
		flags.WithEnvVars(recordFlagFormat, "ATMOS_CAST_RECORD_FORMAT"),
		flags.WithValidValues(recordFlagFormat, recordFormatCast, renderFormatGIF, renderFormatMP4, renderFormatHTML, renderFormatASCII, renderFormatPNG, renderFormatJPG, renderFormatJPEG),
		// Session is the default, not steps: a bare .tape file handed to the
		// CLI is almost always meant to replay as an actual VHS-style
		// session (which commonly includes raw keypresses), so defaulting to
		// steps would make the common case error immediately.
		flags.WithStringFlag(recordFlagMode, "", recordModeSession, "Cast mode: session or steps"),
		flags.WithValidValues(recordFlagMode, recordModeSession, recordModeSteps),
		flags.WithStringFlag(recordFlagShell, "", "", "Session shell command line, optionally with arguments (for example \"bash -o vi\"); overrides the tape's Set Shell"),
		flags.WithEnvVars(recordFlagShell, "ATMOS_CAST_RECORD_SHELL"),
		flags.WithStringFlag(recordFlagShellInit, "", "", "Session shell initialization: clean, user, or the path to an rc file (default: cast.session.shell_init in atmos.yaml, else clean)"),
		flags.WithEnvVars(recordFlagShellInit, "ATMOS_CAST_RECORD_SHELL_INIT"),
	)
}

func init() {
	recordParser.RegisterFlags(recordCmd)
	if err := recordParser.BindToViper(viper.GetViper()); err != nil {
		panic(err)
	}
	castCmd.AddCommand(recordCmd)
	internal.RegisterCommandMetadata(
		recordCmd,
		internal.WithDocumentationURL("https://atmos.tools/cli/commands/cast/record"),
	)
}

func runRecordCommand(cmd *cobra.Command, input string) error {
	output := viper.GetString(recordViperPrefix + "." + recordFlagOutput)
	if output == "" {
		return errUtils.ErrMissingRenderOutput
	}
	mode := viper.GetString(recordViperPrefix + "." + recordFlagMode)
	format := viper.GetString(recordViperPrefix + "." + recordFlagFormat)
	shell := viper.GetString(recordViperPrefix + "." + recordFlagShell)
	shellInit := viper.GetString(recordViperPrefix + "." + recordFlagShellInit)

	plan, err := planRecordOutput(output, format)
	if err != nil {
		return err
	}
	defer plan.Cleanup()

	step := buildRecordStep(input, mode, recordShellOptions{Shell: shell, ShellInit: shellInit}, &plan)
	if _, err := runnerstep.NewStepExecutorWithVars(runnerstep.NewVariables()).Execute(cmd.Context(), step); err != nil {
		return err
	}

	if plan.RenderOpts == nil {
		return nil
	}
	if err := asciicast.Render(plan.CastPath, plan.RenderOpts); err != nil {
		return err
	}
	ui.Successf("Cast rendered: %s", output)
	return nil
}

// recordShellOptions carries the --shell and --shell-init values. An empty
// field leaves the lower-precedence source in charge: the tape's Set Shell
// (then $SHELL) for Shell, and atmos.yaml cast.session.shell_init (then
// "clean") for ShellInit.
type recordShellOptions struct {
	Shell     string
	ShellInit string
}

// buildRecordStep assembles the cast step that replays tapeFile and records
// it to plan.CastPath. A rendered target
// records to a temporary .cast that is deleted after rendering, so that path
// is marked intermediate and not announced.
func buildRecordStep(tapeFile, mode string, shell recordShellOptions, plan *recordOutputPlan) *schema.WorkflowStep {
	return &schema.WorkflowStep{
		Name:      "record",
		Type:      schema.TaskTypeCast,
		Mode:      mode,
		Source:    tapeFile,
		Shell:     shell.Shell,
		ShellInit: shell.ShellInit,
		CastOutput: &schema.CastOutput{
			Cast:         plan.CastPath,
			Intermediate: plan.RenderOpts != nil,
		},
	}
}

// recordOutputPlan is the resolved destination for a recorded tape: either
// the raw recording is kept directly at CastPath (RenderOpts nil), or
// CastPath is a temporary file to be rendered via RenderOpts and removed by
// Cleanup afterward.
type recordOutputPlan struct {
	CastPath   string
	RenderOpts *asciicast.RenderOptions
	Cleanup    func()
}

// planRecordOutput decides whether --output/--format request the raw
// recording (kept exactly at output, no render pass) or a rendered artifact
// (recorded to a temp .cast, rendered to output, temp file removed) --
// mirroring the same "record to temp, render, delete intermediate" pattern
// the global --cast=<gif> flag already documents (see recording.go).
func planRecordOutput(output, format string) (recordOutputPlan, error) {
	if err := checkRecordFormatMatchesExtension(output, format); err != nil {
		return recordOutputPlan{}, err
	}
	if isRecordCastTarget(output, format) {
		return recordOutputPlan{CastPath: output, Cleanup: func() {}}, nil
	}

	tmp, err := os.CreateTemp("", "atmos-cast-record-*.cast")
	if err != nil {
		return recordOutputPlan{}, fmt.Errorf("%w: %w", errUtils.ErrCreateTempDir, err)
	}
	tmpPath := tmp.Name()
	_ = tmp.Close()
	// Remove our own just-created temp placeholder; asciicast.Start recreates it.
	_ = os.Remove(tmpPath)
	cleanup := func() { _ = os.Remove(tmpPath) }

	opts, err := renderOptionsForOutput(output, format, format != "")
	if err != nil {
		cleanup()
		return recordOutputPlan{}, err
	}
	// The user named this exact output path, so re-running replaces it instead
	// of failing after a full recording with "render output already exists".
	opts.Overwrite = true
	return recordOutputPlan{CastPath: tmpPath, RenderOpts: &opts, Cleanup: cleanup}, nil
}

// checkRecordFormatMatchesExtension rejects an explicit --format that
// disagrees with a recognized output extension (.cast, .gif, .mp4, .html,
// .ascii, .png, .jpg, .jpeg; jpg and jpeg are equivalent), in either
// direction, so a recording is never written into a file whose extension
// lies about its contents. An extensionless or unrecognized output path
// accepts any format.
func checkRecordFormatMatchesExtension(output, format string) error {
	if format == "" {
		return nil
	}
	selected, ok := normalizeRecordFormat(format)
	if !ok {
		return fmt.Errorf("%w: unsupported format: %s", errUtils.ErrInvalidFlag, format)
	}
	extension, ok := normalizeRecordFormat(strings.TrimPrefix(filepath.Ext(output), "."))
	if !ok || extension == selected {
		return nil
	}
	return fmt.Errorf("%w: --format %s conflicts with output extension %s", errUtils.ErrInvalidFlag, format, filepath.Ext(output))
}

// normalizeRecordFormat extends normalizeRenderFormat with the raw "cast"
// format that atmos cast record additionally accepts.
func normalizeRecordFormat(format string) (string, bool) {
	if strings.EqualFold(format, recordFormatCast) {
		return recordFormatCast, true
	}
	return normalizeRenderFormat(format)
}

func isRecordCastTarget(output, format string) bool {
	if format != "" {
		return strings.EqualFold(format, recordFormatCast)
	}
	return strings.EqualFold(filepath.Ext(output), ".cast")
}
