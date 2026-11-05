//go:build !windows

package process

import (
	"errors"
	"os"
	"os/exec"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// processGone reports whether pid no longer exists. A zombie that was not
// reaped (possible for orphans in containers without an init process) counts
// as gone: it can no longer run code.
func processGone(pid int) bool {
	if err := syscall.Kill(pid, 0); err != nil {
		return errors.Is(err, syscall.ESRCH)
	}
	if processIsZombie(pid) {
		return true
	}
	// The process may have disappeared while its state was being inspected.
	return errors.Is(syscall.Kill(pid, 0), syscall.ESRCH)
}

func currentProcessGroup() int { return syscall.Getpgrp() }

func processGroupOf(t *testing.T, pid int) int {
	t.Helper()
	pgid, err := syscall.Getpgid(pid)
	require.NoError(t, err)
	return pgid
}

func assertProcessGroupAttr(t *testing.T, cmd *exec.Cmd, wantGrouped bool) {
	t.Helper()
	if wantGrouped {
		require.NotNil(t, cmd.SysProcAttr)
		assert.True(t, cmd.SysProcAttr.Setpgid)
		assert.NotNil(t, cmd.Cancel)
		return
	}
	if cmd.SysProcAttr != nil {
		assert.False(t, cmd.SysProcAttr.Setpgid)
	}
}

// A killed child cannot run even if its parent has not reaped its exit status yet.
func TestProcessGoneRecognizesUnreapedChild(t *testing.T) {
	dir := t.TempDir()
	cmd := exec.Command(os.Args[0])
	cmd.Env = append(os.Environ(), helperModeEnv+"="+modeSleep, helperPIDDirEnv+"="+dir)
	require.NoError(t, cmd.Start())
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	})
	pid := waitForPID(t, dir, "child")
	require.False(t, processGone(pid), "a running child must still be reported alive")
	require.True(t, childAlive(pid, false), "an ungrouped child must be checked by PID")
	require.NoError(t, cmd.Process.Kill())
	// Do not call Wait until cleanup: the exit status must remain unreaped here.
	require.Eventually(t, func() bool { return processGone(pid) }, goneWait, 25*time.Millisecond,
		"a killed, unreaped child must not be mistaken for a running process")
}
