package env

import (
	"io"
	"os"
	"strconv"

	"github.com/spf13/pflag"
)

// ColorOptions contains the color overrides available before configuration loads.
type ColorOptions struct {
	NoColor    bool
	NoColorSet bool
	LogsColor  string
}

// ColorOptionsFromArgs reads color overrides without consuming a command name as
// a boolean value. Args excludes the executable and parsing stops at --.
// This package deliberately has no Atmos dependencies so the logger can use it
// during package initialization, before any diagnostics are emitted.
func ColorOptionsFromArgs(args []string) ColorOptions {
	//nolint:forbidigo // Bootstrap terminal environment detection precedes Viper.
	noColor, _ := strconv.ParseBool(os.Getenv("ATMOS_NO_COLOR"))
	//nolint:forbidigo // Bootstrap logging environment detection precedes Viper.
	options := ColorOptions{NoColor: noColor, LogsColor: os.Getenv("ATMOS_LOGS_COLOR")}
	fs := pflag.NewFlagSet("early-color", pflag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.Usage = func() {}
	fs.ParseErrorsAllowlist.UnknownFlags = true
	fs.BoolP("help", "h", false, "") // Continue scanning color flags on help invocations.
	fs.BoolVar(&options.NoColor, "no-color", options.NoColor, "")
	logsColor := fs.Bool("logs-color", true, "")
	_ = fs.Parse(args) // Normal flag/config validation reports invalid values later.
	options.NoColorSet = fs.Changed("no-color")
	if fs.Changed("logs-color") {
		options.LogsColor = strconv.FormatBool(*logsColor)
	} else if value, err := strconv.ParseBool(options.LogsColor); err == nil {
		options.LogsColor = strconv.FormatBool(value)
	}
	//nolint:forbidigo // NO_COLOR is authoritative even over --no-color=false.
	options.NoColor = options.NoColor || os.Getenv("NO_COLOR") != ""
	return options
}
