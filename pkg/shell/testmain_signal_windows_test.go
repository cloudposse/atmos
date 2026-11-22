//go:build windows

package shell

import "os"

// killSelfEnv is unused on Windows, which has no POSIX signals; the signal
// tests are built only for other platforms.
const killSelfEnv = "_ATMOS_SHELL_TEST_KILL_SELF"

// killSelf cannot deliver a POSIX signal on Windows.
func killSelf(string) {
	os.Exit(96)
}
