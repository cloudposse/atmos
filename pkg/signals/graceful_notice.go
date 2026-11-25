package signals

import (
	"fmt"
	"os"
	"slices"
	"strings"
	"syscall"

	"github.com/cloudposse/atmos/pkg/perf"
)

// GracefulStopNotice tells the user that Atmos is waiting for the named commands to stop
// after the first signal, and how to force the exit.
func GracefulStopNotice(sig os.Signal, names []string) string {
	defer perf.Track(nil, "signals.GracefulStopNotice")()

	commands := "the running command"
	if len(names) > 0 {
		commands = strings.Join(slices.Compact(slices.Sorted(slices.Values(names))), ", ")
	}
	if sig == syscall.SIGTERM {
		return fmt.Sprintf("Termination requested; waiting for %s to stop gracefully. "+
			"Send the signal again to force exit.", commands)
	}
	return fmt.Sprintf("Interrupt received; waiting for %s to stop gracefully. "+
		"Press Ctrl-C again to force exit.", commands)
}
