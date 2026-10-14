package process

import "golang.org/x/sys/unix"

// A Darwin process group can stop accepting signals while a member is still
// completing exit. Inspect all members rather than treating kill(-pgid, 0) as
// proof that they have stopped. Zombies cannot execute and need not be reaped
// by Atmos, which is not the parent of every descendant.
func processGroupAlive(pid int) bool {
	members, err := unix.SysctlKinfoProcSlice("kern.proc.pgrp", pid)
	return darwinProcessGroupAlive(members, err)
}

func darwinProcessGroupAlive(members []unix.KinfoProc, err error) bool {
	if err != nil {
		// An unavailable snapshot is not evidence that cleanup has finished.
		// The caller's existing deadline still bounds the wait.
		return true
	}
	const zombieState = 5 // SZOMB in Darwin's sys/proc.h.
	for i := range members {
		if members[i].Proc.P_stat != zombieState {
			return true
		}
	}
	return false
}
