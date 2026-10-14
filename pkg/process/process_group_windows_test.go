//go:build windows

package process

import (
	"os/exec"
	"testing"
)

// processGone is never consulted on Windows: the tests that use it skip there.
func processGone(int) bool { return true }

func currentProcessGroup() int { return 0 }

func processGroupOf(*testing.T, int) int { return 0 }

func assertProcessGroupAttr(*testing.T, *exec.Cmd, bool) {}
