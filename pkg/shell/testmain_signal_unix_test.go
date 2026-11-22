//go:build !windows

package shell

import (
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"
)

const (
	// killSelfEnv makes the test binary terminate itself with the signal number it holds.
	killSelfEnv = "_ATMOS_SHELL_TEST_KILL_SELF"

	// awaitSignalEnv makes the test binary wait for a signal. It holds the path of a
	// report file: the child writes "ready" once its handlers are installed and then
	// the name of the first signal it acts on, and exits with 128+N for that signal.
	awaitSignalEnv = "_ATMOS_SHELL_TEST_AWAIT_SIGNAL"

	// ignoreSignalsEnv lists signal numbers (comma-separated) the awaiting child ignores.
	ignoreSignalsEnv = "_ATMOS_SHELL_TEST_IGNORE_SIGNALS"

	// awaitSignalReady is the report content once the awaiting child is ready.
	awaitSignalReady = "ready"
)

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

// awaitSignal records the first non-ignored SIGINT, SIGTERM, or SIGHUP and exits with
// 128+N. It gives up after a bound so a broken test never leaves it running.
func awaitSignal(reportPath string) {
	ignored := map[syscall.Signal]bool{}
	for _, field := range strings.Split(os.Getenv(ignoreSignalsEnv), ",") {
		if number, err := strconv.Atoi(field); err == nil {
			ignored[syscall.Signal(number)] = true
		}
	}

	received := make(chan os.Signal, 4)
	signal.Notify(received, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)
	if err := os.WriteFile(reportPath, []byte(awaitSignalReady), 0o600); err != nil {
		os.Exit(95)
	}

	timeout := time.After(20 * time.Second)
	for {
		select {
		case sig := <-received:
			number := sig.(syscall.Signal)
			if ignored[number] {
				continue
			}
			_ = os.WriteFile(reportPath, []byte(strconv.Itoa(int(number))), 0o600)
			os.Exit(128 + int(number))
		case <-timeout:
			os.Exit(94)
		}
	}
}
