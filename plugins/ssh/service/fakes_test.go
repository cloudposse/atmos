package service

import (
	"context"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/plugins/ssh/paths"
)

// fakeRunner records commands and answers them from a script. It never runs anything.
type fakeRunner struct {
	mu    sync.Mutex
	calls []Command
	// reply decides each result from the command line ("name arg arg").
	reply func(line string, n int) Output
}

func (f *fakeRunner) Run(_ context.Context, c Command) (Output, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, c)
	if f.reply == nil {
		return Output{}, nil
	}
	return f.reply(line(c), len(f.calls)), nil
}

func line(c Command) string { return strings.Join(append([]string{c.Name}, c.Args...), " ") }

func (f *fakeRunner) lines() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]string, len(f.calls))
	for i, c := range f.calls {
		out[i] = line(c)
	}
	return out
}

// testPaths returns Paths under a temporary root and a definition directory beside it.
func testPaths(t *testing.T) (*paths.Paths, string) {
	t.Helper()
	root := t.TempDir()
	p, err := paths.New(filepath.Join(root, "ssh"))
	require.NoError(t, err)
	return p, filepath.Join(root, "definitions")
}

func noPause(context.Context, time.Duration) error { return nil }
