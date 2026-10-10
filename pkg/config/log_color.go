package config

import (
	"fmt"
	"os"
	"strconv"

	"github.com/spf13/viper"

	errUtils "github.com/cloudposse/atmos/errors"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/schema"
	terminalenv "github.com/cloudposse/atmos/pkg/terminal/env"
)

// Sources a logs color value can come from, named the way users set them.
const (
	logsColorFlagSource   = "--logs-color"
	logsColorEnvSource    = "ATMOS_LOGS_COLOR"
	logsColorConfigSource = "logs.color"
)

// newInvalidLogsColorError builds the user-facing error for a logs color value that is not a boolean.
// The source names where the offending value was set, so the user knows what to fix.
func newInvalidLogsColorError(source, value string) error {
	builder := errUtils.Build(errUtils.ErrInvalidLogsColor).
		WithExplanationf("%s must be true or false; got %q", source, value)
	switch source {
	case logsColorFlagSource:
		builder = builder.WithHint("Use `--logs-color=true` or `--logs-color=false`")
	case logsColorEnvSource:
		builder = builder.WithHint("Set `ATMOS_LOGS_COLOR=true` or `ATMOS_LOGS_COLOR=false`, or unset it")
	default:
		builder = builder.WithHint("Set `logs.color` to `true` or `false` in atmos.yaml")
	}
	return builder.Err()
}

// setLoggingColor resolves logs.color with CLI flag > ATMOS_LOGS_COLOR > atmos.yaml precedence.
// An unparsable override fails with an error naming the flag or environment variable that supplied it.
func setLoggingColor(config *schema.AtmosConfiguration, flagValue string) error {
	options := terminalenv.ColorOptionsFromArgs(os.Args[1:])
	enabled := config.Logs.Color == nil || *config.Logs.Color
	value, source := options.LogsColor, logsColorEnvSource
	if flagValue != "" {
		value, source = flagValue, logsColorFlagSource
	}
	// ColorOptionsFromArgs normalizes every valid CLI or environment value to "true" or "false",
	// so an unparsable value from it can only be the raw environment variable.
	if value != "" {
		parsed, err := strconv.ParseBool(value)
		if err != nil {
			return newInvalidLogsColorError(source, value)
		}
		enabled = parsed
	}
	config.Logs.Color = &enabled
	log.Default().SetColorEnabled(enabled, options.NoColor)
	return nil
}

// validateLogsColorConfig rejects a logs.color value in the loaded configuration that is not a boolean.
// It reads the raw value because Viper's SetTypeByDefaultValue (enabled with the logs.color default)
// silently coerces anything it cannot parse to false before the configuration is unmarshalled.
// An absent or null value keeps the default.
func validateLogsColorConfig(v *viper.Viper) error {
	logs, ok := v.Get("logs").(map[string]any)
	if !ok {
		return nil
	}
	raw, ok := logs["color"]
	if !ok || raw == nil {
		return nil
	}
	text := fmt.Sprint(raw)
	if _, err := strconv.ParseBool(text); err != nil {
		return newInvalidLogsColorError(logsColorConfigSource, text)
	}
	return nil
}
