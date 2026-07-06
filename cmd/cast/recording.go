package cast

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/asciicast"
	iolib "github.com/cloudposse/atmos/pkg/io"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

const (
	// FlagName is the global flag used to record command output as an asciicast.
	FlagName = "cast"

	autoFlagValue = "__AUTO__"

	castExtension  = ".cast"
	gifExtension   = ".gif"
	mp4Extension   = ".mp4"
	htmlExtension  = ".html"
	asciiExtension = ".ascii"
	pngExtension   = ".png"
	jpgExtension   = ".jpg"
	jpegExtension  = ".jpeg"
)

type activeRecording struct {
	recorder     *asciicast.Recorder
	restore      func()
	renderOutput string
	removeCast   bool
}

var activeCast *activeRecording

// RegisterRecordingFlag registers the global --cast flag on the root command.
func RegisterRecordingFlag(flags *pflag.FlagSet) {
	flags.String(FlagName, "", "Record command output as an asciinema cast (--cast for generated path, --cast=path with a .cast, .gif, .mp4, .html, .ascii, .png, .jpg, or .jpeg extension for explicit output)")
	if castFlag := flags.Lookup(FlagName); castFlag != nil {
		castFlag.NoOptDefVal = autoFlagValue
		castFlag.Hidden = true
	}
}

// StartRecordingIfRequested starts the root-command cast recorder when enabled by config or flag.
func StartRecordingIfRequested(cmd *cobra.Command, atmosConfig *schema.AtmosConfiguration, args []string) error {
	defer perf.Track(atmosConfig, "cmd.cast.StartRecordingIfRequested")()

	castFlag := cmd.Flags().Lookup(FlagName)
	flagChanged := castFlag != nil && castFlag.Changed
	flagValue := ""
	if flagChanged {
		var err error
		flagValue, err = cmd.Flags().GetString(FlagName)
		if err != nil {
			return err
		}
	}
	// Commands with DisableFlagParsing (e.g. `git hooks run`) never parse the
	// global --cast flag, so recover it from the raw arguments — the same way
	// processChdirFlag recovers --chdir.
	if !flagChanged && cmd.DisableFlagParsing {
		flagValue, flagChanged = castFlagFromArgs(args)
	}
	if skipRecording(cmd, atmosConfig, flagChanged) {
		return nil
	}
	rec, plan, err := startRecorder(flagValue, flagChanged, atmosConfig, args)
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

// skipRecording reports whether cast recording should not start for this
// invocation. Help and completion output are recorded only when --cast is
// passed explicitly; implicit (config-enabled) recording skips them so casual
// `--help` calls and shell completion machinery (__complete) are not
// captured. Explicit help recording powers the docs screengrab pipeline.
func skipRecording(cmd *cobra.Command, atmosConfig *schema.AtmosConfiguration, flagChanged bool) bool {
	if isCompletionCommand(cmd) && !flagChanged {
		return true
	}
	isHelp := cmd.Name() == "help" || cmd.Flags().Changed("help")
	if isHelp && !flagChanged {
		return true
	}
	recording := atmosConfig.GetCastRecordingConfig()
	return !recording.Enabled && !flagChanged
}

// castFlagFromArgs extracts the --cast flag from unparsed raw arguments.
// Arguments after "--" belong to the downstream command and are not scanned.
func castFlagFromArgs(args []string) (string, bool) {
	for _, arg := range args {
		if arg == "--" {
			break
		}
		if arg == "--"+FlagName {
			return autoFlagValue, true
		}
		if strings.HasPrefix(arg, "--"+FlagName+"=") {
			return strings.TrimPrefix(arg, "--"+FlagName+"="), true
		}
	}
	return "", false
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
	env := make(map[string]string)
	for _, pair := range os.Environ() {
		k, v, ok := strings.Cut(pair, "=")
		if ok {
			env[k] = v
		}
	}
	return env
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
	result := make([]string, 0, len(args))
	for _, arg := range args {
		if arg == "--"+FlagName || strings.HasPrefix(arg, "--"+FlagName+"=") {
			continue
		}
		result = append(result, arg)
	}
	return result
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
	switch strings.ToLower(filepath.Ext(output)) {
	case gifExtension:
		return asciicast.Render(castPath, &asciicast.RenderOptions{GIF: output})
	case mp4Extension:
		return asciicast.Render(castPath, &asciicast.RenderOptions{MP4: output})
	case htmlExtension:
		return asciicast.Render(castPath, &asciicast.RenderOptions{HTML: output})
	case asciiExtension:
		return asciicast.Render(castPath, &asciicast.RenderOptions{ASCII: output})
	case pngExtension:
		return asciicast.Render(castPath, &asciicast.RenderOptions{PNG: output})
	case jpgExtension, jpegExtension:
		return asciicast.Render(castPath, &asciicast.RenderOptions{JPEG: output})
	default:
		return fmt.Errorf("%w: %s", errUtils.ErrUnsupportedCastOutputExtension, output)
	}
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
