//go:build windows

package process

import (
	"os"
	"os/exec"
)

// enableProcessGroup is a no-op on Windows: process groups are not used, so
// only the direct child is terminated on cancellation.
func enableProcessGroup(_ *exec.Cmd) bool {
	return false
}

func terminateChild(pid int, _ bool) error {
	return killChild(pid, false)
}

func killChild(pid int, _ bool) error {
	p, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	return p.Kill()
}

// childAlive always reports false on Windows: liveness cannot be probed
// without signal 0, so the exit cleanup does not wait for children.
func childAlive(_ int, _ bool) bool {
	return false
}
