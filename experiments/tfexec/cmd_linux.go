//go:build linux && !linting

package main

import "syscall"

// newSysProcAttr returns the correct SysProcAttr for Linux.
func newSysProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{
		Pdeathsig: syscall.SIGKILL, // Kill child process if parent dies
		Setpgid:   true,            // Set process group ID
	}
}
