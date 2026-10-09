package cmd

import (
	"os"

	"github.com/spf13/viper"

	"github.com/cloudposse/atmos/pkg/schema"
	terminalenv "github.com/cloudposse/atmos/pkg/terminal/env"
)

func globalColorDisabled(config *schema.AtmosConfiguration) bool {
	// Terminal.NoColor already reflects an explicit --no-color flag (setLogConfig overrides
	// environment-derived values), and ResolveNoColor covers the startup path that runs
	// before Cobra parses flags so --no-color=false still beats ATMOS_NO_COLOR.
	return config.Settings.Terminal.NoColor ||
		terminalenv.ResolveNoColor(os.Args[1:], viper.GetBool("no-color"))
}
