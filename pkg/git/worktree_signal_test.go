package git

import (
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A signal queued for a watcher that its final release retired still cleans up the registrations
// that another caller made afterwards, instead of exiting and leaking their worktrees.
func TestRetiredWatcherSignalCleansActiveWorktrees(t *testing.T) {
	var exits []int
	cleanups := NewWorktreeSignalCleanups(func(code int) { exits = append(exits, code) })
	var cleanedA, cleanedB int

	releaseA := cleanups.Register(func() { cleanedA++ })
	cleanups.mu.Lock()
	retired := cleanups.watcher
	cleanups.mu.Unlock()
	require.NotNil(t, retired)
	releaseA()

	releaseB := cleanups.Register(func() { cleanedB++ })
	defer releaseB()

	cleanups.terminate(retired, syscall.SIGTERM)

	assert.Equal(t, 1, cleanedB, "the registration made after the first watcher retired is cleaned")
	assert.Zero(t, cleanedA, "a released registration is not cleaned again")
	assert.Equal(t, []int{signalExitBase + int(syscall.SIGTERM)}, exits)
}

// A retired watcher with nothing registered afterwards runs no cleanup but still ends the process.
func TestRetiredWatcherSignalWithoutActiveWorktreesStillExits(t *testing.T) {
	var exits []int
	cleanups := NewWorktreeSignalCleanups(func(code int) { exits = append(exits, code) })
	cleaned := 0

	release := cleanups.Register(func() { cleaned++ })
	cleanups.mu.Lock()
	retired := cleanups.watcher
	cleanups.mu.Unlock()
	release()

	cleanups.terminate(retired, syscall.SIGINT)

	assert.Zero(t, cleaned)
	assert.Equal(t, []int{signalExitBase + int(syscall.SIGINT)}, exits)
}
