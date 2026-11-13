package cmd

import (
	"github.com/spf13/cobra"

	"github.com/cloudposse/atmos/pkg/perf"
)

// linksFlagName is the global flag that selects how long URLs are delivered.
const linksFlagName = "links"

// resolveLinksSetting returns the link delivery mode to hand to ui.SetLinkMode.
// Precedence is flag > env > atmos.yaml > default: configured is
// settings.terminal.links, which already includes ATMOS_TERMINAL_LINKS, and an
// explicitly provided --links flag overrides it. An empty result means auto.
func resolveLinksSetting(cmd *cobra.Command, configured string) string {
	defer perf.Track(nil, "cmd.resolveLinksSetting")()

	if cmd == nil {
		return configured
	}
	flag := cmd.Flags().Lookup(linksFlagName)
	if flag == nil || !flag.Changed {
		return configured
	}
	return flag.Value.String()
}
