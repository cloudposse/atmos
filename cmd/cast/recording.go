package cast

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/cloudposse/atmos/pkg/asciicast"
	iolib "github.com/cloudposse/atmos/pkg/io"
	"github.com/cloudposse/atmos/pkg/schema"
)

const (
	// FlagName is the global flag used to record command output as an asciicast.
	FlagName = "cast"

	autoFlagValue = "__AUTO__"
)

type activeRecording struct {
	recorder *asciicast.Recorder
	restore  func()
}

var activeCast *activeRecording

// RegisterRecordingFlag registers the global --cast flag on the root command.
func RegisterRecordingFlag(flags *pflag.FlagSet) {
	flags.String(FlagName, "", "Record command output as an asciinema cast (--cast for generated path, --cast=path for explicit output)")
	if castFlag := flags.Lookup(FlagName); castFlag != nil {
		castFlag.NoOptDefVal = autoFlagValue
	}
}

// StartRecordingIfRequested starts the root-command cast recorder when enabled by config or flag.
func StartRecordingIfRequested(cmd *cobra.Command, atmosConfig *schema.AtmosConfiguration, args []string) error {
	if isCompletionCommand(cmd) || cmd.Name() == "help" || cmd.Flags().Changed("help") {
		return nil
	}

	castFlag := cmd.Flags().Lookup(FlagName)
	flagChanged := castFlag != nil && castFlag.Changed
	enabled := atmosConfig.Cast.Recording.Enabled || flagChanged
	if !enabled {
		return nil
	}

	flagValue := ""
	if flagChanged {
		var err error
		flagValue, err = cmd.Flags().GetString(FlagName)
		if err != nil {
			return err
		}
	}
	rec, err := startRecorder(flagValue, flagChanged, atmosConfig, args)
	if err != nil {
		return err
	}

	activeCast = &activeRecording{
		recorder: rec,
		restore:  iolib.SetRecorder(rec),
	}
	return nil
}

func startRecorder(flagValue string, flagChanged bool, atmosConfig *schema.AtmosConfiguration, args []string) (*asciicast.Recorder, error) {
	explicitPath := flagChanged && flagValue != "" && flagValue != autoFlagValue
	return asciicast.Start(&asciicast.Options{
		Path:     explicitPathValue(flagValue, explicitPath),
		BasePath: atmosConfig.Cast.Recording.BasePath,
		Command:  recordedCommandArgs(args),
		Width:    atmosConfig.Cast.Recording.Width,
		Height:   atmosConfig.Cast.Recording.Height,
		RecordIn: atmosConfig.Cast.Recording.Input,
		Explicit: explicitPath,
		Env:      environment(),
	})
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

func explicitPathValue(value string, explicit bool) string {
	if !explicit {
		return ""
	}
	return value
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

// FinalizeRecording closes the active root-command cast recorder, if one is running.
func FinalizeRecording() {
	if activeCast == nil {
		return
	}
	rec := activeCast.recorder
	if activeCast.restore != nil {
		activeCast.restore()
	}
	activeCast = nil
	if err := rec.Close(); err != nil {
		_, _ = fmt.Fprintf(iolib.GetContext().UI(), "Failed to close cast recording: %v\n", err)
		return
	}
	_, _ = fmt.Fprintf(iolib.GetContext().UI(), "Cast recorded: %s\n", rec.Path())
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
