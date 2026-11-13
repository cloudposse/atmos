package git

import (
	"os"
	"os/signal"
	"sync"
	"syscall"

	"github.com/cloudposse/atmos/pkg/perf"
)

// signalExitBase is the conventional exit status offset for a process ended by a signal.
const signalExitBase = 128

// WorktreeSignalCleanups removes temporary worktrees when the process receives SIGINT or SIGTERM.
// SIGINT and SIGTERM skip deferred functions, so a worktree and its `.git/worktrees` registration
// would otherwise leak. One watcher owns every active cleanup, so none can exit ahead of another.
type WorktreeSignalCleanups struct {
	mu      sync.Mutex
	watcher *worktreeSignalWatcher
	// exit ends the process with a status; callers inject it so tests can observe it.
	exit func(code int)
}

type worktreeSignalWatcher struct {
	signals  chan os.Signal
	done     chan struct{}
	cleanups map[*worktreeSignalCleanup]struct{}
}

type worktreeSignalCleanup struct {
	run func()
}

// NewWorktreeSignalCleanups returns cleanups that end the process with exit after a signal.
func NewWorktreeSignalCleanups(exit func(code int)) *WorktreeSignalCleanups {
	return &WorktreeSignalCleanups{exit: exit}
}

// Register runs cleanup on SIGINT or SIGTERM, together with every other registered cleanup, before
// exiting with the conventional 128+signal status. Call the returned function after normal cleanup.
func (c *WorktreeSignalCleanups) Register(cleanup func()) func() {
	defer perf.Track(nil, "git.WorktreeSignalCleanups.Register")()

	c.mu.Lock()
	defer c.mu.Unlock()
	watcher := c.watcher
	if watcher == nil {
		watcher = &worktreeSignalWatcher{
			signals:  make(chan os.Signal, 1),
			done:     make(chan struct{}),
			cleanups: make(map[*worktreeSignalCleanup]struct{}),
		}
		c.watcher = watcher
		signal.Notify(watcher.signals, syscall.SIGINT, syscall.SIGTERM)
		go c.watch(watcher)
	}
	key := &worktreeSignalCleanup{run: cleanup}
	watcher.cleanups[key] = struct{}{}
	return sync.OnceFunc(func() {
		c.mu.Lock()
		defer c.mu.Unlock()
		delete(watcher.cleanups, key)
		if len(watcher.cleanups) == 0 {
			signal.Stop(watcher.signals)
			close(watcher.done)
			c.watcher = nil
		}
	})
}

func (c *WorktreeSignalCleanups) watch(watcher *worktreeSignalWatcher) {
	select {
	case received := <-watcher.signals:
		c.terminate(watcher, received)
	case <-watcher.done:
		// A signal that arrived before the final release still terminates the process.
		select {
		case received := <-watcher.signals:
			c.terminate(watcher, received)
		default:
		}
	}
}

// terminate runs the cleanups still registered and exits with the signal's status. Normal release
// may already have retired this watcher and run every cleanup; the received signal still ends the
// process with its documented status instead of letting the command report success.
func (c *WorktreeSignalCleanups) terminate(watcher *worktreeSignalWatcher, received os.Signal) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.watcher == watcher {
		for cleanup := range watcher.cleanups {
			cleanup.run()
		}
	}
	code := 1
	if number, ok := received.(syscall.Signal); ok {
		code = signalExitBase + int(number)
	}
	c.exit(code)
}
