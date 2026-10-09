package config

import (
	"fmt"
	"os"
	"strconv"

	errUtils "github.com/cloudposse/atmos/errors"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/schema"
	terminalenv "github.com/cloudposse/atmos/pkg/terminal/env"
)

func setLoggingColor(config *schema.AtmosConfiguration, flagValue string) error {
	options := terminalenv.ColorOptionsFromArgs(os.Args[1:])
	enabled := config.Logs.Color == nil || *config.Logs.Color
	value := options.LogsColor
	if flagValue != "" {
		value = flagValue
	}
	if value != "" {
		parsed, err := strconv.ParseBool(value)
		if err != nil {
			return fmt.Errorf("%w: logs.color must be true or false; got %q", errUtils.ErrInvalidConfig, value)
		}
		enabled = parsed
	}
	config.Logs.Color = &enabled
	log.Default().SetColorEnabled(enabled, options.NoColor)
	return nil
}
