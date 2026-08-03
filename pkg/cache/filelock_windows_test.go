//go:build windows

package cache

import "testing"

func TestTryWithRLockWindowsExecutesCallback(t *testing.T) {
	executed := false
	acquired, err := NewFileLockAtPath("C:/tmp/repositories.lock").TryWithRLock(func() error {
		executed = true
		return nil
	})
	if err != nil {
		t.Fatalf("TryWithRLock returned error: %v", err)
	}
	if !acquired || !executed {
		t.Fatalf("TryWithRLock should acquire and execute on Windows: acquired=%t executed=%t", acquired, executed)
	}
}
