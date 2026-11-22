//go:build !windows

package shell

import (
	"os"
	"strconv"
	"syscall"
	"time"
)

// killSelfEnv makes the test binary terminate itself with the signal number it holds.
const killSelfEnv = "_ATMOS_SHELL_TEST_KILL_SELF"

// killSelf sends the given signal to the current process, so the parent sees
// a child that was terminated by a signal rather than one that exited.
func killSelf(signal string) {
	number, err := strconv.Atoi(signal)
	if err != nil {
		os.Exit(97)
	}
	_ = syscall.Kill(os.Getpid(), syscall.Signal(number))
	// Give the signal time to be delivered; it terminates the process.
	time.Sleep(10 * time.Second)
	os.Exit(98)
}
