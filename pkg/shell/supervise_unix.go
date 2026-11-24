//go:build !windows

package shell

import (
	"os"
	"syscall"

	"golang.org/x/sys/unix"
)

var (
	// The commandTerminationSignals list is forwarded to an `exec` child when Atmos is terminated.
	commandTerminationSignals = []os.Signal{syscall.SIGTERM}

	// The shellTerminationSignals list is forwarded to an interactive shell when Atmos is
	// terminated. An interactive shell ignores SIGTERM, so SIGHUP (what the shell gets
	// when its terminal goes away) follows if it is still running.
	shellTerminationSignals = []os.Signal{syscall.SIGTERM, syscall.SIGHUP}
)

// terminalDeliversInterrupt reports whether Atmos runs in the foreground process group
// of a controlling terminal on stdin, stdout, or stderr. The terminal then sends Ctrl-C
// (SIGINT) to the whole group, the child included, so Atmos must not send it again.
func terminalDeliversInterrupt() bool {
	group := unix.Getpgrp()
	for _, fd := range []int{0, 1, 2} {
		foreground, err := unix.IoctlGetInt(fd, unix.TIOCGPGRP)
		if err == nil && foreground == group {
			return true
		}
	}
	return false
}
