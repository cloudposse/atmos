//go:build !windows

package process

import (
	"os"
	"syscall"
)

// forwardInterrupt delivers the first interrupt to a graceful child.
func forwardInterrupt(pid int, grouped bool, sig os.Signal) {
	target, value, ok := interruptTarget(pid, grouped, sig)
	if !ok {
		return
	}
	_ = syscall.Kill(target, value)
}

// interruptTarget decides where the first interrupt goes so the child receives it exactly once.
//
// SIGINT from a terminal Ctrl-C reaches the terminal's foreground process group. A child
// that shares that group (its stdin is the terminal) already received it, and a second
// SIGINT would make Terraform abort immediately, so nothing is sent. A grouped child runs
// in its own process group and is signalled as a group, as Ctrl-C would signal it.
//
// SIGTERM is sent to the child process only, as `kill -TERM <terraform pid>` would. Sending
// it to the group would terminate provider plugins, which ignore SIGINT but not SIGTERM, while
// Terraform still needs them to finish the in-flight resource.
func interruptTarget(pid int, grouped bool, sig os.Signal) (int, syscall.Signal, bool) {
	value, ok := sig.(syscall.Signal)
	if !ok {
		value = syscall.SIGINT
	}
	if value != syscall.SIGINT {
		return pid, value, true
	}
	if !grouped {
		return 0, 0, false
	}
	return -pid, syscall.SIGINT, true
}
