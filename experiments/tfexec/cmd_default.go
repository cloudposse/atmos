//go:build !linux && !linting

package main

import "syscall"

// newSysProcAttr returns the correct SysProcAttr for non-Linux platforms.
func newSysProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{
		Setpgid: true, // Still set process group ID for cleanup
	}
}
