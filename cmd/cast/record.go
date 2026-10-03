package cast

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

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
	Use:     "record <input.tape>",
	Short:   "Record a VHS-dialect tape into an asciicast recording",
	Example: markdown.CastRecordUsageMarkdown,
	Args:    cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := recordParser.BindFlagsToViper(cmd, viper.GetViper()); err != nil {
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
	)
}

func init() {
	recordParser.RegisterFlags(recordCmd)
	if err := recordParser.BindToViper(viper.GetViper()); err != nil {
		panic(err)
	}
	castCmd.AddCommand(recordCmd)
}

func runRecordCommand(cmd *cobra.Command, tapeFile string) error {
	output := viper.GetString(recordViperPrefix + "." + recordFlagOutput)
	if output == "" {
		return errUtils.ErrMissingRenderOutput
	}
	mode := viper.GetString(recordViperPrefix + "." + recordFlagMode)
	format := viper.GetString(recordViperPrefix + "." + recordFlagFormat)

	plan, err := planRecordOutput(output, format)
	if err != nil {
		return err
	}
	defer plan.Cleanup()

	step := &schema.WorkflowStep{
		Name:     "record",
		Type:     schema.TaskTypeCast,
		Mode:     mode,
		TapeFile: tapeFile,
		CastOutput: &schema.CastOutput{
			Cast: plan.CastPath,
		},
	}
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
	if isRecordCastTarget(output, format) {
		return recordOutputPlan{CastPath: output, Cleanup: func() {}}, nil
	}

	tmp, err := os.CreateTemp("", "atmos-cast-record-*.cast")
	if err != nil {
		return recordOutputPlan{}, err
	}
	tmpPath := tmp.Name()
	_ = tmp.Close()
	_ = os.Remove(tmpPath) //nolint:gosec // Removes our own just-created temp placeholder; asciicast.Start recreates it.
	cleanup := func() { _ = os.Remove(tmpPath) }

	opts, err := renderOptionsForOutput(output, format, format != "")
	if err != nil {
		cleanup()
		return recordOutputPlan{}, err
	}
	return recordOutputPlan{CastPath: tmpPath, RenderOpts: &opts, Cleanup: cleanup}, nil
}

func isRecordCastTarget(output, format string) bool {
	if format != "" {
		return strings.EqualFold(format, recordFormatCast)
	}
	return strings.EqualFold(filepath.Ext(output), ".cast")
}
