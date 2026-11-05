//go:build !windows && !darwin

package process

func processGroupAlive(pid int) bool {
	return signalChild(pid, true, 0) == nil
}
