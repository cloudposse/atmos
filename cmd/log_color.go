package cmd

import (
	"os"

	"github.com/spf13/viper"

	"github.com/cloudposse/atmos/pkg/schema"
	terminalenv "github.com/cloudposse/atmos/pkg/terminal/env"
)

func globalColorDisabled(config *schema.AtmosConfiguration) bool {
	return config.Settings.Terminal.NoColor || viper.GetBool("no-color") ||
		terminalenv.ColorOptionsFromArgs(os.Args[1:]).NoColor
}
