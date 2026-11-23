//go:build windows

package shell

import "os"

var (
	// commandTerminationSignals is empty on Windows, which has no SIGTERM to forward.
	commandTerminationSignals []os.Signal

	// shellTerminationSignals is empty on Windows, which has no SIGTERM or SIGHUP.
	shellTerminationSignals []os.Signal
)

// terminalDeliversInterrupt reports true on Windows: the console delivers Ctrl-C to
// every process attached to it, the child included, so Atmos never forwards it.
func terminalDeliversInterrupt() bool {
	return true
}
