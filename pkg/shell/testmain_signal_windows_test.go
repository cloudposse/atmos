//go:build windows

package shell

import "os"

const (
	// killSelfEnv is unused on Windows, which has no POSIX signals; the signal
	// tests are built only for other platforms.
	killSelfEnv = "_ATMOS_SHELL_TEST_KILL_SELF"

	// awaitSignalEnv is unused on Windows for the same reason.
	awaitSignalEnv = "_ATMOS_SHELL_TEST_AWAIT_SIGNAL"
)

// killSelf cannot deliver a POSIX signal on Windows.
func killSelf(string) {
	os.Exit(96)
}

// awaitSignal cannot wait for a POSIX signal on Windows.
func awaitSignal(string) {
	os.Exit(96)
}
