//go:build !windows

package step

import (
	"os"
	"strconv"
	"syscall"
)

// stepProcessGone reports whether pid no longer exists. A zombie that was not reaped counts as
// gone: it can no longer run code.
func stepProcessGone(pid int) bool {
	if err := syscall.Kill(pid, 0); err != nil {
		return true
	}
	data, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/stat")
	if err != nil {
		return false
	}
	// Format: "pid (comm) S ..." -- the state follows the last ')'.
	text := string(data)
	for i := len(text) - 1; i >= 0; i-- {
		if text[i] == ')' {
			return i+2 < len(text) && text[i+2] == 'Z'
		}
	}
	return false
}

// stepProcessGroupsSupported reports whether the platform terminates a command's whole process tree.
const stepProcessGroupsSupported = true
