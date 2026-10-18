package process

import "golang.org/x/sys/unix"

// Darwin's sys/proc.h defines SZOMB as an exited process awaiting collection.
const darwinZombieState = 5

func processIsZombie(pid int) bool {
	info, err := unix.SysctlKinfoProc("kern.proc.pid", pid)
	return err == nil && info.Proc.P_stat == darwinZombieState
}
