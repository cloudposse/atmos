//go:build windows

package process

import "os/exec"

// enableProcessGroup is a no-op on Windows: process groups are not used, so
// only the direct child is terminated on cancellation.
func enableProcessGroup(_ *exec.Cmd) bool {
	return false
}
