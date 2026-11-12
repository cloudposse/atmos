//go:build !windows

package wiring

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/plugins/ssh/internal/testfakes"
	"github.com/cloudposse/atmos/plugins/ssh/paths"
)

func TestServeAgentRunsUntilTheContextEnds(t *testing.T) {
	root := filepath.Join(testfakes.ShortDir(t), "ssh")
	p, err := paths.New(root)
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- ServeAgent(ctx, root, "9.9.9", func(string, ...any) {}) }()

	require.Eventually(t, func() bool {
		_, err := os.Stat(p.StateFile())
		return err == nil
	}, 10*time.Second, 20*time.Millisecond)
	cancel()

	select {
	case err := <-done:
		assert.NoError(t, err)
	case <-time.After(10 * time.Second):
		t.Fatal("the daemon did not stop")
	}
	_, err = os.Stat(p.StateFile())
	assert.True(t, os.IsNotExist(err), "the state file is removed on exit")
}
