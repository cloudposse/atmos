//go:build !windows

package process

import (
	"os"
	"os/exec"
	"strconv"
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// processGone reports whether pid no longer exists. A zombie that was not
// reaped (possible for orphans in containers without an init process) counts
// as gone: it can no longer run code.
func processGone(pid int) bool {
	if err := syscall.Kill(pid, 0); err != nil {
		return true
	}
	data, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/stat")
	if err != nil {
		return false
	}
	// Format: "pid (comm) S ..." -- the state follows the last ')'.
	s := string(data)
	for i := len(s) - 1; i >= 0; i-- {
		if s[i] == ')' {
			return i+2 < len(s) && s[i+2] == 'Z'
		}
	}
	return false
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
