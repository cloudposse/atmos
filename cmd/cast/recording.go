package cast

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/asciicast"
	cfg "github.com/cloudposse/atmos/pkg/config"
	envpkg "github.com/cloudposse/atmos/pkg/env"
	pkgFlags "github.com/cloudposse/atmos/pkg/flags"
	iolib "github.com/cloudposse/atmos/pkg/io"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

const (
	// FlagName is the global flag used to record command output as an asciicast.
	FlagName = cfg.CastFlagName

	// EnvName is the environment variable used to request cast recording.
	EnvName = cfg.CastEnvVarName

	autoFlagValue = cfg.CastFlagAutoValue

	castExtension  = ".cast"
	gifExtension   = ".gif"
	mp4Extension   = ".mp4"
	htmlExtension  = ".html"
	asciiExtension = ".ascii"
	pngExtension   = ".png"
	jpgExtension   = ".jpg"
	jpegExtension  = ".jpeg"
)

type recordingSource int

const (
	recordingSourceNone recordingSource = iota
	recordingSourceConfig
	recordingSourceEnv
	recordingSourceFlag
)

type activeRecording struct {
	recorder     *asciicast.Recorder
	restore      func()
	renderOutput string
	removeCast   bool
}

var activeCast *activeRecording

// StartRecordingIfRequested starts the root-command cast recorder when enabled by config or flag.
func StartRecordingIfRequested(cmd *cobra.Command, atmosConfig *schema.AtmosConfiguration, args []string) error {
	defer perf.Track(atmosConfig, "cmd.cast.StartRecordingIfRequested")()

	request, err := resolveRecordingRequest(cmd, atmosConfig, args)
	if err != nil {
		return err
	}
	if skipRecording(cmd, request) {
		return nil
	}
	rec, plan, err := startRecorder(request.value, request.hasPath(), atmosConfig, request.args)
	if err != nil {
		return err
	}

	activeCast = &activeRecording{
		recorder:     rec,
		restore:      iolib.SetRecorder(rec),
		renderOutput: plan.renderOutput,
		removeCast:   plan.removeCast,
	}
	return nil
}

type recordingRequest struct {
	source recordingSource
	value  string
	args   []string
}

func (r recordingRequest) hasPath() bool {
	return r.source == recordingSourceFlag || r.source == recordingSourceEnv
}

func resolveRecordingRequest(cmd *cobra.Command, atmosConfig *schema.AtmosConfiguration, args []string) (recordingRequest, error) {
	resolved, err := pkgFlags.ResolveExplicitStringFlag(cmd, args, FlagName)
	if err != nil {
		return recordingRequest{}, err
	}
	if resolved.Changed {
		return recordingRequest{source: recordingSourceFlag, value: resolved.Value, args: resolved.Args}, nil
	}
	if value, ok := recordingEnvValue(); ok {
		return recordingRequest{source: recordingSourceEnv, value: value, args: args}, nil
	}
	recording := atmosConfig.GetCastRecordingConfig()
	if recording.Enabled {
		return recordingRequest{source: recordingSourceConfig, args: args}, nil
	}
	return recordingRequest{args: args}, nil
}

func recordingEnvValue() (string, bool) {
	value, ok := os.LookupEnv(EnvName)
	if !ok {
		return "", false
	}
	value = strings.TrimSpace(value)
	if value == "" || isFalseEnvValue(value) {
		return "", false
	}
	if isTrueEnvValue(value) {
		return autoFlagValue, true
	}
	return value, true
}

func isTrueEnvValue(value string) bool {
	switch strings.ToLower(value) {
	case "true", "1", "yes", "on":
		return true
	default:
		return false
	}
}

func isFalseEnvValue(value string) bool {
	switch strings.ToLower(value) {
	case "false", "0", "no", "off":
		return true
	default:
		return false
	}
}

// skipRecording reports whether cast recording should not start for this
// invocation. Help and completion output are recorded only when requested by an
// explicit source (`--cast` or ATMOS_CAST); automatic config-enabled recording
// skips them so casual `--help` calls and shell completion machinery
// (__complete) are not captured.
func skipRecording(cmd *cobra.Command, request recordingRequest) bool {
	if request.source == recordingSourceNone {
		return true
	}
	explicit := request.hasPath()
	if isCompletionCommand(cmd) && !explicit {
		return true
	}
	if pkgFlags.IsHelpRequested(cmd, request.args) && !explicit {
		return true
	}
	return false
}

type recordingOutputPlan struct {
	castPath     string
	castBasePath string
	renderOutput string
	removeCast   bool
	explicitCast bool
}

func startRecorder(flagValue string, flagChanged bool, atmosConfig *schema.AtmosConfiguration, args []string) (*asciicast.Recorder, recordingOutputPlan, error) {
	explicitPath := flagChanged && flagValue != "" && flagValue != autoFlagValue
	plan, err := planRecordingOutput(flagValue, explicitPath)
	if err != nil {
		return nil, recordingOutputPlan{}, err
	}
	recording := atmosConfig.GetCastRecordingConfig()
	rec, err := asciicast.Start(&asciicast.Options{
		Path:     plan.castPath,
		BasePath: recordingBasePath(plan, atmosConfig),
		Command:  recordedCommandArgs(args),
		Width:    recording.Width,
		Height:   recording.Height,
		RecordIn: recording.Input,
		Explicit: plan.explicitCast,
		Env:      environment(),
	})
	if err != nil {
		if plan.removeCast && plan.castPath != "" {
			_ = os.Remove(plan.castPath) //nolint:gosec // Removes the intermediate cast path this command planned, not untrusted input.
		}
		return nil, recordingOutputPlan{}, err
	}
	return rec, plan, nil
}

func environment() map[string]string {
	return envpkg.SliceToMap(os.Environ())
}

func planRecordingOutput(value string, explicit bool) (recordingOutputPlan, error) {
	if !explicit {
		return recordingOutputPlan{}, nil
	}
	ext := strings.ToLower(filepath.Ext(value))
	switch ext {
	case castExtension:
		return recordingOutputPlan{castPath: value, explicitCast: true}, nil
	case gifExtension, mp4Extension, htmlExtension, asciiExtension, pngExtension, jpgExtension, jpegExtension:
		return planRenderedRecordingOutput(value)
	default:
		return recordingOutputPlan{}, fmt.Errorf("%w: %s", errUtils.ErrUnsupportedCastOutputExtension, value)
	}
}

func planRenderedRecordingOutput(output string) (recordingOutputPlan, error) {
	if _, err := os.Stat(output); err == nil { //nolint:gosec // Existence check on the user's own --cast output path.
		return recordingOutputPlan{}, fmt.Errorf("%w: %s", asciicast.ErrRenderOutputExists, output)
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return recordingOutputPlan{}, fmt.Errorf("%w: %s: %w", errUtils.ErrStatFile, output, err)
	}
	return recordingOutputPlan{castBasePath: os.TempDir(), renderOutput: output, removeCast: true}, nil
}

func recordingBasePath(plan recordingOutputPlan, atmosConfig *schema.AtmosConfiguration) string {
	if plan.castBasePath != "" {
		return plan.castBasePath
	}
	if atmosConfig == nil {
		return ""
	}
	recording := atmosConfig.GetCastRecordingConfig()
	return recording.BasePath
}

func recordedCommandArgs(args []string) []string {
	return append([]string(nil), args...)
}

// StartHelpRecording starts a cast recording for help output when an explicit
// --cast flag requests one, and returns a writer that records what is written
// to it as cast output events. Cobra renders help before the persistent
// pre-run hooks fire, so the custom help function starts the recording itself
// and tees the rendered help through the returned writer. It returns nil when
// no recording is active or requested.
func StartHelpRecording(cmd *cobra.Command, atmosConfig *schema.AtmosConfiguration) io.Writer {
	defer perf.Track(atmosConfig, "cmd.cast.StartHelpRecording")()

	if activeCast == nil {
		if err := StartRecordingIfRequested(cmd, atmosConfig, os.Args[1:]); err != nil {
			_, _ = fmt.Fprintf(iolib.GetContext().UI(), "Failed to start cast recording: %v\n", err)
			return nil
		}
	}
	if activeCast == nil {
		return nil
	}
	return &recorderOutputWriter{rec: activeCast.recorder}
}

// recorderOutputWriter records writes as cast output events.
type recorderOutputWriter struct {
	rec *asciicast.Recorder
}

func (w *recorderOutputWriter) Write(p []byte) (int, error) {
	w.rec.Record("o", string(p))
	return len(p), nil
}

// ActiveRecordingWidth returns the terminal width (in columns) of the active
// cast recording, or 0 when no recording is running. Help rendering uses this
// so recorded output is laid out at the recorded terminal width.
func ActiveRecordingWidth() int {
	if activeCast == nil {
		return 0
	}
	return activeCast.recorder.Width()
}

// FinalizeRecording closes the active root-command cast recorder, if one is running.
func FinalizeRecording() {
	defer perf.Track(nil, "cmd.cast.FinalizeRecording")()

	if activeCast == nil {
		return
	}
	rec := activeCast.recorder
	renderOutput := activeCast.renderOutput
	removeCast := activeCast.removeCast
	if activeCast.restore != nil {
		activeCast.restore()
	}
	// Clear activeCast before rendering so renderer output is not captured
	// back into the recording.
	activeCast = nil
	if err := rec.Close(); err != nil {
		_, _ = fmt.Fprintf(iolib.GetContext().UI(), "Failed to close cast recording: %v\n", err)
		return
	}
	if removeCast {
		defer func() { _ = os.Remove(rec.Path()) }()
	}
	if renderOutput != "" {
		if err := renderRecordedCast(rec.Path(), renderOutput); err != nil {
			_, _ = fmt.Fprintf(iolib.GetContext().UI(), "Failed to render cast: %v\n", err)
			return
		}
		_, _ = fmt.Fprintf(iolib.GetContext().UI(), "Cast rendered: %s\n", renderOutput)
		return
	}
	_, _ = fmt.Fprintf(iolib.GetContext().UI(), "Cast recorded: %s\n", rec.Path())
}

func renderRecordedCast(castPath, output string) error {
	opts, err := renderOptionsForOutput(output, "", false)
	if err != nil {
		return err
	}
	return asciicast.Render(castPath, &opts)
}

func isCompletionCommand(cmd *cobra.Command) bool {
	if cmd == nil {
		return false
	}

	cmdName := cmd.Name()
	if cmdName == "completion" || cmdName == "__complete" || cmdName == "__completeNoDesc" {
		return true
	}

	//nolint:forbidigo // These are external shell completion variables, not Atmos config.
	if os.Getenv("COMP_LINE") != "" || os.Getenv("_ARGCOMPLETE") != "" {
		return true
	}

	return false
}
