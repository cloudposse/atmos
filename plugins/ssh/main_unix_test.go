//go:build !windows

package main

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/plugins/ssh/internal/testfakes"
	"github.com/cloudposse/atmos/plugins/ssh/paths"
)

// The real daemon starts without the plugin handshake and stops on SIGTERM. The test process
// signals itself; dispatch has already registered for the signal by the time the state file exists.
func TestTheRealDaemonServesWithoutAHandshakeAndStopsOnSIGTERM(t *testing.T) {
	root := filepath.Join(testfakes.ShortDir(t), "ssh")
	e := realEntry()
	done := make(chan int, 1)

	go func() { done <- dispatch([]string{"agent-serve", "--root", root}, e) }()

	p, err := paths.New(root)
	require.NoError(t, err)
	state := p.StateFile()
	require.Eventually(t, func() bool {
		_, statErr := os.Stat(state)
		return statErr == nil
	}, 10*time.Second, 20*time.Millisecond, "the daemon wrote its state file")
	data, err := os.ReadFile(state)
	require.NoError(t, err)
	assert.Contains(t, string(data), `"version":"0.1.0"`)

	require.NoError(t, syscall.Kill(os.Getpid(), syscall.SIGTERM))

	select {
	case code := <-done:
		assert.Equal(t, 0, code)
	case <-time.After(10 * time.Second):
		t.Fatal("the daemon did not stop on SIGTERM")
	}
}
