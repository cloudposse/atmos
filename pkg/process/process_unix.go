//go:build !windows

package process

import (
	"errors"
	"os"
	"os/exec"
	"syscall"
	"time"
)

// enableProcessGroup makes cmd the leader of a new process group and installs
// a cancellation hook that terminates the whole group: SIGTERM first, then
// SIGKILL to surviving group members after childShutdownGrace. Cancellation waits
// for cleanup, so an exiting group leader cannot cancel its descendants' cleanup.
// It reports true when the group was configured.
func enableProcessGroup(cmd *exec.Cmd) bool {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	// A PTY-backed command already calls Setsid, which cannot be combined with
	// Setpgid.
	if cmd.SysProcAttr.Setsid || cmd.SysProcAttr.Setctty {
		return false
	}
	cmd.SysProcAttr.Setpgid = true
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return os.ErrProcessDone
		}
		pid := cmd.Process.Pid
		err := syscall.Kill(-pid, syscall.SIGTERM)
		switch {
		case err == nil:
			finishGroupCancellation(pid)
			return nil
		case errors.Is(err, syscall.ESRCH):
			return os.ErrProcessDone
		default:
			return cmd.Process.Kill()
		}
	}
	return true
}

// signalChild sends sig to the child's process group (grouped) or to the
// child process alone.
func signalChild(pid int, grouped bool, sig syscall.Signal) error {
	if grouped {
		return syscall.Kill(-pid, sig)
	}
	return syscall.Kill(pid, sig)
}

func killChild(pid int, grouped bool) error {
	return signalChild(pid, grouped, syscall.SIGKILL)
}

// childAlive reports whether the child (or, for a group, any member) still exists.
func childAlive(pid int, grouped bool) bool {
	return signalChild(pid, grouped, 0) == nil
}

// finishGroupCancellation gives the signalled group time to exit before killing
// survivors. It returns as soon as the group disappears and leaves no delayed
// signal behind after Cmd.Wait completes. Unix group signaling uses numeric PGIDs;
// probing and signaling a group cannot provide an atomic identity guarantee. The leader may exit before children
// that ignore SIGTERM, so its Wait result cannot decide whether cleanup is done.
func finishGroupCancellation(pid int) {
	deadline := time.Now().Add(childShutdownGrace)
	for childAlive(pid, true) {
		if !time.Now().Before(deadline) {
			_ = killChild(pid, true)
			return
		}
		time.Sleep(childPollInterval)
	}
}
