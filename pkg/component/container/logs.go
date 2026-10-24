package container

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"sync"

	errUtils "github.com/cloudposse/atmos/errors"
	iolib "github.com/cloudposse/atmos/pkg/io"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/signals"
	"github.com/cloudposse/atmos/pkg/ui"
)

// defaultLogsTail mirrors `docker logs` / `podman logs` default of showing all lines.
const defaultLogsTail = "all"

// logsOptions controls how container logs are fetched.
type logsOptions struct {
	follow bool
	tail   string // number of lines to show from the end, or "all".
}

// logsOptionsFrom builds logsOptions from the command's parsed flag map, applying
// defaults for any missing entries.
func logsOptionsFrom(flags map[string]any) logsOptions {
	opts := logsOptions{tail: defaultLogsTail}
	if flags == nil {
		return opts
	}
	if v, ok := flags["follow"].(bool); ok {
		opts.follow = v
	}
	if v, ok := flags["tail"].(string); ok && v != "" {
		opts.tail = v
	}
	return opts
}

// ExecuteLogsWithOptions streams logs for one or many container components.
// A component argument streams just that one; `--all` or no component selects
// many (all components, or an interactive picker). With `--follow` and multiple
// components, logs are streamed concurrently and each line is prefixed with the
// component name; without `--follow` they are printed sequentially.
func ExecuteLogsWithOptions(ctx context.Context, info *schema.ConfigAndStacksInfo, opts logsOptions) error {
	defer perf.Track(nil, "container.ExecuteLogsWithOptions")()

	if opts.tail == "" {
		opts.tail = defaultLogsTail
	}

	// A component argument and --all are mutually exclusive.
	if info.All && info.ComponentFromArg != "" {
		return errUtils.Build(errUtils.ErrContainerComponentWithAll).
			WithCausef("component %q given with --all", info.ComponentFromArg).
			WithHint("Drop the component argument to stream all components, or drop --all to stream just that component.").
			Err()
	}

	// Single component: stream just that one (with optional follow/tail).
	if info.ComponentFromArg != "" && !info.All {
		return streamSingleLogs(ctx, info, opts)
	}

	// Multiple components: resolve the target set (all, or interactive picker).
	targets, err := resolveBulkTargets(info, "logs")
	if err != nil {
		return err
	}
	if len(targets) == 0 {
		ui.Info("No container components selected")
		return nil
	}
	if opts.follow {
		return streamLogsConcurrent(ctx, info, opts, targets)
	}
	return streamLogsSequential(ctx, info, opts, targets)
}

// streamSingleLogs streams logs for the single component named in info to the
// default data/UI channels (no prefix). When following, Ctrl-C stops gracefully
// (the interrupt is caught, the runtime error it causes is suppressed, and a
// friendly closing line is printed) instead of surfacing as a command error.
func streamSingleLogs(ctx context.Context, info *schema.ConfigAndStacksInfo, opts logsOptions) error {
	d, err := discover(ctx, info)
	if err != nil {
		return err
	}

	if opts.follow {
		var stop func()
		ctx, stop = followContext(ctx)
		defer stop()
	}

	err = d.runtime.Logs(ctx, containerRef(d.in), opts.follow, opts.tail, nil, nil)
	if opts.follow && ctx.Err() != nil {
		// The user interrupted (Ctrl-C): a graceful stop, not a failure.
		noteFollowStopped()
		return nil
	}
	if err != nil {
		return fmt.Errorf("%w: logs %q: %w", errUtils.ErrComponentExecutionFailed, d.r.component, err)
	}
	return nil
}

// followContext returns a context that is canceled on Ctrl-C (SIGINT) and, while
// active, suspends Atmos's global interrupt-exit. Without the suspension the
// process-wide SIGINT handler in main() would os.Exit(130) before this command
// could stop gracefully; with it, the foreground `docker/podman logs -f`
// children receive Ctrl-C, exit, and control returns here for a clean close.
// The returned stop releases both the signal notifier and the suspension.
func followContext(ctx context.Context) (context.Context, func()) {
	release := signals.SuspendInterruptExit()
	sigCtx, stop := signal.NotifyContext(ctx, os.Interrupt)
	return sigCtx, func() {
		stop()
		release()
	}
}

// noteFollowStopped prints a friendly closing line after a `--follow` stream is
// interrupted with Ctrl-C. The leading blank line separates it from the
// terminal's "^C" echo.
func noteFollowStopped() {
	ui.Writeln("")
	ui.Info("Stopped following logs")
}

// logsInfoFor returns a per-target copy of info scoped to a single component.
func logsInfoFor(info *schema.ConfigAndStacksInfo, t *instanceRow) schema.ConfigAndStacksInfo {
	itemInfo := *info
	itemInfo.ComponentFromArg = t.component
	itemInfo.Component = t.component
	itemInfo.Stack = t.stack
	itemInfo.All = false
	return itemInfo
}

// streamLogsSequential prints each component's logs in turn, separated by a
// header, continuing past per-component failures and aggregating them.
func streamLogsSequential(ctx context.Context, info *schema.ConfigAndStacksInfo, opts logsOptions, targets []instanceRow) error {
	var errs []error
	for _, t := range targets {
		itemInfo := logsInfoFor(info, &t)
		ui.Infof("==> %s/%s <==", t.stack, t.component)
		if err := streamSingleLogs(ctx, &itemInfo, opts); err != nil {
			ui.Errorf("%s/%s: logs failed: %v", t.stack, t.component, err)
			errs = append(errs, fmt.Errorf("%s/%s: %w", t.stack, t.component, err))
		}
	}
	if len(errs) > 0 {
		return errors.Join(errs...)
	}
	return nil
}

// maxComponentNameLen returns the longest component name across targets, used to
// width-align per-component log labels.
func maxComponentNameLen(targets []instanceRow) int {
	width := 0
	for _, t := range targets {
		if len(t.component) > width {
			width = len(t.component)
		}
	}
	return width
}

// centerLabel uppercases s and centers it within the given width (no-op if
// already wider), so per-component labels read like the uppercase log-level
// badges and stay aligned across components.
func centerLabel(s string, width int) string {
	s = strings.ToUpper(s)
	if len(s) >= width {
		return s
	}
	pad := width - len(s)
	left := pad / 2
	return strings.Repeat(" ", left) + s + strings.Repeat(" ", pad-left)
}

// streamLogsConcurrent follows all targets at once, interleaving their output
// with a per-component line prefix. It returns when every stream ends (each
// container stops) or the user interrupts with Ctrl-C.
func streamLogsConcurrent(ctx context.Context, info *schema.ConfigAndStacksInfo, opts logsOptions, targets []instanceRow) error {
	// Ctrl-C cancels every stream (and suspends Atmos's global interrupt-exit) so
	// the command stops gracefully instead of exiting 130.
	ctx, stop := followContext(ctx)
	defer stop()

	var wg sync.WaitGroup
	streamer := &logStreamer{
		opts: opts,
		// Width-align labels so log content lines up across components.
		width: maxComponentNameLen(targets),
		// One shared lock keeps concurrent prefixed lines from interleaving mid-line.
		writeMu: &sync.Mutex{},
		wg:      &wg,
		errCh:   make(chan error, len(targets)),
	}

	// Real discovery failures (runtime/config/integration) are collected and
	// surfaced; only the expected "no running container" case is skippable.
	var discoverErrs []error

	for i, t := range targets {
		itemInfo := logsInfoFor(info, &t)
		d, err := discover(ctx, &itemInfo)
		if err != nil {
			if errors.Is(err, errUtils.ErrNoRunningContainer) {
				// A component that has no running container is skipped, not fatal.
				ui.Warningf("%s/%s: %v", t.stack, t.component, err)
				continue
			}
			// Any other failure must surface instead of being masked by the
			// success path below.
			discoverErrs = append(discoverErrs, fmt.Errorf("%w: %s/%s: %w", errUtils.ErrComponentExecutionFailed, t.stack, t.component, err))
			continue
		}

		streamer.launch(ctx, d, &t, i)
	}

	wg.Wait()
	close(streamer.errCh)

	// Real discovery failures surface regardless of how streaming ended.
	errs := discoverErrs

	// Ctrl-C: graceful stop (per-stream errors were already suppressed above).
	if ctx.Err() != nil {
		noteFollowStopped()
		if len(errs) > 0 {
			return errors.Join(errs...)
		}
		return nil
	}

	for e := range streamer.errCh {
		errs = append(errs, e)
	}
	if len(errs) > 0 {
		return errors.Join(errs...)
	}
	return nil
}

// logStreamer carries the loop-invariant state shared by every concurrent log
// stream launched in streamLogsConcurrent.
type logStreamer struct {
	opts    logsOptions
	width   int
	writeMu *sync.Mutex
	wg      *sync.WaitGroup
	errCh   chan error
}

// launch follows one discovered target's logs in a new goroutine, writing output
// behind a colored per-component prefix (cycled by index, degrading to `[NAME]`
// without color) and reporting any non-cancellation error on the shared errCh.
func (s *logStreamer) launch(ctx context.Context, d *discovered, t *instanceRow, index int) {
	// The trailing space separates the label from the log line.
	prefix := ui.FormatComponentLabel(centerLabel(t.component, s.width), index) + " "
	stdout := iolib.NewLinePrefixWriterRaw(prefix, iolib.Data, s.writeMu)
	stderr := iolib.NewLinePrefixWriterRaw(prefix, iolib.UI, s.writeMu)
	runtime, ref, stack, comp := d.runtime, containerRef(d.in), t.stack, t.component

	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		defer func() { _ = stdout.Flush(); _ = stderr.Flush() }()
		// Suppress the error caused by our own cancellation (Ctrl-C).
		if err := runtime.Logs(ctx, ref, true, s.opts.tail, stdout, stderr); err != nil && ctx.Err() == nil {
			s.errCh <- fmt.Errorf("%w: %s/%s: %w", errUtils.ErrComponentExecutionFailed, stack, comp, err)
		}
	}()
}
