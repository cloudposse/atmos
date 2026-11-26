package process

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/signals"
)

// InterruptPolicy selects how a running child reacts when Atmos receives SIGINT or SIGTERM.
type InterruptPolicy int

const (
	// InterruptKill terminates the child's process tree when Atmos is interrupted.
	// It is the default for every subprocess.
	InterruptKill InterruptPolicy = iota
	// InterruptGraceful lets the child stop on its own, as Terraform does to save state
	// and release its lock. The first SIGINT/SIGTERM is delivered to the child exactly once
	// and Atmos waits for it to exit; a second signal terminates it as InterruptKill would.
	// Context cancellation that was not caused by a signal, such as a timeout, still
	// terminates the child immediately.
	InterruptGraceful
)

// gracefulStops is the registry graceful children join. Tests substitute a fresh one.
var gracefulStops = signals.DefaultGracefulStops()

// gracefulStartError refuses to start a graceful child after Atmos began stopping,
// so an interrupted multi-step or multi-component run starts no new Terraform command.
func gracefulStartError(ctx context.Context, policy InterruptPolicy) error {
	if policy != InterruptGraceful {
		return nil
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if gracefulStops.Stopping() {
		return errUtils.ErrProcessInterrupted
	}
	return nil
}

// executionContext returns the context the child runs under and its cancel function.
// A graceful child is detached from cancellation caused by an interrupt signal: the
// signal handler forwards the interrupt and a second signal terminates the child
// through exit cleanup. Any other cancellation of ctx still terminates it.
func executionContext(ctx context.Context, policy InterruptPolicy) (context.Context, context.CancelFunc) {
	if policy != InterruptGraceful {
		return context.WithCancel(ctx)
	}
	executionCtx, cancel := context.WithCancel(context.WithoutCancel(ctx))
	stopWatching := context.AfterFunc(ctx, func() {
		if !signals.InterruptedBySignal(ctx) {
			cancel()
		}
	})
	return executionCtx, func() {
		stopWatching()
		cancel()
	}
}

// joinGracefulStop registers a started graceful child so the first signal is forwarded
// to it and Atmos waits for it to exit. The returned function leaves the registry.
func (g *childGroup) joinGracefulStop(command string, policy InterruptPolicy) func() {
	if policy != InterruptGraceful || g.cmd.Process == nil {
		return func() {}
	}
	pid, grouped := g.cmd.Process.Pid, g.grouped
	return gracefulStops.Register(commandDisplayName(command), func(sig os.Signal) {
		forwardInterrupt(pid, grouped, sig)
	})
}

// commandDisplayName names a command for the interrupt notice, e.g. "terraform" or "tofu".
func commandDisplayName(command string) string {
	name := filepath.Base(command)
	if strings.EqualFold(filepath.Ext(name), ".exe") {
		name = name[:len(name)-len(".exe")]
	}
	return name
}
